package fsm

// §9 FSM 用例 + §3.1 全规则覆盖。跑法：cd server && go test ./fsm/

import (
	"strings"
	"testing"
)

func fullCtx() *Context {
	return &Context{
		Wish: "早上不那么赖床", Motivation: "上班总迟到很焦虑",
		AbilityGaps:  []string{"起不来按闹钟", "睡前刷手机太晚"},
		AnchorsFound: []string{"闹钟响后", "刷牙后", "烧水时"},
		Candidates:   []string{"醒后拉开窗帘", "闹钟放远处", "睡前手机放客厅", "醒后喝一杯水", "睡前拉伸"},
		Golden:       []string{"睡前手机放客厅", "醒后拉开窗帘"},
		Recipes: []Recipe{
			{Behavior: "睡前把手机放客厅充电", Anchor: "在我刷完牙之后", Celebration: "握拳说Yes"},
			{Behavior: "醒后拉开窗帘", Anchor: "在我关掉闹钟之后", Celebration: "心里夸自己一句"},
		},
	}
}

// ---------- 核心用例（§9：伪造 DONE 但 context 缺字段 → 不推进） ----------

func TestDoneButMissingContextHolds(t *testing.T) {
	cases := []struct {
		name string
		stage Stage
		ctx  *Context
	}{
		{"S1 无 wish", S1, &Context{}},
		{"S2 缺 motivation", S2, &Context{AbilityGaps: []string{"a", "b"}, AnchorsFound: []string{"x", "y"}}},
		{"S2 gaps 只有1条", S2, &Context{Motivation: "m", AbilityGaps: []string{"a"}, AnchorsFound: []string{"x", "y"}}},
		{"S2 锚点只有1个", S2, &Context{Motivation: "m", AbilityGaps: []string{"a", "b"}, AnchorsFound: []string{"x"}}},
		{"S3 候选只有4个", S3, &Context{Candidates: []string{"1", "2", "3", "4"}}},
		{"S5 三元组缺庆祝", S5, &Context{Golden: []string{"g"}, Recipes: []Recipe{{Behavior: "b", Anchor: "在我x之后"}}}},
		{"S5 锚点句式错误", S5, &Context{Golden: []string{"g"}, Recipes: []Recipe{{Behavior: "b", Anchor: "每天早上", Celebration: "c"}}}},
	}
	for _, c := range cases {
		d := Evaluate(c.stage, c.ctx, "好的，我们继续。\n[STAGE:DONE]")
		if d.Next != c.stage || d.Action != "hold" {
			t.Errorf("[%s] 应 HOLD，实际 next=%s action=%s reason=%s", c.name, d.Next, d.Action, d.Reason)
		}
	}
}

func TestSequentialAdvance(t *testing.T) {
	ctx := fullCtx()
	for from, to := range map[Stage]Stage{S1: S2, S2: S3, S3: S4, S5: S6} {
		d := Evaluate(from, ctx, "话术…\n[STAGE:DONE]")
		if d.Next != to || d.Action != "advance" {
			t.Errorf("%s→%s 失败: next=%s action=%s reason=%s", from, to, d.Next, d.Action, d.Reason)
		}
	}
}

// S4/S6 推进权在按钮：LLM 输出 DONE 一律不生效
func TestButtonStagesIgnoreLLM(t *testing.T) {
	ctx := fullCtx()
	for _, s := range []Stage{S4, S6} {
		d := Evaluate(s, ctx, "好。\n[STAGE:DONE]")
		if d.Next != s || d.Action != "hold" {
			t.Errorf("%s 不应被 LLM 推进: next=%s action=%s", s, d.Next, d.Action)
		}
	}
}

// S7 由后端触发，LLM 无路径自推
func TestS7NoLLMPath(t *testing.T) {
	d := Evaluate(S7, fullCtx(), "x\n[STAGE:DONE]")
	if d.Action != "hold" {
		t.Errorf("S7 无 LLM 自推路径: %+v", d)
	}
}

