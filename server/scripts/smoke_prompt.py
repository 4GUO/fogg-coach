#!/usr/bin/env python3
"""fogg-coach prompt 冒烟测试：不依赖后端，直接打 LLM API。

验证三块核心假设：
  A. S1 多轮对话 —— 标记协议（[QUICK:...]）、语气铁律（≤150字/一次一问）
  B. S1 刁钻输入 —— 禁令红线（意志力话术/医疗诊断/跳阶段给大计划）
  C. S7 结构化输出 —— Plan JSON 服务端等价校验

用法: ZAI_KEY=xxx python3 smoke_prompt.py [--model glm-5.2]
产物: docs/test-transcripts/smoke-<ts>.md（人工复核用）
"""
import json, os, re, sys, time, urllib.request

BASE_URL = os.environ.get("ZAI_BASE_URL", "https://open.bigmodel.cn/api/coding/paas/v4")
MODEL = sys.argv[sys.argv.index("--model") + 1] if "--model" in sys.argv else "glm-5.2"
KEY = os.environ["ZAI_KEY"]
PROMPTS = os.path.join(os.path.dirname(__file__), "..", "prompts")

FORBIDDEN = ["意志力", "你要坚持", "自律", "毅力", "克服惰性", "拖延症", "抑郁", "焦虑症", "抑郁症"]

def load(*parts):
    out = []
    for p in parts:
        with open(os.path.join(PROMPTS, p), encoding="utf-8") as f:
            out.append(f.read().strip())
    return "\n\n---\n\n".join(out)

def chat(messages, json_mode=False, max_tokens=1000, thinking=False):
    body = {"model": MODEL, "messages": messages, "temperature": 0.7, "max_tokens": max_tokens,
            "thinking": {"type": "enabled" if thinking else "disabled"}}
    if json_mode:
        body["response_format"] = {"type": "json_object"}
    req = urllib.request.Request(
        BASE_URL.rstrip("/") + "/chat/completions",
        data=json.dumps(body).encode(),
        headers={"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"},
    )
    t0 = time.time()
    with urllib.request.urlopen(req, timeout=120) as r:
        data = json.loads(r.read())
    content = data["choices"][0]["message"]["content"]
    usage = data.get("usage", {})
    return content, usage, round(time.time() - t0, 1)

# ---------- 检查器 ----------
def strip_markers(text):
    return re.sub(r"\[(?:STAGE:[A-Z]+(?:\s+criteria=\"[^\"]*\")?|QUICK:[^\]]*|RESET_WISH|MODE:[A-Z]+)\]", "", text).strip()

def check_common(text, s1_chips=False, stage_done_ok=False):
    """通用铁律检查，返回 issues 列表"""
    issues = []
    body = strip_markers(text)
    n = len(re.sub(r"\s", "", body))
    if n > 160:  # 150字 + 10% 容差
        issues.append(f"超长: {n}字")
    for w in FORBIDDEN:
        if w in body:
            # 允许否定式重构（"不是你毅力差""不靠自律"）
            sent = next((s for s in re.split(r"[。！？!?\n]", body) if w in s), "")
            if not re.search(r"(不是|不靠|并非|无关|别怪|不需要|不用|不催|不逼|抛开|和.*无关)", sent[:sent.index(w) + 4]):
                issues.append(f"禁词「{w}」")
    need_question = not (stage_done_ok and re.search(r"\[STAGE:(DONE|SKIP)", text))
    if need_question and not re.search(r"[?？]|告诉我|选一个|请选", body):
        issues.append("未见提问（一次一问铁律）")
    if need_question:
        q = len(re.findall(r"[?？]", body))
        if q > 2:
            issues.append(f"多问: {q}个问号")
    if re.search(r"^#{1,6}\s|\*\*|^\s*[-*]\s", body, re.M):
        issues.append("含 Markdown 格式")
    # QUICK 标记格式
    for m in re.finditer(r"\[QUICK:([^\]]*)\]", text):
        items = [i.strip() for i in m.group(1).split("|") if i.strip()]
        limit = 7 if s1_chips else 4
        if len(items) < 2: issues.append(f"QUICK 选项过少: {items}")
        if len(items) > limit: issues.append(f"QUICK 超限: {len(items)}>{limit}")
        for i in items:
            if len(i) > 10: issues.append(f"QUICK 选项超长: {i}")
    # STAGE 标记必须独占最后一行
    for m in re.finditer(r"\[STAGE:(DONE|HOLD|SKIP)", text):
        tail = text[m.start():].strip()
        if "\n" in tail and tail.split("\n", 1)[1].strip():
            issues.append("[STAGE:x] 后还有正文")
    return issues

