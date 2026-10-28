[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **api · Node 无框架后端 + AI Coach**

# api/ — 沐禾健身 Node 版后端（上游 openGym 架构）

> 自托管健身房/体重追踪器的服务端：**单文件、无框架、JSON 文件存储**。passkey 认证 + 每用户状态 + Web Push + 媒体上传 + 可选 AI Coach。fork 现状：**契约已升级为 SaaS 双栈总纲**——`openapi.yaml`（title：沐禾健身 API）是 Node/Go 两侧唯一契约源；Go 重写栈见 [backend-go/](../backend-go/CLAUDE.md)。

## 模块职责

- Passkey（WebAuthn）注册/登录/管理，HMAC 签名会话 cookie（无 JWT、无 session store 依赖）
- 每用户训练数据存储与字段级合并同步（`state-<uid>.json`）
- 管理面板（用户/邀请码/活动日志）、限流、审计日志
- Web Push（休息计时器/当日提醒）、自定义动作媒体上传（图片/GIF/短视频）
- **AI Coach（可选子系统 `coach/`）**：训练计划设计与复盘，多 provider（Anthropic/OpenAI/Gemini/OpenAI 兼容端点/Claude Agent SDK/Codex CLI）
- **fork 新增**：SaaS 后台契约冻结（8 端点挂 501 stub）+ 测试套件支持双跑 Go 栈（MUHE_GO）

## 技术栈与入口

| 项 | 值 |
|----|----|
| 运行时 | Node 22（`node:22-alpine`），ESM；包名仍为 `gym-api` |
| 框架 | **无**（`node:http` + `routes` 对象按 `'METHOD /path'` 分发） |
| 运行时依赖 | 仅 3 个：`@simplewebauthn/server`、`undici`、`web-push`（optional: `@anthropic-ai/claude-agent-sdk`，仅 coach 镜像装） |
| 入口 | `server.js`（`npm start`） |
| API 契约 | `openapi.yaml`（手写 OpenAPI 3.1，title **沐禾健身 API**；P0 已冻结为双栈契约源；浏览版 website/api.html） |
| 测试 | `node --test test/*.test.js`（74 个文件） |

## 契约冻结与 501 stub（fork，P0）

`server.js` 路由表内新增 `notYetGo` 处理器：fork 的 SaaS 后台端点**已进 openapi.yaml 契约**，Node 侧不实现、一律 501 `{error:"planned for the Go backend", code:"not-implemented"}`——前端 `views/admin/` 对这些只做占位渲染不会调用，501 是误调用的明确信号。挂 stub 的 8 个键：

`GET /api/admin/overview` · `GET|PUT /api/admin/users/{id}/quota` · `GET /api/admin/users/{id}/activity` · `GET /api/admin/roles` · `POST /api/admin/roles/grant` · `POST /api/admin/roles/revoke` · `POST /api/admin/step-up`

真正的实现落在 [backend-go/](../backend-go/CLAUDE.md)（P5：九端点 + support/ops/super_admin 角色分层 + step-up + 审计）。

## 关键文件地图

**根级模块**（server.js 直接 import，Dockerfile 逐个 COPY——`dockerfile-copy.test.js` 强制清单同步）：

| 文件 | 职责 |
|------|------|
| `server.js` | 主服务器：路由表（含 501 stub 区）、会话、db 读写、审计、健康检查 |
| `password.js` | 可选密码登录（scrypt、重置码、邮箱别名） |
| `passkeys-store.js` | passkey 凭据存取（上限、改名、移除保护） |
| `device-link.js` | 一次性设备配对码（移动端 redeem 换同签名 Bearer token） |
| `media.js` | 自定义动作媒体存储（配额/限额/GC，`uploads/<profile>/`） |
| `queue.js` / `sync-stamps.js` / `durable.js` | 队列例程、同步时间戳、原子写（write-temp-then-rename） |
| `rate-limit.js` | 指数回退限流（密码/配对路由按地址计数） |
| `nudge.js` / `nudge-copy.js` / `push-messages.js` | 提醒判定、推送文案、Web Push 消息 |
| `verify-error.js` | WebAuthn 验证错误的可读化 |

**coach/ 子系统**（默认镜像不含，`API_TARGET=coach` 才构建）：

- `coach/routes.js` — Coach HTTP 路由（挂进主 routes 表）
- `coach/jobs.js` / `cadence.js` / `cohort.js` / `warmup.js` — 任务调度、周节奏、队列分组、预热
- `coach/config.js` — 配置与凭据（加密进 `data/coach.json`）
- `coach/core/` — 纯核心：`pipeline.js`、`prompt(s).js` / `system-prompt.js`、`schemas.js` / `validate.js`、`refine-changes.js`、`plan-hash.js`、`library(-data).js`、`instructions-zh.js`（中文动作名+分步指导包，fork 已随动作说明中文化同步扩充，`build-coach-assets.mjs` 生成）
- `coach/core/adapters/` — provider 适配器：anthropic / openai / gemini / compatible（Ollama/vLLM）/ http（纯 HTTPS，default 镜像可用）
- `coach/adapters/` — 容器内运行时：claude（Agent SDK）、codex（Codex CLI）、`spawn.js`（**降权到 coach 用户，fail-closed，Coach 读不到 /data 的控制点**）
- `coach/prompts/*.md` — create / refine / review / repair / debrief 提示词模板
- `fixture-cli.mjs` — fixture provider 的手动走环 CLI

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

## 环境变量要点

`RP_ID` / `ORIGIN`（passkey 绑定主机名，**动 auth 前必读 docs/SELF_HOSTING.md**）、`PORT`(3000)、`DATA_DIR`、`SESSION_DAYS`(90)、`ADMIN_UIDS` / `FIRST_USER_ADMIN` / `INVITE_ONLY` / `ALLOW_GUEST` / `DEFAULT_LANG` / `PASSWORD_LOGIN` / `TRUST_PROXY`、`AUDIT_LOG/MAX/DAYS/IP`、`MEDIA_*`、`VAPID_SUBJECT`、`COACH_DISABLED` / `COACH_JOB_TIMEOUT_MS` / `API_TARGET`。完整注释见根 [.env.example](../.env.example)。

## 镜像（api/Dockerfile，单文件双 target）

- `default`（最后 stage，默认构建）：`npm ci --omit=optional` —— 无 AI 运行时，并删除 npm 本身
- `coach`：再加 Agent SDK + Codex CLI + bubblewrap + `/coach-auth` 卷
- healthcheck：`wget /api/health`；模块 COPY 清单改动必须同步 Dockerfile（有测试盯着）

## 测试（74 文件，含 fork 双跑扩展）

```bash
cd api && npm test        # node --test test/*.test.js
MUHE_GO=1 npm test        # fork：同一批用例拉起 Go 栈双跑（需 PG+Redis，见 backend-go/compose.go.yml）
```

- `routes.test.js` + `openapi-routes.test.js` — 路由与 OpenAPI 契约一致（**501 stub 端点也在契约内**）
- `server-*.test.js` — 按子系统切分：passkeys/password/media/push/pairing/admin/data-rev/cors/presence…
- **fork 双跑**：26 个文件带 `MUHE_TEST=1` 启动分支；`MUHE_GO=1` 时改为拉起 Go 二进制（`backend-go/internal/testmode` 提供每测试独立 PG 库、seed、`BootLine` 端口行与 Node 同序）——**backend-go 的主测法就在这套件里**
- coach 侧：adapters / pipeline(payload) / prompts / jobs / cadence / cohort / review-repair
- `dockerfile-copy.test.js`、`k8s-web-port.test.js` — 部署件一致性
- 夹具：`helpers.mjs`、`fake-clock.mjs`、`fixtures/media-refs.json`

## FAQ / 坑

- **加路由**：`routes` 对象加键 → 补 `openapi.yaml` → `node scripts/build-api-docs.mjs` 重生成 api.html → 新模块文件加进 Dockerfile COPY 行；**fork 流程**：新端点可先在 Node 挂 `notYetGo` 501 占位，实现落 Go 栈
- **db.json 损坏**：server 只在文件缺失时从空启动，不可读时拒绝启动
- **CSRF**：非 GET 路由查 `Sec-Fetch-Site`/`Origin`；豁免仅限带一次性凭据的 passkey 四路由 + `pair/redeem` + Bearer（Go 侧 `httpx.CSRF` 为同语义移植）
- **限流内存态**：重启清零；IPv6 按 /64 计
- **与 Go 栈并存**：Node 是上游架构 + fork 契约占位；切换/回滚规程见 `backend-go/scripts/gray-switch.sh`

## 相关文件清单（节选）

`server.js` · `openapi.yaml` · `Dockerfile` · `package.json`（gym-api）· `password.js` · `media.js` · `coach/`（33 文件）· `test/`（74 文件）· `scripts/`（build-nudge-copy / check-core-loadable）· 文档：[docs/AI_COACH.md](../docs/AI_COACH.md)、[docs/API.md](../docs/API.md)、[backend-go/CLAUDE.md](../backend-go/CLAUDE.md)

---

变更记录: 2026-10-27 V1.1 增量（init-architect）:契约冻结与 501 stub、MUHE_GO 双跑、测试数 71→74、品牌名；2026-10-20 V1.0 初版
