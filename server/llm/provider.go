// Package llm Provider 抽象（system-design §3.2）。
// OpenAI 兼容协议，GLM/DeepSeek 仅 baseUrl+model 不同。
package llm

import "context"

type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

type Usage struct {
	In  int `json:"prompt_tokens"`
	Out int `json:"completion_tokens"`
}

type ChatOpts struct {
	Messages     []Message
	Temperature  float64
	JSONMode     bool             // S7 结构化输出
	Thinking     bool             // §3.2.5：S1-S6 关，S7 开
	MaxTokens    int
	OnDelta      func(string)     // SSE 流式回调；nil = 非流式
}

type ChatResult struct {
	Content string
	Usage   Usage
}

type Provider interface {
	Chat(ctx context.Context, opts ChatOpts) (*ChatResult, error)
}