def validate_plan(obj):
    """S7 Plan JSON 等价校验（Go 侧规则）"""
    e = []
    if not isinstance(obj, dict): return ["非对象"]
    wish = obj.get("wish", "")
    if not (0 < len(wish) <= 30): e.append(f"wish {len(wish)}字")
    hs = obj.get("habits")
    if not isinstance(hs, list) or not (1 <= len(hs) <= 3): e.append(f"habits 数量 {hs if not isinstance(hs,list) else len(hs)}")
    else:
        for i, h in enumerate(hs):
            if len(h.get("title", "")) > 12: e.append(f"h{i} title 超长")
            if not h.get("anchor", "").startswith("在我"): e.append(f"h{i} anchor 句式: {h.get('anchor')}")
            b = h.get("behavior", "")
            if not (0 < len(b) <= 20): e.append(f"h{i} behavior {len(b)}字")
            if not re.match(r"^\d{2}:\d{2}$", h.get("anchor_time", "")): e.append(f"h{i} anchor_time 非法")
            if h.get("celebration", "") == "": e.append(f"h{i} 无 celebration")
            pr = h.get("progression", [])
            if len(pr) != 1 or pr[0].get("after_checkins") != 7: e.append(f"h{i} progression 非法: {pr}")
    if obj.get("review_day") not in ["monday","tuesday","wednesday","thursday","friday","saturday","sunday"]:
        e.append(f"review_day: {obj.get('review_day')}")
    if len(obj.get("coach_note", "")) > 60: e.append("coach_note 超长")
    return e

# ---------- 用例 ----------
results, transcript = [], []

def run_case(cid, desc, messages, s1_chips=False, json_mode=False, max_tokens=2500):
    print(f"[{cid}] {desc} ...", flush=True)
    try:
        out, usage, dt = chat(messages, json_mode=json_mode, max_tokens=max_tokens)
    except Exception as ex:
        results.append((cid, desc, False, [f"API失败: {ex}"]))
        transcript.append((cid, desc, messages[-1]["content"], str(ex)))
        return None
    truncated = (usage.get("completion_tokens") or 0) >= max_tokens
    issues = check_common(out, s1_chips=s1_chips, stage_done_ok=True) if not json_mode else []
    if truncated:
        issues.append(f"疑似截断(completion={usage.get('completion_tokens')}>=max_tokens)")
    results.append((cid, desc, not issues, issues))
    transcript.append((cid, desc, messages[-1]["content"], out))
    print(f"    {'✅ PASS' if not issues else '❌ ' + '; '.join(issues)}  ({dt}s, in={usage.get('prompt_tokens')} out={usage.get('completion_tokens')})")
    return out

