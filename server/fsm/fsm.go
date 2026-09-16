// Package fsm 会话状态机（system-design §3.1）——全项目主轴。
//
// 铁律：阶段推进权在后端，绝不信任 LLM 自评（三道防线的第 2 道）。
// LLM 只能在回复末尾输出标记，服务端校验 context 齐全度后按白名单顺序推进。
package fsm

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// ---------- 阶段常量 ----------

type Stage string

const (
	S1      Stage = "S1" // 探索愿望
	S2      Stage = "S2" // 澄清现状
	S3      Stage = "S3" // 头脑风暴
	S4      Stage = "S4" // 焦点地图（按钮事件）
	S5      Stage = "S5" // 配方设计
	S6      Stage = "S6" // 确认（按钮事件）
	S7      Stage = "S7" // 生成 Plan JSON（后端触发）
	S8      Stage = "S8" // 复盘（执行期）
	S9      Stage = "S9" // 解坏习惯（用户主动）
	Active  Stage = "active"
)

// 顺序白名单：诊断漏斗唯一推进路径，禁跳步
var order = []Stage{S1, S2, S3, S4, S5, S6, S7}

func nextOf(s Stage) (Stage, bool) {
	for i, st := range order {
		if st == s && i+1 < len(order) {
			return order[i+1], true
		}
	}
	return "", false
}

// ---------- 会话 context（sessions.context JSON） ----------

type Recipe struct {
	Behavior    string `json:"behavior"`
	Anchor      string `json:"anchor"`      // 「在我X之后」句式
	Celebration string `json:"celebration"`
	AnchorTime  string `json:"anchor_time,omitempty"` // S5.md 推进条件：提醒时间（原文，S7 归一化 HH:mm）
}

type Context struct {
	Wish         string   `json:"wish,omitempty"`          // ≤30字
	Motivation   string   `json:"motivation,omitempty"`
	AbilityGaps  []string `json:"ability_gaps,omitempty"`  // ≥2
	AnchorsFound []string `json:"anchors_found,omitempty"` // ≥2（prompt 争取3）
	Candidates   []string `json:"candidates,omitempty"`    // ≥5
	Golden       []string `json:"golden,omitempty"`        // 1-3（S4 按钮）
	Recipes      []Recipe `json:"s5_recipes,omitempty"`
	ReviseCount  int      `json:"revise_count,omitempty"`  // S6→S5 回退计数（>2 锁 S6）
	Superseded   []map[string]any `json:"superseded,omitempty"` // RESET_WISH 旧数据保留（§3.1.5）
}

func ParseContext(raw string) *Context {
	ctx := &Context{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), ctx)
	}
	return ctx
}

func (c *Context) JSON() string {
	b, _ := json.Marshal(c)
	return string(b)
}

// ---------- 标记解析（§3.1.3，2026-09-16 方括号协议） ----------

var (
	reStage   = regexp.MustCompile(`\[STAGE:(DONE|HOLD|SKIP)(?:\s+criteria="([^"]*)")?\]`)
	reQuick   = regexp.MustCompile(`\[QUICK:([^\]]+)\]`)
	reReset   = regexp.MustCompile(`\[RESET_WISH\]`)
	reUndo    = regexp.MustCompile(`\[MODE:UNDO\]`)
)

type Markers struct {
	Stage       string // DONE | HOLD | SKIP | ""（无标记）
	Criteria    string
	QuickReplies []string
	ResetWish   bool
	ModeUndo    bool
	Valid       bool // 标记是否位于输出末尾（否则视为噪音，按 HOLD）
}

