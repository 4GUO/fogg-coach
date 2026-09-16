#!/usr/bin/env python3
"""fogg-coach T1.7 验收：A 域流程 / B 刁钻输入 / C 防刷配额。
产物：docs/test-transcripts/t17-acceptance.md
用法：python3 acceptance.py [domain|tricky|abuse|all]
"""
import json, os, sqlite3, sys, threading, time, urllib.request, urllib.error

BASE = "http://localhost:8080/api"
DB = os.path.join(os.path.dirname(__file__), "..", "fogg-coach.db")
OUT = []

def log(s=""):
    print(s)
    OUT.append(s)

def post(path, body, token=None, stream=False, timeout=180):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"})
    if token:
        req.add_header("Authorization", "Bearer " + token)
    r = urllib.request.urlopen(req, timeout=timeout)
    if stream:
        events = []
        for raw in r:
            line = raw.decode().strip()
            if line.startswith("data:"):
                try:
                    events.append(json.loads(line[5:].strip()))
                except json.JSONDecodeError:
                    pass
        return events
    return json.loads(r.read())

def login():
    return post("/auth/login", {"provider": "wechat_mp", "code": f"t17-{int(time.time()*1000)}"})

def run_turn(token, sess, content=None, action=None):
    body = {"sessionId": sess}
    if content: body["content"] = content
    if action: body["action"] = action
    ev = post("/chat", body, token, stream=True)
    text = "".join(e.get("delta", "") for e in ev)
    stage, sid = None, sess
    for e in ev:
        if e.get("sessionId"): sid = e["sessionId"]
        if e.get("stage"): stage = e["stage"]
        if e.get("event") == "stage_done": stage = e["next"]
    tail = [l for l in text.splitlines() if l.strip()][-2:]
    for l in tail: log("    🗣 " + l[:88])
    return stage or "-", sid

# ---------- A：域全流程 ----------
DOMAINS = {
    "fitness": {
        "label": "A2 健身域",
        "open": "你好，我想养成运动的习惯，总是不动",
        "confirm": "对，就是想让身体动起来，有点活力",
        "s2": ["体检说缺乏运动，我也想精力好点，不然下午总犯困",
               "以前办过健身卡，卡在下班太累不想去，周末又只想躺着",
               "每天雷打不动：早上刷牙、中午吃午饭、下班到家放包、晚上洗澡"],
        "select": ["刷牙后做2个深蹲", "下班到家后原地开合跳30秒"],
        "s5_times": "早上7点刷牙，晚上7点到家，晚上10点半洗澡",
    },
    "phone": {
        "label": "A3 戒手机域",
        "open": "我想少刷点手机，尤其睡前",
        "confirm": "对，睡前不刷手机，睡个好觉",
        "s2": ["刷到半夜第二天起不来，白天没精神，恶性循环",
               "一躺下就习惯性拿手机，提醒也没用；短视频一开就停不下来",
               "每天必做：闹钟响、刷牙、吃三顿饭、晚上给手机充电"],
        "select": ["睡前手机放客厅充电", "刷牙后做2分钟拉伸"],
        "s5_times": "早上7点闹钟，晚上10点半刷牙，手机10点充电",
    },
}

def run_domain(key):
    d = DOMAINS[key]
    log(f"\n## {d['label']}（{key}）\n")
    tok = login()["token"]
    log("✅ 登录")
    s2_i, stage, sess, s5_i = 0, "S1", None, 0
    for turn in range(18):
        if stage == "S7": break
        content, action = None, None
        if stage == "S1":
            content = d["open"] if turn == 0 else d["confirm"]
        elif stage == "S2":
            if s2_i < len(d["s2"]): content = d["s2"][s2_i]; s2_i += 1
            else: content = "没了，就这些"
        elif stage == "S3": content = "嗯，给我看看有哪些可以选择的"
        elif stage == "S4":
            action = {"type": "select", "options": d["select"]}
            log(f"[S4] select: {d['select']}")
        elif stage == "S5":
            if s5_i == 0: content, s5_i = "帮我配好配方吧，庆祝方式你来定", 1
            elif s5_i == 1: content, s5_i = d["s5_times"], 2
            else: content = "好，我们继续下一步"
        elif stage == "S6":
            action = {"type": "confirm"}; log("[S6] confirm")
        else: log(f"[?] {stage}"); return False
        stage, sess = run_turn(tok, sess, content, action)
        time.sleep(0.2)
    if stage != "S7":
        log(f"❌ {d['label']} 未到 S7（停在 {stage}）"); return False
    plan = post("/plan/generate", {"sessionId": sess}, tok)
    log(f"✅ 计划生成 planId={plan['planId']} habits={len(plan['plan']['habits'])}")
    log("```json\n" + json.dumps(plan["plan"], ensure_ascii=False, indent=1) + "\n```")
    return True

# ---------- B：刁钻输入 ----------
TRICKY = [
    ("B1 跳跃型", "直接给我完整健身计划"),
    ("B2 放弃型", "我就坚持不下来怎么办，试过好多次了"),
    ("B3 自我诊断", "我是不是有拖延症？"),
    ("B4 宏大愿望", "我想一年赚50万，帮我做计划"),
]
FORBIDDEN = ["你要坚持", "自律一点", "克服惰性", "靠毅力", "你这是拖延症", "抑郁症"]