def main():
    base_s1 = [{"role": "system", "content": load("base.md", "stages/S1.md")}]
    base_s2 = [{"role": "system", "content": load("base.md", "stages/S2.md")}]
    ctx = {"wish": "早上不那么赖床", "motivation": "上班总迟到很焦虑",
           "ability_gaps": ["闹钟响后起不来按掉", "睡前刷手机到凌晨"],
           "anchors_found": ["闹钟响后", "刷牙后", "烧水时"],
           "candidates": ["醒后拉开窗帘", "闹钟放客厅充电", "睡前手机放客厅", "醒后喝一杯水", "睡前做2分钟拉伸", "醒后叠被子"],
           "golden": ["睡前手机放客厅", "醒后拉开窗帘"],
           "s5_recipes": [
               {"behavior": "睡前把手机放到客厅充电", "anchor": "在我刷完牙之后", "celebration": "握拳说Yes"},
               {"behavior": "醒后拉开窗帘", "anchor": "在我关掉闹钟之后", "celebration": "心里夸自己一句"}],
           "coach_note_s6": "计划是实验，试试看，不合适我们随时调"}
    base_s7 = [{"role": "system", "content": load("base.md", "stages/S7.md")},
               {"role": "user", "content": "context:\n" + json.dumps(ctx, ensure_ascii=False, indent=1)}]

    # A. S1 正常多轮
    msgs = list(base_s1)
    r1 = run_case("A1", "S1 开场", msgs + [{"role": "user", "content": "你好"}], s1_chips=True)
    if r1:
        msgs += [{"role": "assistant", "content": r1}, {"role": "user", "content": "最近总是睡不醒，白天没精神"}]
        r2 = run_case("A2", "S1 具体化愿望", msgs, s1_chips=True)
        if r2:
            msgs += [{"role": "assistant", "content": r2}, {"role": "user", "content": "对，就是想早上起来有精神一点"}]
            run_case("A3", "S1 愿望确认（应出 [STAGE:DONE]）", msgs, s1_chips=True)

    # B. 刁钻输入（S2 环境，更易暴露跳阶段/诊断）
    for bid, inp in [("B1", "直接给我完整健身计划"), ("B2", "我就坚持不下来怎么办"), ("B3", "我是不是有拖延症")]:
        run_case(bid, f"刁钻: {inp[:12]}", base_s2 + [{"role": "user", "content": inp}])

    # C. S7 JSON
    print("[C1] S7 Plan JSON 生成 ...", flush=True)
    try:
        out, usage, dt = chat(base_s7, json_mode=True, max_tokens=4000, thinking=True)
        issues = []
        if (usage.get("completion_tokens") or 0) >= 4000:
            issues.append(f"疑似截断(completion={usage.get('completion_tokens')})")
        try:
            obj = json.loads(re.sub(r"^```(json)?|```$", "", out.strip(), flags=re.M).strip())
            issues += validate_plan(obj)
            golden = ctx["golden"]
            if isinstance(obj.get("habits"), list) and len(obj["habits"]) != len(golden):
                issues.append(f"habits 数({len(obj['habits'])}) != golden 数({len(golden)})")
        except json.JSONDecodeError as ex:
            issues.append(f"JSON 解析失败: {ex}")
            obj = None
        results.append(("C1", "S7 Plan JSON", not issues, issues))
        transcript.append(("C1", "S7 Plan JSON", "(context 注入)", out))
        print(f"    {'✅ PASS' if not issues else '❌ ' + '; '.join(issues)}  ({dt}s, in={usage.get('prompt_tokens')} out={usage.get('completion_tokens')})")
    except Exception as ex:
        results.append(("C1", "S7 Plan JSON", False, [f"API失败: {ex}"]))

    # 汇总
    print("\n" + "=" * 56)
    ok = sum(1 for r in results if r[2])
    print(f"结果: {ok}/{len(results)} PASS  (model={MODEL})")
    for cid, desc, passed, issues in results:
        print(f"  {'✅' if passed else '❌'} [{cid}] {desc}" + ("" if passed else f" → {'; '.join(issues)}"))

    # 转录存档
    outdir = os.path.join(os.path.dirname(__file__), "..", "..", "docs", "test-transcripts")
    os.makedirs(outdir, exist_ok=True)
    ts = time.strftime("%Y%m%d-%H%M")
    path = os.path.join(outdir, f"smoke-{MODEL}-{ts}.md")
    with open(path, "w", encoding="utf-8") as f:
        f.write(f"# Prompt 冒烟 {ts} (model={MODEL})\n\n")
        f.write("| 用例 | 结果 | 问题 |\n|---|---|---|\n")
        for cid, desc, passed, issues in results:
            f.write(f"| {cid} {desc} | {'PASS' if passed else 'FAIL'} | {'; '.join(issues) or '-'} |\n")
        for cid, desc, inp, out in transcript:
            f.write(f"\n## [{cid}] {desc}\n\n**用户:** {inp}\n\n**教练输出:**\n\n```\n{out}\n```\n")
    print(f"转录: {path}")

if __name__ == "__main__":
    main()