// 标记必须位于输出末尾：只检查最后 2 个非空行（防线：正文出现标记 = 注入噪音）
func ExtractMarkers(content string) Markers {
	m := Markers{}
	lines := strings.Split(content, "\n")
	var tail []string
	for i := len(lines) - 1; i >= 0 && len(tail) < 2; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			tail = append([]string{lines[i]}, tail...)
		}
	}
	tailBlock := strings.Join(tail, "\n")

	if sm := reStage.FindStringSubmatch(tailBlock); sm != nil {
		m.Stage, m.Criteria, m.Valid = sm[1], sm[2], true
	}
	if q := reQuick.FindStringSubmatch(tailBlock); q != nil {
		for _, item := range strings.Split(q[1], "|") {
			if s := strings.TrimSpace(item); s != "" {
				m.QuickReplies = append(m.QuickReplies, s)
			}
		}
	}
	if reReset.MatchString(tailBlock) {
		m.ResetWish, m.Valid = true, true
	}
	if reUndo.MatchString(tailBlock) {
		m.ModeUndo, m.Valid = true, true
	}
	return m
}

// StripMarkers 去掉尾部标记，返回用户可见正文
func StripMarkers(content string) string {
	s := reStage.ReplaceAllString(content, "")
	s = reQuick.ReplaceAllString(s, "")
	s = reReset.ReplaceAllString(s, "")
	s = reUndo.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// ClipQuickReplies chips 上限（常规 ≤4，S1 愿望域特例 ≤7；每项 ≤8 字）
func ClipQuickReplies(items []string, stage Stage) []string {
	limit := 4
	if stage == S1 {
		limit = 7
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		r := []rune(it)
		if len(r) > 8 {
			r = r[:8]
		}
		out = append(out, string(r))
	}
	return out
}

// ---------- 推进条件（§3.1.2 硬条件） ----------

// missing 返回当前阶段推进到下一阶段所缺的 context 字段（空 = 可推进）
func missing(stage Stage, ctx *Context) []string {
	var lack []string
	switch stage {
	case S1:
		if strings.TrimSpace(ctx.Wish) == "" {
			lack = append(lack, "wish")
		} else if len([]rune(ctx.Wish)) > 30 {
			lack = append(lack, "wish>30字")
		}
	case S2:
		if strings.TrimSpace(ctx.Motivation) == "" {
			lack = append(lack, "motivation")
		}
		if len(ctx.AbilityGaps) < 2 {
			lack = append(lack, "ability_gaps<2")
		}
		if len(ctx.AnchorsFound) < 2 { // 硬条件 ≥2，prompt 争取 3
			lack = append(lack, "anchors_found<2")
		}
	case S3:
		if len(ctx.Candidates) < 5 {
			lack = append(lack, "candidates<5")
		}
	case S4:
		if len(ctx.Golden) < 1 {
			lack = append(lack, "golden未选")
		} else if len(ctx.Golden) > 3 {
			lack = append(lack, "golden>3")
		}
	case S5:
		if len(ctx.Recipes) != len(ctx.Golden) {
			lack = append(lack, "recipes数量不齐")
			break
		}
		for i, r := range ctx.Recipes {
			if r.Anchor == "" || r.Behavior == "" || r.Celebration == "" {
				lack = append(lack, "recipe三元组缺项")
				break
			}
			if r.AnchorTime == "" {
				lack = append(lack, "recipe缺anchor_time")
				break
			}
			if !strings.HasPrefix(r.Anchor, "在我") {
				lack = append(lack, "recipe锚点句式")
			}
			_ = i
		}
	}
	return lack
}

// ---------- 决策 ----------

type Decision struct {
	Next        Stage  // 结果阶段（HOLD 时 = 当前阶段）
	Action      string // advance | hold | skip | reset_wish | mode_undo
	Reason      string // 未推进原因（日志/调试用）
	QuickReplies []string
}

// Evaluate 处理一轮 LLM 输出：标记解析 → 条件校验 → 白名单推进
// S4/S6 的按钮事件不走这里（见 OnSelect/OnConfirm），LLM 无权自推 S7。
func Evaluate(current Stage, ctx *Context, llmContent string) Decision {
	d := Decision{Next: current, Action: "hold"}
	m := ExtractMarkers(llmContent)
	d.QuickReplies = ClipQuickReplies(m.QuickReplies, current)

	if m.ResetWish {
		return OnResetWish(current, ctx)
	}
	if m.ModeUndo {
		return OnModeUndo(current)
	}
	if !m.Valid || m.Stage == "" || m.Stage == "HOLD" {
		if !m.Valid && m.Stage == "" && len(m.QuickReplies) == 0 && !m.ResetWish && !m.ModeUndo {
			d.Reason = "无有效标记（按 HOLD）"
		}
		return d
	}

	// STAGE:DONE / SKIP：校验当前阶段 context 硬条件
	// 防线：S4 选择、S6 确认走按钮事件，LLM 无权推进（§3.1.3）
	if current == S4 || current == S6 {
		d.Reason = "当前阶段推进走按钮事件，LLM 标记不生效"
		return d
	}
	lack := missing(current, ctx)
	switch {
	case m.Stage == "DONE" && len(lack) > 0:
		d.Reason = "标记 DONE 但 context 缺: " + strings.Join(lack, ",")
		return d // 关键防线：LLM 说了不算，缺字段不推进
	case m.Stage == "SKIP":
		ApplySkipDefaults(current, ctx) // 默认值兜底（不破坏顺序）
		if lack2 := missing(current, ctx); len(lack2) > 0 {
			d.Reason = "SKIP 兜底后仍缺: " + strings.Join(lack2, ",")
			return d
		}
	}
	nxt, ok := nextOf(current)
	if !ok {
		d.Reason = "当前阶段无顺序后继（S7 由后端主动触发）"
		return d
	}
	d.Next, d.Action = nxt, "advance"
	if m.Stage == "SKIP" {
		d.Action = "skip"
	}
	return d
}

// ---------- 按钮事件（绕过 LLM，§3.1.3 第 3 道防线） ----------

var ErrNotInStage = errors.New("当前阶段不允许此操作")

// OnSelect S4 用户选定黄金行为（1-3 个）
func OnSelect(current Stage, ctx *Context, golden []string) Decision {
	d := Decision{Next: current, Action: "hold"}
	if current != S4 {
		d.Reason = ErrNotInStage.Error()
		return d
	}
	ctx.Golden = golden
	if lack := missing(S4, ctx); len(lack) > 0 {
		d.Reason = strings.Join(lack, ",")
		return d
	}
	return Decision{Next: S5, Action: "advance"}
}

// OnConfirm S6 用户点确认 → 后端触发 S7（LLM 无权自推）
func OnConfirm(current Stage) Decision {
	if current != S6 {
		return Decision{Next: current, Action: "hold", Reason: ErrNotInStage.Error()}
	}
	return Decision{Next: S7, Action: "advance"}
}

// OnRevise S6「再改改」→ 回退 S5；>2 次锁定 S6（§3.1.5）
func OnRevise(current Stage, ctx *Context) Decision {
	d := Decision{Next: current, Action: "hold"}
	if current != S6 {
		d.Reason = ErrNotInStage.Error()
		return d
	}
	if ctx.ReviseCount >= 2 {
		d.Reason = "S6→S5 回退已达 2 次，锁定 S6（只许确认或放弃）"
		return d
	}
	ctx.ReviseCount++
	return Decision{Next: S5, Action: "hold", Reason: "revise"}
}

// OnResetWish 改愿望：回退 S2（不重走 S1），每 session ≤2 次（§3.1.5）
// 旧 context 标 superseded 保留，可复用已摸清的动机/锚点。
func OnResetWish(current Stage, ctx *Context) Decision {
	d := Decision{Next: current, Action: "hold"}
	if current == S1 || current == S7 || current == Active {
		d.Reason = "当前阶段不可 RESET_WISH"
		return d
	}
	if ctx.Superseded == nil {
		ctx.Superseded = []map[string]any{}
	}
	if old, err := json.Marshal(ctx); err == nil {
		var m map[string]any
		_ = json.Unmarshal(old, &m)
		m["superseded"] = nil
		ctx.Superseded = append(ctx.Superseded, m)
	}
	// 保留可复用底料：motivation/anchors 与愿望无关的部分保留，其余清空
	ctx.Wish, ctx.Candidates, ctx.Golden, ctx.Recipes = "", nil, nil, nil
	return Decision{Next: S2, Action: "reset_wish"}
}

// OnModeUndo S9 解坏习惯：仅用户主动（active 期表达"想戒掉XX"）
func OnModeUndo(current Stage) Decision {
	if current != Active {
		return Decision{Next: current, Action: "hold", Reason: "仅执行期可进入 S9"}
	}
	return Decision{Next: S9, Action: "mode_undo"}
}

// OnPlanGenerated S7 Plan 校验通过入库 → active
func OnPlanGenerated() Decision {
	return Decision{Next: Active, Action: "advance"}
}

// ---------- SKIP 默认值兜底（信息不足用默认值补，永远让用户拿到计划离开） ----------

var (
	defaultAnchors = []string{"早上闹钟响后", "刷牙之后", "午饭后", "晚上洗漱后"}
	defaultAnchorActions = []string{"关掉早上的闹钟", "刷牙", "吃午饭", "晚上洗漱"} // S5 配方用（拼「在我X之后」）
	defaultGaps    = []string{"想不起来做", "和日常安排对不上"}
	defaultCandidates = []string{ // S3 通用候选集（通用微行为，30秒内零决策）
		"醒后拉开窗帘", "闹钟放客厅充电", "睡前手机放客厅",
		"醒后喝一杯水", "刷牙后做2个深蹲", "睡前做2分钟拉伸", "午饭后靠墙站30秒",
	}
	defaultCelebrations = []string{"握拳说Yes", "心里夸自己一句", "微笑一下"}
	defaultTimes = []string{"08:00", "22:00", "12:30", "21:00"}
)

// ApplySkipDefaults 用户明确要求跳过 → 默认值兜底（不破坏顺序）
func ApplySkipDefaults(stage Stage, ctx *Context) {
	switch stage {
	case S1:
		if ctx.Wish == "" {
			ctx.Wish = "让生活更有精神一点"
		}
	case S2:
		if ctx.Motivation == "" {
			ctx.Motivation = "想把状态调好一点"
		}
		if len(ctx.AbilityGaps) < 2 {
			ctx.AbilityGaps = append(ctx.AbilityGaps, defaultGaps[:2-len(ctx.AbilityGaps)]...)
		}
		if len(ctx.AnchorsFound) < 2 {
			ctx.AnchorsFound = append(ctx.AnchorsFound, defaultAnchors[:2-len(ctx.AnchorsFound)]...)
		}
	case S3:
		if len(ctx.Candidates) < 5 {
			ctx.Candidates = defaultCandidates
		}
	case S5:
		if len(ctx.Recipes) != len(ctx.Golden) {
			ctx.Recipes = nil
			for i, g := range ctx.Golden {
				ctx.Recipes = append(ctx.Recipes, Recipe{
					Behavior: g, Anchor: "在我" + defaultAnchorActions[i%len(defaultAnchorActions)] + "之后",
					Celebration: defaultCelebrations[i%len(defaultCelebrations)],
					AnchorTime: defaultTimes[i%len(defaultTimes)],
				})
			}
		}
	}
}

// CanAdvance 后端复评（铁律的正面应用）：S1/S2/S3/S5 的 context 硬条件已齐
// 即可推进，无需等待 LLM 输出 [STAGE:DONE]（LLM 忘标兜底）。
// S4/S6 仍 exclusively 按钮事件；S7 后端触发。
func CanAdvance(stage Stage, ctx *Context) bool {
	if stage != S1 && stage != S2 && stage != S3 && stage != S5 {
		return false
	}
	return len(missing(stage, ctx)) == 0
}

// CanAdvanceTo 返回顺序后继（供后端复评）
func CanAdvanceTo(stage Stage) Stage {
	if n, ok := nextOf(stage); ok {
		return n
	}
	return ""
}
