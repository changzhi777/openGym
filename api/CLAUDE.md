[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **api · Node 无框架后端 + 契约冻结（批任务 Coach 停止演进）**

# api/ — 沐禾健身 Node 版后端（上游 openGym 架构）

> 自托管健身房/体重追踪器的服务端：**单文件、无框架、JSON 文件存储**。passkey 认证 + 每用户状态 + Web Push + 媒体上传 + AI Coach（批任务）。fork 现状：**契约已升级为 SaaS 双栈总纲**——`openapi.yaml`（title：沐禾健身 API）是 Node/Go 两侧唯一契约源；Go 重写栈见 [backend-go/](../backend-go/CLAUDE.md)。🆕 AI-Native 重构（chat/memory/usage + tool-loop）**已于 2026-11-11 六批次实施完毕**——对话面契约冻结在本模块 openapi（Node 侧 7+4 端点挂 501 stub），实现在 Go 栈 `internal/agent`；**Node 侧 coach/ 定调停止演进，保留为契约参考与回滚源**——见下方"AI-Native 契约与 Node 侧现状"节。

## 模块职责

- Passkey（WebAuthn）注册/登录/管理，HMAC 签名会话 cookie（无 JWT、无 session store 依赖）
- 每用户训练数据存储与字段级合并同步（`state-<uid>.json`）
- 管理面板（用户/邀请码/活动日志）、限流、审计日志
- Web Push（休息计时器/当日提醒）、自定义动作媒体上传（图片/GIF/短视频）
- **AI Coach（批任务子系统 `coach/`，停止演进）**：训练计划设计与复盘，多 provider（Anthropic/OpenAI/Gemini/OpenAI 兼容端点/Claude Agent SDK/Codex CLI）
- **fork 新增**：SaaS 后台 + 认证重构 + AI-Native 对话面契约冻结（**32 端点挂 501 stub**）+ 测试套件支持双跑 Go 栈（MUHE_GO）

## 技术栈与入口

| 项 | 值 |
|----|----|
| 运行时 | Node 22（`node:22-alpine`），ESM；包名仍为 `gym-api` |
| 框架 | **无**（`node:http` + `routes` 对象按 `'METHOD /path'` 分发） |
| 运行时依赖 | 仅 3 个：`@simplewebauthn/server`、`undici`、`web-push`（optional: `@anthropic-ai/claude-agent-sdk`，仅 coach 镜像装） |
| 入口 | `server.js`（`npm start`） |
| API 契约 | `openapi.yaml`（手写 OpenAPI 3.1，title **沐禾健身 API**；双栈契约源；**107 端点**，浏览版 website/api.html 已再生） |
| 测试 | `node --test test/*.test.js`（76 个文件） |

## 契约冻结与 501 stub（fork）

`server.js` 路由表内 `notYetGo` 处理器：fork 的**契约已冻结但实现落 Go 栈**的端点，Node 侧不实现、一律 501 `{error:"planned for the Go backend", code:"not-implemented"}`——前端对这类端点只做占位渲染不会调用，501 是误调用的明确信号。**当前挂 stub 共 32 个键**（`server.js` L1915–1950，2026-11-11 实测；演进轨迹 8 → 21（2026-11-10）→ 32（AI-Native 批次 1/5/6））：

- **SaaS 后台批（8）**：`GET /api/admin/overview` · `GET|PUT /api/admin/users/{id}/quota` · `GET /api/admin/users/{id}/activity` · `GET /api/admin/roles` · `POST /api/admin/roles/grant` · `POST /api/admin/roles/revoke` · `POST /api/admin/step-up`
- **认证重构批（7）**：`GET /api/auth/captcha` · `POST /api/auth/sms/send` · `POST /api/auth/sms/verify` · `GET /api/auth/wechat/qrcode` · `GET /api/auth/wechat/poll` · `POST /api/auth/wechat/confirm` · `POST /api/auth/bind-phone`（对应 backend-go captcha/sms/wechat 包与 0012_auth_identities 迁移）
- **管理面批 1a（6）**：`GET /api/admin/session` · `GET /api/admin/users/{id}/export` · `GET|PUT /api/admin/settings` · `POST /api/alive` · `GET /api/admin/live`
- **🆕 AI-Native 对话面批（7）**：`POST /api/coach/chat` · `GET /api/coach/chat/{id}/events`（SSE）· `POST /api/coach/chat/{id}/confirm`（确认闸门）· `GET /api/coach/memory` · `PUT /api/coach/memory/{id}` · `DELETE /api/coach/memory/{id}` · `GET /api/coach/usage`（三池 ledger）
- **🆕 计量批（2）**：`GET /api/admin/coach/usage-overview` · `PUT /api/admin/coach/limits`
- **🆕 治理批（2）**：`GET|PUT /api/admin/coach/agent-config`（MAX_TURNS 1–8 + 审核词表 ≤200，PUT 后下一回合热生效）

**实现全在 Go 栈**（对话面 = `backend-go/internal/agent`，见 [backend-go/CLAUDE.md](../backend-go/CLAUDE.md)）。Coach 批任务现有路由（`/api/coach/disclosure|status|plan|review|debrief|pending/resolve|cohort(+share)|account|forget` 共 10 条契约路径）为**已实现端点**，不挂 stub。

带 `{id}` 参数的路由须同步 `server.js` 分发器的模板键映射（L2697–2709：`mu` 管理面 / `mm` 媒体 / `cc` 对话面 events+confirm / `cmm` memory/{id}，批次 2 补漏）——契约一致性测试只查键存在不查 URL 可达，双跑冒烟才抓得住这类遗漏。

## 🆕 AI-Native 契约与 Node 侧现状（2026-11-11 六批次实施完毕）

> 方案全文：《沐禾健身-AINative重构方案-v2.md》（`.zcf/`，fork 本地）。**本模块已无"待实施"项**：Node 侧角色 = 契约冻结 + 501 stub + 双跑冒烟被测方；Agent 内核/工具/审核/记忆全部在 Go 栈。

### 契约现状（批次 1/5/6 冻结）

- **对话面 7 操作 / 6 路径**：POST `/api/coach/chat`（发起/续聊，回会话句柄）· GET `/api/coach/chat/{id}/events`（SSE 六事件：token / tool_call / proposal / confirm_result / blocked / done，回复先审后流，blocked 带拦截原因）· POST `/api/coach/chat/{id}/confirm`（确认闸门 = Coach 唯一写入口）· GET `/api/coach/memory` + PUT/DELETE `/api/coach/memory/{id}`（叙事/结构化双类记忆，删=先结构化后叙事跨存储一致性）· GET `/api/coach/usage`（chat/review/embedding 三池）
- **管理面 4 操作**：GET `/api/admin/coach/usage-overview`（全站三池日/月 + 今日活跃 + 14 天趋势，无用户级行与对话原文）· PUT `/api/admin/coach/limits`（per-user 硬限额，范围校验 1–10000/≥1000）· GET|PUT `/api/admin/coach/agent-config`（MAX_TURNS 1–8 + 词表 ≤200）
- **新增 schema**：`CoachChatSession` / `CoachMemoryEntry` / `CoachUsage`（openapi L4792–4817）；`website/api.html` 已再生（**107 endpoints**，L250 实测）
- **Node 侧分发器**：批次 2 补 5 条 `{id}` 路由映射（批次 1 遗漏——契约一致性测试只查键存在不查 URL 可达，双跑冒烟抓出后修复，`server.js` L2705–2709 有注释）

### Node 侧 coach/ 停止演进（定调）

- **批任务 Coach（`coach/` 40 文件）原样保留**：routes/jobs/pipeline/validate/adapters 一律不动，作为 ① 契约参考（disclosure/plan/review 等批任务语义）② 回滚源 ③ 上游 rebase 的对齐面
- **v1 规划中的 Node 侧改造全部作废（方案 v2 R 系裁决）**：`coach/core/agent-tools.js` 不建、`pipeline.js` tool-loop 旁路不做、`core/agent/` 目录不存在——Agent 内核整体落 Go 栈 `backend-go/internal/agent`（`loop.go` 手写循环，零新依赖）；MCP 注册表"提炼进 Coach 内核"方案作废，改为**算法单源、三实现**（见 [mcp/CLAUDE.md](../mcp/CLAUDE.md)）
- 改 `coach/` 下任何文件的正确姿势：先问"是否真的还要动批任务"——对话式能力一律加在 Go 栈

## 关键文件地图

**根级模块**（server.js 直接 import，Dockerfile 逐个 COPY——`dockerfile-copy.test.js` 强制清单同步）：

| 文件 | 职责 |
|------|------|
| `server.js` | 主服务器：路由表（含 501 stub 区 + `{id}` 分发器映射）、会话、db 读写、审计、健康检查 |
| `password.js` | 可选密码登录（scrypt、重置码、邮箱别名） |
| `passkeys-store.js` | passkey 凭据存取（上限、改名、移除保护） |
| `device-link.js` | 一次性设备配对码（移动端 redeem 换同签名 Bearer token） |
| `media.js` | 自定义动作媒体存储（配额/限额/GC，`uploads/<profile>/`） |
| `queue.js` / `sync-stamps.js` / `durable.js` | 队列例程、同步时间戳、原子写（write-temp-then-rename） |
| `rate-limit.js` | 指数回退限流（密码/配对路由按地址计数） |
| `nudge.js` / `nudge-copy.js` / `push-messages.js` | 提醒判定、推送文案、Web Push 消息 |
| `verify-error.js` | WebAuthn 验证错误的可读化 |

**coach/ 子系统**（40 文件，批任务实现，**停止演进**；默认镜像不含，`API_TARGET=coach` 才构建）：

- `coach/routes.js` — Coach HTTP 路由（挂进主 routes 表）
- `coach/jobs.js` / `cadence.js` / `cohort.js` / `warmup.js` — 任务调度、周节奏、队列分组、预热
- `coach/config.js` — 配置与凭据（加密进 `data/coach.json`）
- `coach/core/` — 纯核心：`pipeline.js`（批任务管线，保持不动）、`prompt(s).js` / `system-prompt.js`、`schemas.js` / `validate.js`（**安全边界，语义永不动**）、`refine-changes.js`、`plan-hash.js`、`library(-data).js`、`instructions-zh.js`（中文动作名+分步指导包，`build-coach-assets.mjs` 生成）
- `coach/core/adapters/` — provider 适配器：anthropic / openai / gemini / compatible（Ollama/vLLM）/ http（纯 HTTPS，default 镜像可用）
- `coach/adapters/` — 容器内运行时：claude（Agent SDK）、codex（Codex CLI）、`spawn.js`（**降权到 coach 用户，fail-closed**）
- `coach/prompts/*.md` — create / refine / review / repair / debrief 提示词模板
- `fixture-cli.mjs` — fixture provider 的手动走环 CLI
- （AI-Native 规划的 `core/agent-tools.js` / `core/agent/` 已按方案 v2 R 系裁决**取消**——Agent 内核在 Go 栈，本目录勿新增）

## 数据模型（全部为 JSON 文件，`DATA_DIR` 默认 /data）

| 文件 | 内容 |
|------|------|
| `db.json` | 全部账号、passkey 凭据、密码哈希、push 订阅、邀请码 |
| `state-<uid>.json` | 每用户训练数据（与前端 Zustand 状态同构，字段级合并；**Go 栈里对应 PG `states` 表，`muhemigrate` 可双向迁移**） |
| `secret` | 会话 HMAC 密钥（首启生成，丢失=全员登出；**灰度两栈必须同钥，cookie 才互通**） |
| `vapid.json` | Web Push VAPID 密钥（首启生成；Go 栈共用） |
| `audit.log` | JSONL 活动日志（登录成败、管理员动作，容量/天数封顶） |
| `uploads/<profile>/` | 自定义动作媒体（仅属主可读回；Go 栈 `<DATA_DIR>/media/<uid>/<sha>` 布局兼容） |
| `coach.json` + `coach/` | Coach 配置与加密凭据（Codex 刷新凭据在 `./coach-auth`，故意不在 data/ 内） |
| （AI-Native 数据不落 data/） | 会话/提议/约束/记忆/用量在 Go 栈 PG 0013 五表（`coach_chat_sessions` / `coach_proposals` / `coach_constraints` / `coach_memory_narrative` / `coach_usage_events`）——v1 规划的 `coach-memory.json`/`coach-usage.json` 未采用 |

## 环境变量要点

`RP_ID` / `ORIGIN`（passkey 绑定主机名，**动 auth 前必读 docs/SELF_HOSTING.md**）、`PORT`(3000)、`DATA_DIR`、`SESSION_DAYS`(90)、`ADMIN_UIDS` / `FIRST_USER_ADMIN` / `INVITE_ONLY` / `ALLOW_GUEST` / `DEFAULT_LANG` / `PASSWORD_LOGIN` / `TRUST_PROXY`、`AUDIT_LOG/MAX/DAYS/IP`、`MEDIA_*`、`VAPID_SUBJECT`、`COACH_DISABLED` / `COACH_JOB_TIMEOUT_MS` / `API_TARGET`。完整注释见根 [.env.example](../.env.example)。（AI-Native 的 LLM/审核/WeKnora env 全在 Go 栈侧，见 backend-go 文档。）

## 镜像（api/Dockerfile，单文件双 target）

- `default`（最后 stage，默认构建）：`npm ci --omit=optional` —— 无 AI 运行时，并删除 npm 本身
- `coach`：再加 Agent SDK + Codex CLI + bubblewrap + `/coach-auth` 卷（批任务 Coach，停止演进）
- healthcheck：`wget /api/health`；模块 COPY 清单改动必须同步 Dockerfile（有测试盯着）

## 测试（76 文件，含 fork 双跑扩展）

```bash
cd api && npm test        # node --test test/*.test.js
MUHE_GO=1 npm test        # fork：同一批用例拉起 Go 栈双跑（需 PG+Redis，见 backend-go/compose.go.yml）
```

- `routes.test.js` + `openapi-routes.test.js` — 路由与 OpenAPI 契约一致（**501 stub 端点也在契约内**；注意只查键存在不查 URL 可达，`{id}` 路由靠分发器映射 + 双跑冒烟兜底）
- `server-*.test.js` — 按子系统切分：passkeys/password/media/push/pairing/admin/data-rev/cors/presence…
- **fork 双跑**：29 个文件带 `MUHE_TEST=1` 启动分支（2026-11-11 实测，较 11-10 的 28 个 +1）；`MUHE_GO=1` 时改为拉起 Go 二进制（`backend-go/internal/testmode` 提供每测试独立 PG 库、seed、`BootLine` 端口行与 Node 同序）——**backend-go 的主测法就在这套件里**
- 🆕 `coach-chat-off.test.js` — AI-Native 双跑冒烟（批次 2）：Node 走 501 stub / Go 走 503 off（Provider 未配）+ memory·usage 真数据，JWT cookie 双栈互通验证
- coach 批任务侧：adapters / pipeline(payload) / prompts / jobs / cadence / cohort / review-repair（随 coach/ 停止演进，此区冻结）
- `dockerfile-copy.test.js`、`k8s-web-port.test.js` — 部署件一致性
- 夹具：`helpers.mjs`、`fake-clock.mjs`、`fixtures/media-refs.json`
- 台账口径：批次 2 后 api 套件 612 测试 **606 pass / 0 fail**（基线 605 + coach-chat-off 1）

## FAQ / 坑

- **加路由**：`routes` 对象加键 → 补 `openapi.yaml` → `node scripts/build-api-docs.mjs` 重生成 api.html → 新模块文件加进 Dockerfile COPY 行；**fork 流程**：新端点先在 Node 挂 `notYetGo` 501 占位，实现落 Go 栈；**带 `{id}` 的路由必须同步 server.js 分发器模板键映射**（批次 2 教训，见契约冻结节）
- **db.json 损坏**：server 只在文件缺失时从空启动，不可读时拒绝启动
- **CSRF**：非 GET 路由查 `Sec-Fetch-Site`/`Origin`；豁免仅限带一次性凭据的 passkey 四路由 + `pair/redeem` + Bearer（Go 侧 `httpx.CSRF` 为同语义移植）
- **限流内存态**：重启清零；IPv6 按 /64 计
- **与 Go 栈并存**：Node 是上游架构 + fork 契约占位；切换/回滚规程见 `backend-go/scripts/gray-switch.sh`
- **AI-Native 能力去哪加**：一律加在 Go 栈 `backend-go/internal/agent`（先改 openapi 契约）；本模块 coach/ 已停止演进，勿在此实现对话式能力（见"Node 侧 coach/ 停止演进"节）

## 相关文件清单（节选）

`server.js`（含 501 stub 区 + {id} 分发器）· `openapi.yaml`（107 端点，含对话面）· `Dockerfile` · `package.json`（gym-api）· `password.js` · `media.js` · `coach/`（40 文件，停止演进）· `test/`（76 文件，含 coach-chat-off 双跑冒烟）· `scripts/`（build-nudge-copy / check-core-loadable）· 文档：[docs/AI_COACH.md](../docs/AI_COACH.md)、[docs/API.md](../docs/API.md)、[backend-go/CLAUDE.md](../backend-go/CLAUDE.md)（Agent 内核实现）、[mcp/CLAUDE.md](../mcp/CLAUDE.md)（算法单源三实现）

---

变更记录: 2026-11-11 V1.3 增量（init-architect，实施后刷新轮）:AI-Native 规划节改写为"契约与 Node 侧现状"——七端点对话面 + 计量/治理 4 端点契约已冻结（stub 21→32 键，L1915–1950 实测）、api.html 107 端点、Node 分发器 5 条 {id} 映射已补（批次 2 教训入 FAQ）、coach-chat-off.test.js 双跑冒烟已加；**Node 侧 coach/ 定调停止演进（契约参考与回滚源）**，v1 规划的 agent-tools.js/tool-loop 旁路/core/agent/ 全部作废（方案 v2 R 系裁决）；测试 74→76 文件、MUHE_TEST 28→29；数据模型注记 AI-Native 落 PG 0013 五表非 data/ JSON。2026-11-10 V1.2 增量:新增 AI-Native 重构规划节（P0 五组端点 / P1 agent-tools.js + tool-loop 旁路 / P3/P4 落点，全部规划态代码未动）；修正漂移——501 stub 8→21 键、MUHE_TEST 26→28、coach/ 33→40 文件；2026-10-27 V1.1 增量:契约冻结与 501 stub、MUHE_GO 双跑、测试数 71→74、品牌名；2026-10-20 V1.0 初版
