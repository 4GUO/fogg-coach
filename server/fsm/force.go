package fsm

// ForceFill 25 轮兜底（§3.5）：到达轮次上限时后端强制补齐全部默认值，
// 让用户永远能拿到计划离开。后端权限，不走 LLM 标记路径。
func ForceFill(ctx *Context) {
	ApplySkipDefaults(S1, ctx)
	ApplySkipDefaults(S2, ctx)
	if len(ctx.Candidates) < 5 {
		seen := map[string]bool{}
		for _, c := range ctx.Candidates {
			seen[c] = true
		}
		for _, c := range defaultCandidates {
			if !seen[c] {
				ctx.Candidates = append(ctx.Candidates, c)
			}
		}
	}
	if len(ctx.Golden) < 1 {
		ctx.Golden = []string{ctx.Candidates[0]}
	} else if len(ctx.Golden) > 3 {
		ctx.Golden = ctx.Golden[:3]
	}
	ApplySkipDefaults(S5, ctx)
}
