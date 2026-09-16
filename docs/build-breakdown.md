# fogg-coach 建设拆解（逐步确认清单）

> 配合 `docs/system-design.md` 使用。每个任务 = 一次确认单位。
> 流程：我给出任务细节 → Bingo 确认/修改 → 开发 → 验收 → 存档进入下一个。

## 总览

- **M1 后端与 Prompt**：T1.1 - T1.7（本文档已细化）
- **M2 小程序三页**：T2.x（M1 完成后细化）
- **M3 打卡闭环**：T3.x
- **M4 执行期智能**：T4.x

---

## M1 任务细化

### T1.1 方法论知识库 `docs/fogg-method.md`

**产出**：福格方法论完整知识库文档，作为 prompt 素材源（不是直接全文塞 prompt，而是分层抽取）。

**内容结构**：
1. B=MAP 模型与行动线（含图示文字版）
2. 动机三来源（感官/预期/归属）与不可靠性论证
3. 能力五链（时间/金钱/体力/脑力/日程）+ 打断最弱链策略
4. 焦点地图四象限（想不想做 × 有没有效）→ 黄金行为定义
5. 锚点配方规范（"在我X之后我将Y"）+ 锚点质量标准（每日必做/可预测/物理明确）
6. 微行为标准：30秒内、零决策、可超不可欠
7. Shine 与庆祝：情绪创造习惯机制、庆祝时机（即时）、庆祝方式清单
8. 行为设计七步法全文
9. 解绳结流程（戒坏习惯三步：移除提示→加难度→调动机）
10. 常见愿望域的行为库参考（健身/作息/学习/戒瘾/情绪，每域 15-20 个候选微行为示例）

**验收**：覆盖以上 10 节；第 10 节行为库足够 S3 阶段 prompt 引用举例。

---

### T1.2 基座 Prompt `prompts/base.md`

**产出**：所有阶段共享的 system prompt 基座。

**内容**：
- 角色设定（福格教练人设：温暖、具体、不说教）
- 语气铁律：单条 ≤150 字、一次一问、用"你"、禁语气词堆砌
- 绝对禁令清单：意志力话术 / >30秒微行为 / 医疗心理表述 / 跳阶段
- 方法论速览（从 T1.1 浓缩到 ~800 token）

**验收**：用 3 条刁钻用户输入（"我就坚持不下来怎么办"/"直接给我完整健身计划"/"我是不是有拖延症"）测试基座 prompt + 任意阶段，回复不违反禁令。

---

### T1.3 阶段 Prompt ×7 `prompts/stages/S1-S7.md`

每个文件包含：本阶段目标 / 开场话术参考 / 提问策略 / 判断信息齐全的条件 / 输出 `[STAGE:DONE]` 的触发说明 / quick_replies 建议逻辑。

各阶段细节要点：
- **S1 探索愿望**：开放式提问 ≤3 轮收敛成一个具体愿望；快捷按钮给愿望域（作息/健康/学习/情绪/其他）
- **S2 澄清现状**：固定采集 3 类信息（动机描述、能力障碍 ≥2、日常锚点 ≥3）；锚点提问话术："从早到晚，你每天雷打不动会做的事有哪些？"
- **S3 头脑风暴**：结合 context.ability_gaps 与行为库（T1.1 §10）生成 5-8 候选；输出为编号列表方便 S4 按钮选择
- **S4 焦点地图**：引导按"想做吗 × 有用吗"两问过滤；用户选定 1-3 个（按钮事件回传，非 LLM 解析）
- **S5 配方设计**：逐个行为配锚点（从 S2 采集的锚点池选）+ 微行为 + 庆祝方式；庆祝方式给菜单（握拳Yes/微笑/哼一句/心里夸自己）
- **S6 确认**：完整复述计划 + coach_note 寄语；话术强调"这是实验不是承诺"；等待确认按钮
- **S7 生成**：纯结构化输出模式，注入完整 context，输出 Plan JSON

