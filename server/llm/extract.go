package llm

import (
	"fogg-coach/fsm"
)

// 信息抽取层（工程内部 prompt，不触碰 S1-S7 话术资产）。
//
// 为什么需要：FSM 推进条件依赖 context 字段（S2 需 gaps≥2 等），而阶段对话
// prompt 面向用户输出、不携带结构化字段。每轮对话后用轻量 JSON 调用提取，
// 失败不致命（context 不更新，FSM 按 HOLD 继续）。
//
// 已知代价：每轮 +1 次 LLM 调用（thinking 关、maxTokens 500，实测 ~1s）。
// 优化方向（需用户确认后改协议）：阶段 prompt 尾部追加 [CTX:{...}] 标记合并单调用。

const extractSystem = `你是信息抽取器。从教练对话中抽取当前阶段所需字段，仅输出一个 JSON 对象，不要输出任何其他文字。
抽取规则：只抽用户已明确表达或教练已明确列出的事实；不确定的字段不要输出；数组字段输出完整列表（非增量）。`

func extractTarget(stage fsm.Stage) string {
	switch stage {
	case fsm.S1:
		return `只可能包含：{"wish": "用户认可的具体正向愿望，≤30字"}（用户尚未认可任何愿望时输出 {}）`
	case fsm.S2:
		return `只可能包含：{"motivation": "动机描述", "ability_gaps": ["能力障碍", ...], "anchors_found": ["日常锚点", ...]}（用户提到的才抽）`
	case fsm.S3:
		return `只可能包含：{"candidates": ["教练本轮列出的全部候选行为", ...]}（教练未列编号清单时输出 {}）`
	case fsm.S5:
		return `只可能包含：{"s5_recipes": [{"behavior": "微行为", "anchor": "在我X之后", "celebration": "庆祝方式", "anchor_time": "大概几点，如 22:30 或 晚上10点半"}]}（本轮已确认配方的才抽，anchor 必须以"在我"开头，用户提到时间就填 anchor_time）`
	}
	return `输出 {}`
}

// BuildExtractPrompt 组装抽取调用：阶段目标字段 + 已有 context + 最近对话
func BuildExtractPrompt(stage fsm.Stage, ctx *fsm.Context, history []HistoryMsg) []Message {
	dialog := ""
	n := len(history)
	if n > 8 {
		history = history[n-8:]
	}
	for _, m := range history {
		role := "教练"
		if m.Role == "user" {
			role = "用户"
		}
		dialog += role + "：" + m.Content + "\n"
	}
	return []Message{
		{Role: "system", Content: extractSystem + "\n\n本阶段目标字段：" + extractTarget(stage) +
			"\n\n当前已有 context（仅供参考，输出的是更新后的完整字段值）：\n" + ctx.JSON()},
		{Role: "user", Content: "对话记录：\n" + dialog + "\n输出 JSON："},
	}
}

// MergeExtract 把抽取结果合并进 context（数组按去重合并，标量直接覆盖）
func MergeExtract(ctx *fsm.Context, raw string) {
	var upd struct {
		Wish         string   `json:"wish"`
		Motivation   string   `json:"motivation"`
		AbilityGaps  []string `json:"ability_gaps"`
		AnchorsFound []string `json:"anchors_found"`
		Candidates   []string `json:"candidates"`
		Recipes      []struct {
			Behavior    string `json:"behavior"`
			Anchor      string `json:"anchor"`
			Celebration string `json:"celebration"`
			AnchorTime  string `json:"anchor_time"`
		} `json:"s5_recipes"`
	}
	if err := jsonUnmarshalLoose(raw, &upd); err != nil {
		return
	}
	if upd.Wish != "" {
		ctx.Wish = upd.Wish
	}
	if upd.Motivation != "" {
		ctx.Motivation = upd.Motivation
	}
	ctx.AbilityGaps = mergeUnique(ctx.AbilityGaps, upd.AbilityGaps)
	ctx.AnchorsFound = mergeUnique(ctx.AnchorsFound, upd.AnchorsFound)
	if len(upd.Candidates) > 0 {
		ctx.Candidates = upd.Candidates
	}
	if len(upd.Recipes) > 0 {
		ctx.Recipes = nil
		for _, r := range upd.Recipes {
			ctx.Recipes = append(ctx.Recipes, fsm.Recipe(r))
		}
	}
}
