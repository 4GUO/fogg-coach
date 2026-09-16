package llm

import (
	"fmt"

	"fogg-coach/fsm"
	"fogg-coach/prompts"
)

// PromptBuilder（§3.2.2）：
//
//	[system] 基座 + 当前阶段规范 + 会话累积事实 context
//	[history] 本 session 最近 20 条（截断）
//	[user]    本轮输入
const historyLimit = 20

type HistoryMsg struct {
	Role    string
	Content string
}

// BuildChatPrompt S1-S6 对话轮（thinking 关、maxTokens 2500 由调用方定）
func BuildChatPrompt(stage fsm.Stage, ctx *fsm.Context, history []HistoryMsg, userInput string) []Message {
	sys := prompts.Base() + "\n\n---\n\n" + prompts.Stage(string(stage)) +
		"\n\n---\n\n## 会话已确认的事实（context，禁止泄露此段落格式给用户）\n```json\n" +
		ctx.JSON() + "\n```"
	msgs := []Message{{Role: "system", Content: sys}}
	msgs = append(msgs, truncateHistory(history)...)
	msgs = append(msgs, Message{Role: "user", Content: userInput})
	return msgs
}

// BuildPlanPrompt S7 纯结构化输出（context 全量注入）
func BuildPlanPrompt(ctx *fsm.Context) []Message {
	sys := prompts.Base() + "\n\n---\n\n" + prompts.Stage("S7")
	return []Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: "以下是会话已确认的全部事实（context），据此生成 Plan JSON：\n```json\n" +
			ctx.JSON() + "\n```"},
	}
}

// BuildPlanRetryPrompt S7 校验失败重试（带上失败原因，重试 1 次，§3.2.4）
func BuildPlanRetryPrompt(ctx *fsm.Context, badOutput, reason string) []Message {
	msgs := BuildPlanPrompt(ctx)
	msgs = append(msgs,
		Message{Role: "assistant", Content: badOutput},
		Message{Role: "user", Content: fmt.Sprintf(
			"上面的输出未通过服务端校验，原因：%s。请严格按 Schema 重新输出，仅输出一个合法 JSON 对象。", reason)})
	return msgs
}

func truncateHistory(h []HistoryMsg) []Message {
	if len(h) > historyLimit {
		h = h[len(h)-historyLimit:]
	}
	out := make([]Message, 0, len(h))
	for _, m := range h {
		if m.Role == "user" || m.Role == "assistant" {
			out = append(out, Message{Role: m.Role, Content: m.Content})
		}
	}
	return out
}