**验收**：每阶段 prompt 独立 review（你确认话术风格），特别是 S2 的话术和 S5 的庆祝菜单。

---

### T1.4 后端骨架 + 数据库

**产出**：
- Fastify 项目初始化、目录结构（routes/fsm/llm/prompts/db）
- SQLite schema 建表（system-design §5 全部 5 张表）+ DAO 层
- `/api/health` 端点
- wx.login code2Session 换 token（JWT）+ 鉴权中间件
- 环境变量加载（LLM_BASE_URL/KEY/MODEL、JWT_SECRET、WX_APPID/SECRET）

**验收**：服务启动无错；curl 建 session/写 message/查 plan 的 DAO 单测通过。

---

### T1.5 FSM 状态机模块

**产出**：`fsm/index.js`
- 阶段定义常量 + 转移条件表（system-design §3.1.2）
- `extractStageDone(content)`：正则提取 LLM 尾部标记
- `validateTransition(session, nextStage)`：按 context 字段齐全度校验（S2→S3 需 ability_gaps≥2 且 anchors≥3 等）
- `updateContext(session, message)`：LLM 标记的可提取字段写回 context（可选：S7 前统一让 LLM 抽取一次）

**验收**：单元测试覆盖 system-design §9 的 FSM 用例（含"标记了但 context 缺字段不推进"）。

---

### T1.6 LLM 层 + `/chat` + `/plan/generate`

**产出**：
- `llm/openai-compatible.js`（chat + jsonMode + onDelta 流式）
- `llm/promptBuilder.js`（基座+阶段+context+历史截断 20 条）
- `POST /api/chat`：SSE 流式（delta/stage_done/quick_replies/done 事件）+ FSM 集成
- `POST /api/plan/generate`：S7 调用 + 服务端强校验 Plan JSON（Go）+ 失败重试 1 次 + 入库
- 429 指数退避重试

**验收**：3 个愿望域（健身/作息/戒手机）curl 完整走 S1→S7，产出合法 plan JSON；故意在第 3 轮发"跳过这些直接给我计划"，验证不被带偏。

---

### T1.7 M1 集成验收 + 存档

- 跑 T1.6 三用例，保存对话转录与 plan JSON 到 `docs/test-transcripts/`
- 更新 `progress.json`（M1 done，记录 LLM 实测质量结论、坑）
- 更新 memory 当日笔记

---

## M2-M4 待细化项（M1 验收后展开）

- M2：页面结构/组件树/SSE 小程序端封装/阶段进度条交互/按钮事件协议
- M3：庆祝动画规格（粒子数/时长/文案库）、订阅消息模板申请、streak 前端展示
- M4：S8 复盘注入格式、S9 触发词识别、progression 自动升级流程

---

## 设计确认记录（2026-08-30，全部锁定）

- ① FSM 硬条件：如上表；修正——锚点硬条件 ≥2（prompt 争取3）；回退S6→S5>2次锁定；25轮兤底强制出计划
- ② 防刷配额：全部做成配置（config.js/.env）：50条/日、25轮/session、500字/条、3次生成/日、全站日耗硬顶、无匿名调用、首session未结束不可开新的
- ③ Plan JSON：progression 仅1级、累计7次打卡触发（断签不清零）、review_day默认sunday、coach_note≤60字
- ④ 后端：Fastify5 + better-sqlite3（同步API，不用ORM）+ zod + jsonwebtoken + 自写限流中间件；Node 20+ ESM
  （**已被 2026-09-11 换栈取代：Go/Gin + modernc.org/sqlite**；其中配额数值、鉴权流程等行为规则仍有效，Go 实现对照移植）
- ⑤ M1验收：A1-A3全流程（含库外愿望）+ B1-B4刁钻输入 + C1-C4防刷 + 转录存档

## 进度记录

