#!/usr/bin/env python3
"""fogg-coach E2E 冒烟 v2：阶段感知状态机，作息域 S1→S7 全流程（真 LLM）。"""
import json, sys, time, urllib.request, urllib.error

BASE = "http://localhost:8080/api"

def post(path, body, token=None, stream=False):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"})
    if token:
        req.add_header("Authorization", "Bearer " + token)
    r = urllib.request.urlopen(req, timeout=180)
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

def run_turn(token, sess, content=None, action=None):
    body = {"sessionId": sess}
    if content: body["content"] = content
    if action: body["action"] = action
    ev = post("/chat", body, token, stream=True)
    text = "".join(e.get("delta", "") for e in ev)
    stage = None
    for e in ev:
        if e.get("sessionId"): globals()["SESS"] = e["sessionId"]
        if e.get("stage"): stage = e["stage"]
        if e.get("event") == "stage_done":
            stage = e["next"]
            print(f"    ⏩ → {e['next']}")
    tail = [l for l in text.splitlines() if l.strip() and not l.startswith("[")][-2:]
    for l in tail: print("    🗣", l[:90])
    return stage

def main():
    login = post("/auth/login", {"provider": "wechat_mp", "code": f"e2e-{int(time.time())}"})
    token = login["token"]
    print("✅ 登录", login["user"]["userId"])

    s2_script = ["做到了早上不赖床，上班就不会迟到，我也没那么焦虑了",
                 "以前试过早睡早起，卡在睡前刷手机停不下来，早上闹钟响了又按掉起不来",
                 "每天雷打不动会做的：闹钟响后关闹钟、刷牙、中午吃午饭，晚上烧水"]
    s2_i, stage, sess = 0, "S1", None
    opening = "你好，最近总是睡不醒，想早上起来有精神一点"

    for turn in range(16):
        if stage == "S7": break
        content, action = None, None
        if stage == "S1":
            content = opening if turn == 0 else "对，就是这个愿望，帮我搞定它"
        elif stage == "S2":
            if s2_i < len(s2_script): content = s2_script[s2_i]; s2_i += 1
            else: content = "没有了，都告诉你了"
        elif stage == "S3":
            content = "嗯，给我看看有哪些可以选择的行为"
        elif stage == "S4":
            action = {"type": "select", "options": ["醒后拉开窗帘", "睡前手机放客厅充电"]}
            print("[S4] 按钮 select ×2")
        elif stage == "S5":
            if not globals().get("S5_ASKED"):
                globals()["S5_ASKED"] = True
                content = "帮我配好配方吧，庆祝方式你来定"
            elif not globals().get("S5_TOLD"):
                globals()["S5_TOLD"] = True
                content = "晚上10点半左右刷牙，早上7点闹钟，中午12点午饭"
            else:
                content = "好，那我们继续下一步吧"
        elif stage == "S6":
            action = {"type": "confirm"}
            print("[S6] 按钮 confirm")
        else:
            print(f"[?] 未知阶段 {stage}"); sys.exit(1)
        if turn == 0 and sess is None:
            pass
        print(f"— turn{turn} stage={stage}")
        new_stage = run_turn(token, sess, content, action)
        sess = globals().get("SESS") or sess
        if new_stage: stage = new_stage
        # 从事件中拿 sessionId（首个 done 事件带）
        # （简化：每次 run_turn 后由服务端保持同一 session）
        time.sleep(0.2)

    # sessionId 通过 409 恢复：直接再开 chat 空 content 会拿到现有 session 的 409
    if sess is None:
        try:
            post("/chat", {"sessionId": "", "content": "继续"}, token)
        except urllib.error.HTTPError as ex:
            if ex.code == 409:
                sess = json.loads(ex.read())["sessionId"]
    print(f"📍 最终阶段 {stage} session={sess}")

    if stage != "S7":
        print("❌ 未到 S7"); sys.exit(1)
    print("[S7] POST /plan/generate")
    try:
        plan = post("/plan/generate", {"sessionId": sess}, token)
        print("✅ 计划生成成功 planId=" + plan["planId"])
        print(json.dumps(plan["plan"], ensure_ascii=False, indent=1))
    except urllib.error.HTTPError as ex:
        print(f"❌ 生成失败 {ex.code}: {ex.read().decode()[:300]}")
        sys.exit(1)

if __name__ == "__main__":
    main()