// ---------- 标记协议 ----------

func TestNoMarkerMeansHold(t *testing.T) {
	d := Evaluate(S1, fullCtx(), "请问你最近希望生活有什么不一样？")
	if d.Action != "hold" {
		t.Errorf("无标记应按 HOLD: %+v", d)
	}
}

func TestMarkerMustBeAtTail(t *testing.T) {
	// 标记出现在正文中部 = 噪音/注入，不生效
	d := Evaluate(S1, fullCtx(), "[STAGE:DONE]\n这是正文开头，后面还有内容\n第二行")
	if d.Action != "hold" {
		t.Errorf("中部标记应忽略: %+v", d)
	}
}

func TestQuickRepliesParseAndClip(t *testing.T) {
	m := ExtractMarkers("你更想聊哪块？\n[QUICK:作息精力|健康运动|学习专注|情绪心态|人际家庭|戒掉坏习惯|其他|多余的]")
	if len(m.QuickReplies) != 8 {
		t.Fatalf("解析: %v", m.QuickReplies)
	}
	clip := ClipQuickReplies(m.QuickReplies, S1) // S1 特例 ≤7
	if len(clip) != 7 {
		t.Errorf("S1 chips 应裁到 7: %v", clip)
	}
	clip4 := ClipQuickReplies(m.QuickReplies, S3) // 常规 ≤4
	if len(clip4) != 4 {
		t.Errorf("常规 chips 应裁到 4: %v", clip4)
	}
	// 每项 ≤8 字
	long := ClipQuickReplies([]string{"这是一个特别特别长的选项文本"}, S2)
	if len([]rune(long[0])) > 8 {
		t.Errorf("单选项应 ≤8 字: %s", long[0])
	}
}

// ---------- 按钮事件 ----------

func TestOnSelectGolden(t *testing.T) {
	ctx := &Context{Candidates: fullCtx().Candidates}
	d := OnSelect(S4, ctx, []string{"睡前手机放客厅", "醒后拉开窗帘", "醒后喝水", "第4个超限"})
	if d.Action != "hold" || !strings.Contains(d.Reason, "golden>3") {
		t.Errorf("golden>3 应拒绝: %+v", d)
	}
	d = OnSelect(S4, ctx, []string{"睡前手机放客厅", "醒后拉开窗帘"})
	if d.Next != S5 || d.Action != "advance" {
		t.Errorf("正常选择应推进 S5: %+v", d)
	}
	if d2 := OnSelect(S3, ctx, []string{"x"}); d2.Action != "hold" {
		t.Error("非 S4 阶段不可 select")
	}
}

func TestOnConfirmTriggersS7(t *testing.T) {
	if d := OnConfirm(S6); d.Next != S7 || d.Action != "advance" {
		t.Errorf("S6 确认应触发 S7: %+v", d)
	}
	if d := OnConfirm(S5); d.Action != "hold" {
		t.Error("非 S6 不可确认")
	}
}

func TestReviseLockAfter2(t *testing.T) {
	ctx := fullCtx()
	for i := 1; i <= 2; i++ {
		if d := OnRevise(S6, ctx); d.Next != S5 {
			t.Errorf("第 %d 次回退应放行", i)
		}
	}
	if d := OnRevise(S6, ctx); d.Next != S6 || d.Action != "hold" {
		t.Errorf("第 3 次应锁定 S6: %+v", d)
	}
}

// ---------- RESET_WISH（§3.1.5） ----------