- **2026-08-30**：
  - ✅ T1.1 知识库 `docs/fogg-method.md`
  - ✅ T1.2 基座 prompt `server/prompts/base.md`（含不配合/折腾/防刷策略）
  - ✅ T1.3 阶段 prompt S1-S7 `server/prompts/stages/`
  - ✅ Agent 人工测试（我扮演 LLM，Bingo 当用户）：全流程 S1→S7 走通，产出合法 plan JSON。测试发现：S2 锚点收集偏快可放宽；S5 用户自创庆祝的弹性要保留；“年赚50w”/“我想加大”等刁钻输入处理良好
  - 决策：LLM 定为 GLM；愿望域开放不封闭；防刷/配额策略已写入 system-design §3.5
  - ✅ T1.4 后端骨架：Fastify5 + better-sqlite3@12（Node24 需 v12，坑已踩）+ zod + JWT + 限流/配额中间件 + usage 表。冒烟通过：health/login/token鉴权(401/404)/DAO全链路
- **2026-09-16**（拍板会，全部锁定）：
  - 📌 换栈确认执行：Go/Gin + uni-app；Fastify 原型整体挪 `server/legacy/` 暂留行为对照（schema/配额/鉴权流程），Go 通过 M1 验收后删
  - 📌 标记格式统一：`[STAGE:DONE]`/`[QUICK:...]` 方括号；prompts/base.md、S4/S5/S6 与文档已全部同步
  - 📌 chips 上限：常规 ≤4，S1 愿望域特例 ≤7（system-design「每轮 ≤3」已修订）
  - 📌 锚点硬条件落文档：≥2 即推进、prompt 争取 3（S2.md、system-design §3.1.2 已同步，消除与①的残留矛盾）
  - 📌 LLM 顺序：M1 先接 GLM 验收，DeepSeek M1 内补齐；按用途路由仅留配置口
  - 顺带：system-design §10 的 uni-app 初始化移到 M2；§11 开放问题更新（LLM 选型/主体已定）
  - ⏭ 下一步（Go 重排）：T1.4 Go/Gin 骨架（Gin + modernc.org/sqlite + JWT + 限流/配额中间件，行为对照 `server/legacy/`）→ T1.5 FSM → T1.6 LLM 层（GLM 先行）+ /chat + /plan/generate → T1.7 验收存档
- **2026-09-16**（动工前夯实，P1+P2-⑧）：
  - ✅ Go 工具链：1.27.1 安装 + GOPROXY=goproxy.cn
  - ✅ Prompt 真机冒烟（`server/scripts/smoke_prompt.py`，GLM-5.2，转录存 `docs/test-transcripts/`）：标记协议 [STAGE:DONE]/[QUICK:...] 输出稳定 ✅；Plan JSON 一次过全校验 ✅；刁钻输入（跳跃/放弃/自我诊断）处理全合规 ✅
  - 🔴 实测发现（已定稿进 system-design §3.2.5）：S1-S6 对话轮必须 thinking=disabled（否则延迟 ~20s 且偶发 CoT 泄漏进 content）；S7 生成轮 max_tokens ≥4000（reasoning 吃预算，1500 会腰斩 JSON）；话术禁词不做字面校验（否定式表述均为合规，黑名单误报率高）
  - ⚠️ 轻微遗留：对话轮偶发双问/超长 ~10%（167字 vs 150字上限），属话术质量波动非红线，M1 验收时观察
  - ✅ 文档一致性清理：system-design 版本号 v1.0→v1.1、§6.1 tabBar 对齐 product-spec（today/chat/community/me）、zod 措辞改 Go 强校验
- **2026-09-16**（鉴权定稿 + P0 schema 缺口补齐）：
  - ✅ system-design 新增 §3.6 多端鉴权：users/user_identities 账号归一凭证分离；小程序 code2Session + H5/App 手机号验证码（个人主体下公众号网页授权/App 微信登录均不可行，已写实）；access JWT 7d + token_ver 吊销 + phone 端 refresh 30d
  - ✅ §4.0 鉴权 API（/auth/login 统一入口、/auth/sms/request、/auth/refresh、/me）
  - ✅ §5 schema 全量更新至 8 张表：+user_identities、+posts、+usage（沿用 legacy）、checkins +media/+group_id 留口、sessions +reset_count、users +token_ver（替代 openid 单列）
