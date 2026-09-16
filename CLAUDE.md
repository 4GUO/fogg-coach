# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目是什么

fogg-coach：内嵌微信小程序的「福格行为模型 AI 教练」。用户与教练 Agent 对话诊断（B=MAP 方法论），生成微习惯计划（Plan JSON）；小程序承载每日打卡、即时庆祝（Shine）、数据回流与复盘迭代。仓库当前处于 **M1 阶段（后端与 Prompt）**，前端尚未创建。

## 技术栈（2026-09-12 已确认）

- **后端**：Go 1.22+ / Gin + SQLite（modernc.org/sqlite，纯 Go 无 CGO），单二进制 + systemd 部署（system-design §2/§7）
- **前端**：uni-app（Vue3 + TS + Vite），一套代码编译到微信小程序 / H5 / 安卓，端差异用条件编译（system-design §6.0）
- **LLM**：OpenAI 兼容协议 Provider 抽象，GLM + DeepSeek 双支持，可按用途路由/热切

### 遗留代码说明

`server/src/` 现存 **Fastify 5 + better-sqlite3 的 Node 原型**（T1.4 骨架：auth/JWT、限流/配额、schema+DAO），确认换栈后**仅作行为参考**——其 schema、配额规则、鉴权流程是已拍板行为，Go 实现时直接对照移植。新后端代码一律写 Go，不要在 Node 原型上继续扩展业务逻辑。`.gitignore` 已预留 Go 二进制名 `server/fogg-coach`。Go 模块放 `server/`（2026-09-16 拍板；内部结构 routes/fsm/llm/prompts/db 见 system-design §2），原型已挪至 `server/legacy/`，Go 通过 M1 验收后删除。

## 文档地图（文档驱动开发，改代码前先查对应文档）

| 文档 | 职责 |
|------|------|
| `docs/product-spec-final.md` | 产品定稿，所有已拍板决策（2026-09-10） |
| `docs/system-design.md` | 系统设计 v1.1：FSM、LLM 层、API、Schema、部署（技术细节唯一来源） |
| `docs/build-breakdown.md` | 任务拆解 T1.x-T4.x + **进度记录**（当前做到哪、踩过什么坑，每次交付后更新） |
| `docs/fogg-method.md` | 福格方法论知识库（prompt 素材源） |
| `docs/pages-interaction.md` / `docs/design-style.md` | 前端信息架构/交互规则、视觉与教练人格规范 |
| `server/prompts/` | `base.md` 基座 + `stages/S1-S7.md` 阶段 prompt —— **核心资产，改动需用户确认话术** |

## 常用命令

目前 Node 原型挪在 `server/legacy/` 可运行（Go 项目尚未脚手架，脚手架后更新本节）：

```bash
cd server/legacy
npm install
cp .env.example .env        # 所有配置走 .env，禁止硬编码数值
npm run dev                 # node --watch src/app.js，监听 127.0.0.1:3210

# 冒烟（WX_MOCK=1 时任意 code 登录成功）
curl localhost:3210/api/health
curl -X POST localhost:3210/api/auth/login -H 'Content-Type: application/json' -d '{"code":"test"}'
```

- 无测试框架、无 lint 配置。验收方式是 build-breakdown.md 里各任务的 curl 用例（如 3 个愿望域走通 S1→S7 出合法 plan JSON）
- SQLite 文件 `server/fogg-coach.db` 启动时自动按 `src/db/schema.sql` 建表（已 gitignore）
- M1 验收产物（对话转录）存 `docs/test-transcripts/`

## 核心架构概念

**会话状态机 S1-S9 是全项目的主轴**：S1-S7 诊断漏斗（探索愿望→澄清现状→头脑风暴→焦点地图→配方→确认→生成 Plan JSON），S8 复盘迭代，S9 解坏习惯。

**铁律：阶段推进权在后端，绝不信任 LLM 自律**（system-design §3.1.3 三道防线）：
1. 每阶段独立 system prompt（LLM 只见当前阶段规范）
2. LLM 输出尾部结构化标记，服务端正则解析 + 白名单顺序推进（禁跳步）；解析不到按 HOLD；S7 由后端在 S6 确认后主动触发，LLM 无权自推
3. S4 选择、S6 确认走按钮事件（`action` 字段），完全绕过 LLM 判断

**标记协议（2026-09-16 已统一）**：`[STAGE:DONE]`/`[STAGE:HOLD]`/`[STAGE:SKIP]`/`[QUICK:...]` 方括号格式；chips 常规 ≤4 个，S1 愿望域特例 ≤7 个。

**其他关键设计**：
- SSE 流式对话：`delta` / `stage_done` / `quick_replies` / `done` 事件；小程序端 `wx.request enableChunked`，H5/安卓 fetch ReadableStream
- Plan JSON schema（S7 输出）服务端强校验：habits 1-3 条、微行为 ≤20 字且 30 秒可完成、progression 1 级 after_checkins=7
- 多 plan 并存（不锁单 active，软上限 3 个，仅 prompt 引导不硬限）；改愿望走 `[RESET_WISH]` 回退 S2，每 session ≤2 次
- 防刷/配额全部走 config（.env）：50 条/日、25 轮/session、500 字/条、3 次生成/日、全站日耗硬顶
- LLM 层 OpenAI 兼容协议抽象，GLM + DeepSeek 双支持，可按用途路由/热切；M1 先接 GLM 验收，DeepSeek M1 内补齐

## 产品红线（写代码/prompt 时必须守住）

- **失败 = 设计问题**：任何文案与教练话术禁止出现意志力指责（"坚持/自律/毅力"）
- **合规**（个人主体审核）：无医疗/心理诊断表述；S9 仅用户主动触发，不自动推荐；社区无评论仅"加油"点赞、内容全量过 msgSecCheck/imgSecCheck
- **庆祝即时性**：Shine 动画本地触发 <200ms，不等网络返回（乐观更新）
- **打卡永远最轻**：任何功能不得加长"打开→打卡"路径；记录/分享默认可跳过
- 教练人格与语气约束的权威来源是 `server/prompts/base.md` 与 `docs/design-style.md`

## 语言约定

文档、代码注释、commit message、教练输出均为中文；变量与标识符用英文。
