package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"fogg-coach/config"
)

// OpenAICompatible 覆盖 GLM/DeepSeek（baseUrl + model 不同即可）。
// §3.2.5 实测结论已内置：reasoning_content 只忽略；thinking 按轮次开关。
type OpenAICompatible struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func NewFromConfig() *OpenAICompatible {
	cfg := config.Get()
	return &OpenAICompatible{
		BaseURL: strings.TrimSuffix(cfg.LLMBaseURL, "/"),
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
		Client:  &http.Client{Timeout: 180 * time.Second},
	}
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []Message     `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens"`
	Stream         bool          `json:"stream"`
	Thinking       *thinkingConf `json:"thinking,omitempty"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type thinkingConf struct {
	Type string `json:"type"` // enabled | disabled
}

type respFormat struct {
	Type string `json:"type"` // json_object
}

func (p *OpenAICompatible) Chat(ctx context.Context, opts ChatOpts) (*ChatResult, error) {
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 2500
	}
	think := "disabled"
	if opts.Thinking {
		think = "enabled"
	}
	body := chatRequest{
		Model: p.Model, Messages: opts.Messages, Temperature: opts.Temperature,
		MaxTokens: opts.MaxTokens, Stream: opts.OnDelta != nil,
		Thinking: &thinkingConf{Type: think},
	}
	if opts.JSONMode {
		body.ResponseFormat = &respFormat{Type: "json_object"}
	}

	// 429 指数退避重试 2 次（§4.6）
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<attempt) * time.Second): // 2s, 4s
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		res, err := p.call(ctx, body, opts)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("重试耗尽: %w", lastErr)
}

type httpErr struct {
	Code int
	Body string
}

func (e *httpErr) Error() string { return fmt.Sprintf("http %d: %s", e.Code, e.Body) }

func isRetryable(err error) bool {
	he, ok := err.(*httpErr)
	if !ok {
		return false // 网络错误交给上层（对话中已流式，无法重放）
	}
	return he.Code == 429 || he.Code >= 500
}

func (p *OpenAICompatible) call(ctx context.Context, body chatRequest, opts ChatOpts) (*ChatResult, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions",
		bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &httpErr{Code: resp.StatusCode, Body: string(b)}
	}

	if !body.Stream {
		return p.parseNonStream(resp.Body)
	}
	return p.parseStream(resp.Body, opts.OnDelta)
}

type wireResp struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"` // 忽略（§3.2.5）
		} `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

func (p *OpenAICompatible) parseNonStream(r io.Reader) (*ChatResult, error) {
	var w wireResp
	if err := json.NewDecoder(r).Decode(&w); err != nil {
		return nil, err
	}
	if len(w.Choices) == 0 {
		return nil, fmt.Errorf("空 choices")
	}
	return &ChatResult{Content: w.Choices[0].Message.Content, Usage: w.Usage}, nil
}

// parseStream 逐行读 SSE：data: {"choices":[{"delta":{"content":"..."}}]}
// delta.reasoning_content 一律丢弃；[DONE] 结束。
func (p *OpenAICompatible) parseStream(r io.Reader, onDelta func(string)) (*ChatResult, error) {
	var full strings.Builder
	var usage Usage
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue // 容忍非 JSON 心跳行
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			full.WriteString(chunk.Choices[0].Delta.Content)
			onDelta(chunk.Choices[0].Delta.Content)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &ChatResult{Content: full.String(), Usage: usage}, nil
}