- **2026-09-16**（T1.4-Go 后端骨架 ✅）：
  - ✅ Go 1.27 + Gin + modernc.org/sqlite（纯 Go 无 CGO，37M 单二进制）+ golang-jwt/v5 + godotenv
  - ✅ 目录：`server/{main.go, config/, db/(db.go+schema.sql+dao.go), middleware/(auth.go+quota.go), routes/(health.go+auth.go)}`
  - ✅ §5 定稿 8 张表启动自动建表（SQLite WAL）；鉴权：`POST /api/auth/login`（wechat_mp 双 provider 入口，phone 返回 501 占位）+ WX_MOCK 模式 + JWT(uid+ver) + token_ver 吊销检查
  - ✅ 配额中间件移植（滑动窗口限流/冷却/日配额/全站成本顶/长消息连击，行为对照 legacy/quota.go，挂载点待 T1.6 chat 路由接上）
  - ✅ 冒烟全过：health 200 / 无 token 401 / 假 token 401 / WX_MOCK 登录出 token（7d）/ /me 200 / 同 code 复登 userId 幂等 / 未知 provider 400 / 8 表结构验证
  - 🕳 坑：`server/fogg-coach.db` 是 8-30 legacy 冒烟残留（旧 schema），`CREATE TABLE IF NOT EXISTS` 静默跳过导致登录 500——已备份至 /tmp 并重建；发布脚本须留意旧库迁移（M1 内生产无此问题）
  - ⏭ 下一步：T1.5 FSM 状态机（转移表/extractStageDone/validateTransition，单测覆盖 §9 用例）
- **2026-09-16**（T1.5 FSM 状态机 ✅）：
  - ✅ `server/fsm/fsm.go`：S1-S9 + active 阶段常量、顺序白名单（禁跳步）、Context 结构（含 superseded 保留与 revise_count）
  - ✅ 标记协议（§3.1.3 方括号格式）：`ExtractMarkers` 只认末尾 2 行（中部标记=注入噪音→HOLD）、`[QUICK:...]` 解析 + Clip（常规≤4/S1≤7/每项≤8字）、`[RESET_WISH]`/`[MODE:UNDO]`
  - ✅ `Evaluate`（LLM 路径）：DONE 但 context 缺字段不推进（§9 核心用例）；SKIP 默认值兜底（S1-S5 各有通用默认集）；S4/S6 LLM 标记不生效（推进权在按钮）；S7 无 LLM 自推路径
  - ✅ 按钮事件：`OnSelect`（S4 选定 1-3）、`OnConfirm`（S6→S7 后端触发）、`OnRevise`（S6→S5 超 2 次锁定）、`OnResetWish`（回 S2 不重走 S1，旧数据 superseded 保留、motivation/anchors 复用；次数上限由 sessions.reset_count 列在路由层执行）、`OnModeUndo`（仅 active）
  - ✅ 单测 16/16 全过：缺字段 HOLD×7 场景、顺序推进、按钮阶段拒 LLM、标记尾部校验、chips 裁剪、revise 锁定、RESET superseded、SKIP 兜底 S2/S3/S5、S9 门禁、context JSON 往返
  - ⏭ 下一步：T1.6 LLM 层（provider 抽象 + promptBuilder + thinking 分轮策略 §3.2.5）+ `POST /api/chat`（SSE）+ `POST /api/plan/generate`（S7 强校验）

1. ✅/❌ T1.1 知识库内容结构（10 节是否够/要加域）
2. ✅/❌ T1.2 基座 prompt 的禁令与语气
3. ✅/❌ T1.3 各阶段话术细节（尤其 S2 提问、S5 庆祝菜单）
4. ✅/❌ FSM 转移条件（各阶段推进的硬性字段要求）
5. ✅/❌ LLM 首选（SenseNova flash-lite 实测，不满意换）