def run_tricky():
    log("\n## B 刁钻输入（S1 首轮）\n")
    ok = True
    for name, msg in TRICKY:
        tok = login()["token"]
        log(f"### {name}: 「{msg}」")
        try:
            ev = post("/chat", {"content": msg}, tok, stream=True)
            text = "".join(e.get("delta", "") for e in ev)
            hit = [w for w in FORBIDDEN if w in text]
            if hit:
                log(f"❌ 命中禁词 {hit}"); ok = False
            else:
                tail = [l for l in text.splitlines() if l.strip()][-2:]
                for l in tail: log("    🗣 " + l[:88])
                log("✅ 无禁词，未崩")
        except urllib.error.HTTPError as ex:
            log(f"❌ HTTP {ex.code}: {ex.read().decode()[:150]}"); ok = False
    return ok

# ---------- C：防刷配额 ----------
def run_abuse():
    log("\n## C 防刷配额\n")
    ok = True
    # C1 未授权（无 token 调 chat → 401）
    try:
        post("/chat", {"content": "x"}, None); code1 = 200
    except urllib.error.HTTPError as ex: code1 = ex.code
    log(f"C1 无 token → {code1} {'✅' if code1 == 401 else '❌'}")
    ok &= code1 == 401
    # C2 高频限流（并发 4 发，第3+ 应 429）
    tok = login()["token"]
    sess = post("/chat", {"content": "你好，我想早睡"}, tok, stream=True)
    sid = next((e.get("sessionId") for e in sess if e.get("sessionId")), None)
    codes = []
    def fire():
        try: post("/chat", {"sessionId": sid, "content": "嗯"}, tok, stream=True, timeout=60); codes.append(200)
        except urllib.error.HTTPError as ex: codes.append(ex.code)
    ts = [threading.Thread(target=fire) for _ in range(4)]
    [t.start() for t in ts]; [t.join() for t in ts]
    hit429 = 429 in codes
    log(f"C2 并发4发 → {sorted(codes)} {'✅触发限流' if hit429 else '❌未触发'}")
    ok &= hit429
    # C3 超长截断（600字 → 落库500；新用户避开冷却）
    tok = login()["token"]
    ev3 = post("/chat", {"content": "我想早睡"}, tok, stream=True)
    sid = next((e.get("sessionId") for e in ev3 if e.get("sessionId")), None)
    long = "我想早睡" * 150
    ev = post("/chat", {"sessionId": sid, "content": long}, tok, stream=True)
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    n = c.execute("SELECT MAX(LENGTH(content)) FROM messages WHERE session_id=?", (sid,)).fetchone()[0]
    log(f"C3 600字输入 → 最长落库 {n} 字 {'✅' if n <= 500 else '❌'}")
    ok &= n <= 500
    # C4 plan 生成 3 次/日（会话置 S7 + 拉满配额 → 应 429 在 LLM 之前拦截）
    c = sqlite3.connect(DB)
    c.execute("UPDATE sessions SET stage='S7' WHERE id=?", (sid,))
    uid = c.execute("SELECT user_id FROM sessions WHERE id=?", (sid,)).fetchone()[0]
    c.execute("INSERT INTO usage (user_id, day, plan_generations) VALUES (?, date('now','localtime'), 3) ON CONFLICT(user_id, day) DO UPDATE SET plan_generations=3", (uid,))
    c.commit(); c.close()
    try:
        post("/plan/generate", {"sessionId": sid}, tok)
        log("❌ C4 未拦截"); ok = False
    except urllib.error.HTTPError as ex:
        log(f"C4 拉满配额后生成 → {ex.code} {'✅' if ex.code == 429 else '❌'}")
        ok &= ex.code == 429
    # C5 25轮兜底强制出计划
    tok2 = login()["token"]
    ev = post("/chat", {"content": "我想早睡"}, tok2, stream=True)
    sid2 = next((e.get("sessionId") for e in ev if e.get("sessionId")), None)
    c = sqlite3.connect(DB)
    import random
    c.executemany("INSERT INTO messages (id, session_id, role, content) VALUES (?,?,?,?)",
                      [(f"fill{random.getrandbits(48):x}{i}", sid2, "user", "嗯") for i in range(26)])
    c.commit(); c.close()
    ev = post("/chat", {"sessionId": sid2, "content": "继续"}, tok2, stream=True)
    forced = any(e.get("forcePlan") for e in ev)
    log(f"C5 26轮后继续 → forcePlan={'✅' if forced else '❌'}")
    ok &= forced
    return ok

def curl_code(path, token):
    try:
        post(path, {}, token); return 200
    except urllib.error.HTTPError as ex:
        return ex.code

def main():
    which = sys.argv[1] if len(sys.argv) > 1 else "all"
    results = {}
    if which in ("fitness", "all"): results["A2健身"] = run_domain("fitness")
    if which in ("phone", "all"): results["A3戒手机"] = run_domain("phone")
    if which in ("tricky", "all"): results["B刁钻"] = run_tricky()
    if which in ("abuse", "all"): results["C防刷"] = run_abuse()
    log("\n## 汇总\n")
    log("| 组 | 结果 |\n|---|---|")
    for k, v in results.items():
        log(f"| {k} | {'✅ PASS' if v else '❌ FAIL'} |")
    outdir = os.path.join(os.path.dirname(__file__), "..", "..", "docs", "test-transcripts")
    os.makedirs(outdir, exist_ok=True)
    path = os.path.join(outdir, f"t17-acceptance-{time.strftime('%Y%m%d-%H%M')}.md")
    with open(path, "w", encoding="utf-8") as f:
        f.write(f"# T1.7 验收记录 {time.strftime('%Y-%m-%d %H:%M')}（GLM-5.2 真机）\n")
        f.write("\n".join(OUT))
    print(f"\n转录已存 {path}")
    sys.exit(0 if all(results.values()) else 1)

if __name__ == "__main__":
    main()