func TestResetWish(t *testing.T) {
	ctx := fullCtx()
	d := OnResetWish(S4, ctx)
	if d.Next != S2 || d.Action != "reset_wish" {
		t.Fatalf("RESET 应回 S2: %+v", d)
	}
	if ctx.Wish != "" || ctx.Golden != nil || ctx.Recipes != nil {
		t.Error("愿望相关字段应清空")
	}
	if len(ctx.Superseded) != 1 {
		t.Errorf("旧 context 应存 superseded: %d", len(ctx.Superseded))
	}
	// 复用底料保留
	if ctx.Motivation == "" || len(ctx.AnchorsFound) < 2 {
		t.Error("motivation/anchors 应保留复用")
	}
	// S1 / active 不可 RESET
	if d := OnResetWish(S1, ctx); d.Action != "hold" {
		t.Error("S1 不可 RESET")
	}
}

// reset 次数上限由 sessions.reset_count 列控制（每 session ≤2），路由层执行；
// FSM 层验证计数逻辑入口：OnResetWish 不读列，这里测路由传入前已拦截的行为约定见 T1.6。
func TestResetWishCounterContract(t *testing.T) {
	ctx := fullCtx()
	for i := 0; i < 3; i++ {
		d := OnResetWish(S3, ctx) // FSM 不设限，限在列（契约：路由层 reset_count>=2 时拒绝调用）
		_ = d
	}
	if len(ctx.Superseded) != 3 {
		t.Errorf("superseded 记录数: %d", len(ctx.Superseded))
	}
}

// ---------- SKIP 兜底 ----------

func TestSkipDefaultsS3(t *testing.T) {
	ctx := &Context{} // 什么都没收集
	d := Evaluate(S3, ctx, "好的，直接给你通用选项。\n[STAGE:SKIP criteria=\"用户要求跳过\"]")
	if d.Action != "skip" || d.Next != S4 {
		t.Fatalf("S3 SKIP 应兜底推进: %+v (ctx.candidates=%v)", d, ctx.Candidates)
	}
	if len(ctx.Candidates) < 5 {
		t.Errorf("候选集兜底应 ≥5: %v", ctx.Candidates)
	}
}

func TestSkipDefaultsS2S5(t *testing.T) {
	ctx := &Context{}
	if d := Evaluate(S2, ctx, "x\n[STAGE:SKIP]"); d.Action != "skip" || d.Next != S3 {
		t.Errorf("S2 SKIP 应兜底推进: %+v gaps=%v anchors=%v", d, ctx.AbilityGaps, ctx.AnchorsFound)
	}
	ctx2 := &Context{Golden: []string{"醒后喝水"}}
	d := Evaluate(S5, ctx2, "x\n[STAGE:SKIP]")
	if d.Action != "skip" || d.Next != S6 {
		t.Errorf("S5 SKIP 应兜底推进: %+v recipes=%v", d, ctx2.Recipes)
	}
	if !strings.HasPrefix(ctx2.Recipes[0].Anchor, "在我") {
		t.Errorf("兜底锚点句式错误: %s", ctx2.Recipes[0].Anchor)
	}
}

// ---------- S9（执行期） ----------

func TestModeUndo(t *testing.T) {
	if d := OnModeUndo(Active); d.Next != S9 || d.Action != "mode_undo" {
		t.Errorf("active 应可进 S9: %+v", d)
	}
	if d := OnModeUndo(S3); d.Action != "hold" {
		t.Error("诊断期不可进 S9")
	}
	// LLM 内容里的 [MODE:UNDO] 仅执行期生效（诊断期拒绖）
	if d := Evaluate(S3, fullCtx(), "x\n[MODE:UNDO]"); d.Action == "mode_undo" || d.Next == S9 {
		t.Errorf("诊断期 MODE:UNDO 应被拒: %+v", d)
	}
}

// ---------- context JSON 往返 ----------

func TestContextRoundTrip(t *testing.T) {
	ctx := fullCtx()
	ctx.Superseded = append(ctx.Superseded, map[string]any{"wish": "旧愿望"})
	got := ParseContext(ctx.JSON())
	if got.Wish != ctx.Wish || len(got.Recipes) != 2 || len(got.Superseded) != 1 {
		t.Errorf("round-trip 丢失: %+v", got)
	}
}
