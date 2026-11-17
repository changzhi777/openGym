[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **backend-go**

# backend-go/ — 沐禾健身 Go 版后端（fork 专属）

> openGym（Node 无框架版）的 **Go 重写栈**：Gin + PostgreSQL(pgx) + Redis，**迁移内嵌随进程跑**，部署面只剩 PG + Redis + 本进程。这是 fork（沐禾健身 / changzhi777/openGym）的 SaaS 化主战场与**生产后端**（qingmulife.com）；上游 `api/`（Node）保持可运行作为对照与回滚源。🆕 **AI-Native Agent 内核已于 2026-11-11 实施完毕（批次 1–6 + 后段）**：`internal/llm` + `internal/agent` + `internal/weknora` 三新包、0013 五表、对话面七端点 + admin coach 四端点、tool-loop + 审核链 + 双 golden（onerm 4480 案 / plan 八场景）——见"AI-Native 实施现状"节。

## 模块职责与推进状态

按 P0 → P6 推进（方案：`.zcf/plan/current/saas-backend-go.md`，fork 本地）：

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 契约冻结：`api/openapi.yaml` 为唯一契约源 | ✅ |
| P1 | 骨架：Gin + pgx + go-redis + `/api/health`（探活含 PG/Redis 双写检） | ✅ |
| P2 | 认证闭环（**已被 2026-10-29 认证重构取代，见下**） | ✅→♻️ |
| P3 | 数据核心：`/api/data` GET/PUT + `/api/data/rev` + `/api/config`；`muhemigrate` 双向迁移器（round-trip 达标） | ✅（2026-10-28 缺口已修复，见下） |
| P4 | Web Push 七端点（VAPID 与 Node 共用 `data/vapid.json`）+ 媒体四端点（sha256 校验 + 魔数嗅探 + `<DATA_DIR>/media/<uid>/<sha>` 布局与 Node 兼容） | ✅ |
| P5 | 管理后台：九端点 + 角色分层（support 只读 / ops 运营 / super_admin 全权）+ 敏感操作 step-up + 审计 | ✅ |
| P6 | 灰度演练：`scripts/gray-switch.sh`（备份 → 导入 → round-trip 验收 → 切换 → 回滚） | ✅ 已演练 |
| 批1-3 | 2026-10-28 管理面全量：session/邀请码全链路/实例设置/审计深化(90天)/Users 重写/activity/export/quota used/收编 P5 瘦尾 WIP(coach 桥+quota+媒体 GC,0007-0008)/presence(Redis)+live SSE/coach 放开 ops+step-up/概览 v2 | ✅（FORK.md §5 台账） |
| 认证重构 | **2026-10-29**：手机号+短信验证码（`/api/auth/captcha` 图形码 + `sms/send` + `sms/verify`）+ 微信小程序码扫码（`wechat/qrcode` + `poll` + `confirm`）+ `bind-phone` 换绑；**passkey/密码/配对在本栈退役**；新增 `internal/captcha` / `internal/sms` / `internal/wechat` 三包 + 0012_auth_identities 迁移。Node 侧对应 7 端点挂 501 stub | ✅ |
| **🆕 AI-Native 批次 1–6 + 后段** | **2026-11-10/11**：方案 v2（`.zcf/沐禾健身-AINative重构方案-v2.md`）六批次全落地——批次 1 契约先行（对话面 7 操作/6 路径 + 3 schema 进 openapi）· 批次 2 Go Agent 内核（`internal/llm` + `internal/agent` + 0013 五表 + golden 4480 案 + 修 0007 迁移幂等 bug + 补 Node 分发器 5 条 {id} 映射）· 批次 3 WeKnora 知识集成（`internal/weknora` + `compose.weknora.yml` + ETL）· 批次 4 对话前端（frontend 侧）· 批次 5 计量 admin 化（usage-overview + limits）· 批次 6 治理台（agent-config 热生效 + get_bodyweight 营养域 v1 + reviewer 每回合热组装）· **批次 6 后段**（tool_call refs 动作卡 `Tool.ExtractRefs` + 前端记忆页 + **get_week_plan 入注册表**：plan.go + golden 八场景） | ✅（**FORK.md §5 台账八行为准**） |

**✅ 编译缺口已修复（2026-10-28）**：`internal/data`（P3a 数据三端点的 `data.Service`）代码早已写完，但被根 `.gitignore` 裸写的 `data/` 规则（无根锚定、任意深度匹配）整包静默吞掉——`git add` 跳过无报错、全历史零踪迹，实体仅存于主 checkout 工作区。已拷回 3 文件（handlers/stamps/stamps_test，592 行）并把规则锚根为 `/data/`；`go build ./...` 与 `go test ./internal/data/` 全绿，2026-10-28 随本仓提交恢复入库。教训：新目录提交前跑 `git check-ignore -v` 或核对 `git status`，ignore 规则一律根锚定。

## 目录地图（~100 文件 · internal 19 包 · 54 个 .go（internal 52 + cmd 2）；2026-11-18 复核）

| 路径 | 内容 |
|------|------|
| `cmd/api/main.go` | 装配入口：env 读取（与 Node 同名同义：`RP_NAME`(默认"沐禾健身")/`RP_ID`/`ORIGIN`/`SESSION_DAYS`/`SECRET`/`DATA_DIR`/`PORT`(默认 **3100**)/`DATABASE_URL`/`REDIS_ADDR`）+ 中间件（CORS→CSRF→BodyGuard）+ 全部路由表 + testmode 双跑钩子 + 🆕 AI-Native 装配（llm FromEnv / weknora client / agent Service，L188 起注入工具集与 globalKB） |
| `cmd/muhemigrate/main.go` | **双向迁移器**（P3b）：`import <dataDir>` 把 Node 的 `db.json`(users/creds) + `state-<uid>.json` 灌进 PG；`export <outDir>` 反向导出同构文件供比对/回滚。按 id upsert 幂等，passkey 材料全程内存不落盘 |
| `internal/agent/` 🆕（15 .go + testdata×2） | **AI-Native Agent 内核**（批次 2/5/6 + 后段）：`loop.go`（tool-loop 引擎：MAX_TURNS 硬顶默认 4/admin 可配 1–8、工具结果截断、**提议永不执行**、T1 先审后流、事件流）· `tools.go`+`tools_setup.go`（工具集装配：`estimate_1rm` / `propose_constraint` / `propose_memory` / **`get_week_plan`** / `get_bodyweight` / `search_knowledge` 共 6 工具，WeKnora 未配置时不注册检索工具=D15 降级）· `review.go`（审核链：`reviewerFor` **每回合热组装** = 配置词表∪`MUHE_REVIEW_WORDS` env 兜底 → LLM 复检 fail-closed，`reason="llm_error"` 合规优先于可用性）+ `Tool.ExtractRefs`（批次 6 后段：search_knowledge 结果抽 catalogue 动作 id → tool_call refs）· `session.go`（会话状态机 active ⇄ waiting_confirm → closed，PG 存取）· `handlers.go`（对话面七端点 + admin coach 四端点 handler）· `limits.go`（GetLimits/SetLimits 读 `instance_settings.coach_limits` + UsageOverview 全站聚合 + generate_series 14 天补零趋势；坏 JSON 防御回默认）· `agentconfig.go`（GetAgentConfig/SetAgentConfig，坏 JSON 防御回默认，PUT 后下一回合热生效）· `onerm.go`（frontend onerm.js 逐行翻译）+ `onerm_golden_test.go` + `testdata/onerm-golden.json`（**golden 4480 案 JS↔Go 等价验收** + selfcheck 防双端同名回退假绿）· 🆕 `plan.go`（批次 6 后段：queue.js 全文翻译 + history.js effectiveRoutineIds + mcp get_week_plan 组装层对齐——plannedBy 六分支 + days 7 天 + coach_week + weekdays 周模板行）+ `plan_golden_test.go` + `testdata/plan-golden.json`（**golden 八场景全等**，含 firstDiffPath 字段级诊断）· 测试：`loop_test.go` / `review_test.go` / `session_pg_test.go`（PG 真库链路） |
| `internal/llm/` 🆕（3 文件） | provider 三件套抽象（`provider.go`，**FromEnv 返回接口根治 typed-nil**——`*Client` nil 装接口后 `==nil` 恒 false 曾致 503 off 失效）+ Anthropic 兼容协议客户端首实现（`anthropic.go`，直连智谱 GLM：`BIGMODEL_API_KEY` / `BIGMODEL_BASE_URL` / `BIGMODEL_MODEL`，零新依赖）+ `client_test.go` |
| `internal/weknora/` 🆕（2 文件） | WeKnora REST 客户端（`client.go`：knowledge-search / knowledge-bases / documents；宽容解析；API 形状依 WeKnora v0.9 `{success, data[], has_more}` + Bearer/ApiKey；**D14 强制 kbIDs 谓词——空 ids 直接拒，杜绝裸检索越区**；D15 失败回填语义）+ `client_test.go`（httptest 6 剧本）。env：`WEKNORA_BASE_URL` / `WEKNORA_API_KEY` / `WEKNORA_GLOBAL_KB`（main.go 注入） |
| `internal/auth/`（3 文件） | 会话（HMAC cookie，与 Node 同钥互认）+ 请求体绑定；🆕 认证重构后含短信/微信登录 handler（`handlers_auth.go`），challenge/pair 内存 store 随 passkey/配对退役 |
| `internal/admin/`（2 文件） | 管理后台 service + handlers：角色三层 + `RequireStepUp()` + 审计 |
| `internal/apigen/gen.go` | **oapi-codegen 生成物，勿手编**（配置 `oapi-codegen.yaml`：从根 `api/openapi.yaml` 生成 models + gin-server，strict-server 关） |
| `internal/cache/` | go-redis 封装。业务态（短信码/微信态）用 Redis；重启即失的内存态已收敛 |
| `internal/captcha/` | 图形验证码（认证重构配套） |
| `internal/coach/` | 教练 sidecar 桥：/api/coach/* 反代 + admin 六端点（ops+，密钥操作挂 StepUpOK）+ /internal/coach 回读面 + 配额闸 |
| `internal/data/`（3 文件） | P3a 数据三端点 handler：`Get`/`GetRev`/`Put`；`stamps.go` 为文档 `_rev/_wid/_wids/_ts` 盖章与裁剪逻辑（Node sync-stamps 移植），`stamps_test.go` 是其纯函数用例 |
| `internal/httpx/` | CORS（预检 204 + Capacitor 源，与 Node `APP_ORIGINS` 同集）、CSRF（Node `csrfOk` 移植，Sec-Fetch-Site 优先）、`BodyGuard`（5MiB cap→413，坏 JSON→400）、`ConfigSvc`（`/api/config`） |
| `internal/media/`（4 文件，含 `media_test.go`） | 媒体四端点：PUT/GET `:hash`、`missing`、`sweep`；sha256 校验 + 魔数嗅探 + mp4 处理 |
| `internal/presence/` | 训练在线表：POST /api/activity 心跳（Redis TTL 70s/hidden 45s，Node 同口径）+ /api/admin/live SSE（15s 快照/10 分钟断开） |
| `internal/push/` | Web Push 七端点（webpush-go）；休息计时调度 |
| `internal/quota/`（含 `quota_test.go`） | coach 月配额：Redis 周期键 + 60s 落账 usage_counters（0008），限额来自 user_quotas |
| `internal/sms/` | 短信发送（env 配置 sender）+ Redis 验证码存取（`sms.Store{Rdb}`） |
| `internal/store/`（6 文件） | pgx 连接池 + users/states/invites/settings/identities 存取 + `Migrate()`（**`go:embed migrations/*.sql` + golang-migrate，启动自动 up，幂等**，无独立 migrate 容器） |
| `internal/testmode/` | Node 测试双跑支撑：`Setup`（每测试独立 PG 库 + secret）/`Seed`/`FreePort`/`BootLine`（启动行与 Node 同序，供测试解析） |
| `internal/wechat/` | 微信小程序码扫码登录（`WX_MINI_APPID`/`WX_MINI_SECRET` + Redis 态） |
| `internal/store/migrations/` | **13 个迁移版本（0001–0013，2026-11-18 复核）**：0001_init / 0002_users_sv / 0003_passkeys_meta / 0004_device_links / 0005_push_subs / 0006_admin_extra / 0007_passkeys_last_used（🆕 批次 2 幂等化修复，见 FAQ）/ 0008_usage_per_user / 0009_users_invited_by / 0010_instance_settings / 0011_admin_invites / 0012_auth_identities / **0013_agent_chat_memory（AI-Native 五表，0009 起带 down；顶层 `migrations/` 为同内容副本，两处都要放）** |
| `compose.go.yml` | **fork 专属本地栈**（上游 `docker-compose.yml` 不动，两栈并存）：pg:16-alpine + redis:7-alpine + api(3100→3110)。`docker compose -f compose.go.yml up -d --build` |
| `compose.weknora.yml` 🆕 | **WeKnora 独立部署编排**（批次 3）：仅内网无公网端口（D10）/ 独立 PG（D18）/ 一期关 LLM 生成（D19）/ Embedding-3 密钥 .env 注入 |
| `Dockerfile` | 多阶段 → **scratch 运行层**（静态单二进制 + CA，非 root 65534）；`GOPROXY=https://goproxy.cn,direct` |
| `scripts/gray-switch.sh` | P6 灰度闸门：`migrate`（备份+导入+round-trip 全等验收，不停服）/`switch`（停 Node→起 Go→health+人工登录冒烟）/`rollback` |

## 路由清单（main.go 装配，≈66 条已实现，2026-11-11 实测）

- `GET /api/health`（写不进 DATA_DIR / PG/Redis 不通 → 503，与 Node 同语义）
- **认证（2026-10-29 重构后 10 条）**：`GET /api/auth/captcha` · `POST /api/auth/sms/send` · `POST /api/auth/sms/verify` · `GET /api/auth/wechat/qrcode` · `GET /api/auth/wechat/poll` · `POST /api/auth/wechat/confirm` · `POST /api/auth/bind-phone` · `GET /api/me` · `POST /api/logout` · `POST /api/logout/all`。**passkey/密码/配对端点已退役**
- 数据 4：`GET /api/data`、`GET /api/data/rev`、`PUT /api/data`、`GET /api/config`
- Push 7：public-key / subscribe / status / unsubscribe / test / rest-timer / rest-timer/cancel
- Media 4：`PUT|GET /api/media/:hash`、missing、sweep
- Admin 20+：overview、users(搜索/筛选/分页)、user、user/disable、user/delete、quota GET/PUT、user activity/export、roles、roles/grant、roles/revoke、audit(+clear)、step-up、session、invites×3、settings×2、live SSE（角色：support 只读 → ops → super_admin；敏感操作挂 step-up）
- Presence：`POST /api/activity` + `GET /api/admin/live`
- **🆕 AI-Native 对话面 7（批次 2，main.go L192–198）**：`POST /api/coach/chat`（异步轮次）· `GET /api/coach/chat/:id/events`（SSE 回放+实时，tool_call 事件带 refs）· `POST /api/coach/chat/:id/confirm`（事务确认）· `GET /api/coach/memory` · `PUT /api/coach/memory/:id` · `DELETE /api/coach/memory/:id`（n/c 前缀双类）· `GET /api/coach/usage`（三池）。Provider 未配置 → 503 `{error:"coach off"}`（与批任务 `COACH_DISABLED` 语义呼应）
- **🆕 admin coach 4（批次 5/6，main.go L260–264）**：`GET /api/admin/coach/usage-overview`（support 只读）· `PUT /api/admin/coach/limits`（ops+step-up）· `GET /api/admin/coach/agent-config`（support 只读）· `PUT /api/admin/coach/agent-config`（ops+step-up）
- `/internal/coach/*` 回读面 4 条（INTERNAL_TOKEN 相认，未设 token 不注册）
- **NoRoute → 501 `{error:"planned for a later phase", code:"not-implemented"}`** —— 与 Node 侧 `notYetGo` stub 同语义

## 🆕 AI-Native 实施现状（批次 1–6 + 后段完毕，2026-11-11）

> 方案全文：《沐禾健身-AINative重构方案-v2.md》（`.zcf/`）；台账八行在 [FORK.md](../FORK.md) §5。本节是 Go 侧的部件地图——**规划态已清零，下表全部为已实现代码**。

### 内核装配链（main.go）

`llm.FromEnv()`（BIGMODEL_* 未配 → 503 off）→ `weknora.NewClient()`（WEKNORA_* 未配 → nil，检索工具不注册）→ `agent.DefaultTools(db, wkClient, WEKNORA_GLOBAL_KB)`（main.go L188）→ `agent.NewService(...)` 注入审核链/limits/agentconfig → 七端点挂路由。

### 工具注册表（tools_setup.go，6 工具，2026-11-18 实测）

| 工具 | 类别 | 说明 |
|------|------|------|
| `estimate_1rm` | 纯计算真工具 | onerm.js 翻译实现，golden 4480 案锚定 |
| `propose_constraint` | 提议（确认制载体） | 伤病黑名单等硬约束，用户确认后生效于所有计划生成且硬读取 |
| `propose_memory` | 提议（确认制载体） | 叙事/结构化记忆写入提议 |
| `get_week_plan` 🆕 | 数据读取（批次 6 后段"最小里程碑"） | queue.js 全文翻译 + history.js effectiveRoutineIds + mcp get_week_plan 组装层对齐（plannedBy 六分支 + days 7 天 + coach_week + weekdays 周模板行），plan golden 八场景全等（gen-plan.mjs 夹具） |
| `get_bodyweight` | 数据读取（批次 6 营养域 v1） | states doc 体重序列/目标差/单位，MCP 同口径排序 |
| `search_knowledge` | 知识检索（批次 3） | WeKnora 检索，globalKB 共享只读；未配置不注册=D15 降级；范围收敛记注释（记忆条目检索已由 system prompt 注入实现）；批次 6 后段加 `ExtractRefs` 抽动作 id → tool_call refs 动作卡 |

✅ **get_week_plan 已入注册表（2026-11-11 批次 6 后段，tools_setup.go L94 实测）**——"先查周计划再改计划"闭环补齐；两条实施教训入 FORK.md 台账：**golden 对照要连组装层一起锚定**（漏读 handler 尾部 weekdays 组装段，6/8 全等时才暴露）、**JS 未用局部在 Go 是编译错**（todayWd/weekdayIds 曾被误删）。

### 测试保护网（internal 侧）

- `onerm_golden_test.go`：4480 案等价 + selfcheck（≥7 个公式唯一值防"双端同名回退假绿"）+ 夹具厚度下限（≥1000 案/≥50 精度样）；夹具由 `scripts/golden/gen-onerm.mjs` 用 frontend 真函数生成，禁止手编
- `plan_golden_test.go` 🆕：八场景全等（planned_by 六分支 + queue 容错；firstDiffPath 字段级诊断）；夹具由 `scripts/golden/gen-plan.mjs` 用 **MCP 真 handler** 生成（时钟 monkey-patch 钉死、state.js 单例 DATA_DIR 固定），禁止手编
- `loop_test.go` / `review_test.go`：tool-loop 与审核链单测
- `session_pg_test.go`：会话状态机 PG 真库链路（`TEST_PG_ADMIN_URL`）
- `limits`/`overview`：`TestLimitsAndOverview` 真库全链（默认→设置→坏 JSON 回退→记账→全站聚合→趋势补零→限额回响）
- `weknora/client_test.go`：httptest 6 剧本
- 主测法仍是 **Node 套件 MUHE_GO 双跑**（29 个 MUHE_TEST 文件 + `coach-chat-off.test.js` 冒烟：Go 侧 503 off + memory·usage 真数据）

### 红线落实点

validate 安全边界语义（批任务翻译等价 + chat 走审核链）、零 agent 框架（`loop.go` 手写循环，三新包零新依赖）、未经确认写入为零（`propose_*` 永不执行 + `coach_proposals` 队列 + confirm 事务）、健康数据不出实例。

## 数据模型要点

- `states` 表：`doc` 存**整份** JSON 文档（与 Node `state-<uid>.json` 同构，含 `_rev/_wid/_wids/_ts`），`rev` 列是投影（`GET /api/data/rev` 免解析整文档）
- 乐观并发：`PutStateDoc(doc, expect)` —— expect=0 首建 INSERT … ON CONFLICT DO NOTHING；expect>0 条件 UPDATE；冲突回 `ErrConflict`（409 带文档）
- 会话 HMAC 密钥：env `SECRET` → `DATA_DIR/secret` 文件 → dev 兜底。**灰度切换时两栈必须同钥，cookie 才互通**（gray-switch 的前提之一）
- 0012_auth_identities：认证重构后的身份表（手机号/微信 openid ↔ uid）
- 🆕 **0013_agent_chat_memory（AI-Native 五表）**：`coach_chat_sessions`（会话状态机 + history jsonb + usage_in/out）· `coach_proposals`（提议队列，decision NULL=待决——**Agent 唯一写入口**）· `coach_constraints`（结构化硬约束，active 部分索引，删除=inactive 留审计）· `coach_memory_narrative`（叙事记忆，批次 3 起主写 WeKnora、本表为 D15 降级镜像）· `coach_usage_events`（三池用量流水）。用户 id 全库惯例 `text REFERENCES users(id)`

## 与 Node api 的关系（双栈并存，生产 = Go 栈）

| 维度 | Node（`api/`） | Go（本模块） |
|------|---------------|--------------|
| 契约 | `api/openapi.yaml`（唯一契约源，title 沐禾健身 API，90 路径/100 操作；api.html 计数器显 107） | oapi-codegen 从同一文件生成 |
| 未实现端点 | fork 新增端点挂 `notYetGo` 501（当前 **32 键**，含 AI-Native 对话面 11） | NoRoute 一律 501 |
| 认证 | passkey/密码（上游体系保留） | **2026-10-29 重构：手机号+短信 / 微信扫码；passkey/密码/配对退役** |
| AI Coach | 批任务 `coach/`（**停止演进，契约参考**） | 🆕 对话式 Agent 内核 `internal/agent`（生产实现） |
| 测试 | `npm test`（75 test 文件 + 3 辅件，29 个 MUHE_TEST + coach-chat-off 冒烟） | `stamps_test.go` / `media_test.go` / `quota_test.go` 纯函数保护网 + `internal/agent` 全套 + 双 golden；主测法为 Node 套件双跑 |
| 存储 | JSON 文件 `./data` | PG（迁移内嵌 0001–0013）+ 媒体仍走 DATA_DIR 文件 |
| 端口 | 3000 | 3100（compose 映射 3110） |

**双跑机制**：`api/test/` 75 个测试文件中 29 个带 `MUHE_TEST=1` 启动分支；设 `MUHE_GO=1` 跑 `npm test` 时，用例改为拉起 Go 二进制（`testmode.BootLine` 输出端口行，与 Node 同序），**同一批断言对两栈各跑一遍** —— Go 侧的"单测"就在 Node 套件里。前置：本地有 PG+Redis（`compose.go.yml` 起）+ `go build` 出 `muhe-api`。

## 常见问题 (FAQ)

- **`go build` 失败？** `internal/data` 曾被 .gitignore 裸 `data/` 规则吞包（2026-10-28 已修复并锚根 `/data/`）；同类症状先跑 `git check-ignore -v <path>`。
- **Coach 503 off？** `BIGMODEL_API_KEY` 未配置时 `llm.FromEnv` 返回 nil provider，对话面七端点一律 503 `{error:"coach off"}`——这是**特性不是故障**（本地无 key 也能起服务）；⚠️ 实现注意：FromEnv **必须返回接口类型**，`(*Client)(nil)` 装进接口后 `==nil` 恒 false（typed-nil 陷阱，批次 2 曾致 503 off 失效，已根治为返回接口）。
- **全新库迁移炸在 0007？** 历史 bug：0007 与 0003 重复给 passkeys 加 `last_used` 列且无 `IF NOT EXISTS`——老库靠版本号跳过掩盖，全新库顺序迁移必炸；批次 2（2026-11-11）已幂等化修复。新迁移写 `ALTER TABLE` 一律带 `IF NOT EXISTS`/`IF EXISTS`。
- **pgx5://**：golang-migrate 的 pgx/v5 driver 用 `pgx5://` scheme，`main.go` 的 `pgx5URL()` 负责把习惯的 `postgres://` 转换。
- **为什么 scratch 运行层**：本地拉不到 distroless（gcr），Go 静态编译的正统替代就是 scratch + CA 证书。
- **加新端点流程**：改 `api/openapi.yaml`（契约先行）→ `oapi-codegen` 重生成 `internal/apigen/gen.go` → 在对应 internal 包写 handler → `cmd/api/main.go` 路由表注册（按阶段挂角色/step-up 中间件）→ Node 套件补双跑用例。AI-Native 对话面七端点即按此流程落地（先 501 stub 后 Go 实现）。
- **Redis 到底干嘛**：健康探针 + presence 心跳 + 短信验证码/微信扫码态（认证重构后）；challenge/pair 内存 store 已随 passkey 退役。若要跨实例部署，检查这几块是否都需要迁移。
- **迁移文件放哪**：`internal/store/migrations/`（go:embed 生效处）+ 顶层 `migrations/`（同内容副本），**两处都要放**（历史惯例，勿只放一处；0013 两处已核对齐）。
- **WeKnora 什么时候生效**：`WEKNORA_BASE_URL` + `WEKNORA_API_KEY` 配齐才有检索工具（`search_knowledge`），`WEKNORA_GLOBAL_KB` 指共享知识区 kb id；编排见 `compose.weknora.yml`（仅内网/独立 PG/一期关生成），ETL 见 `scripts/weknora-import/import-catalogue.mjs`（dry-run / 断点续跑 / 媒体永不进管道）。

## 相关文件清单

`cmd/api/main.go` · `cmd/muhemigrate/main.go` · `oapi-codegen.yaml` · `compose.go.yml` · `compose.weknora.yml`（🆕）· `Dockerfile` · `go.mod`（直接依赖：gin / pgx/v5 / go-redis / go-webauthn / webpush-go / golang-migrate / oapi-codegen runtime，AI-Native 三新包零新增）· `internal/`（19 包，含 🆕 agent/llm/weknora）· `internal/agent/testdata/onerm-golden.json`（4480 案，禁止手编）· `internal/agent/testdata/plan-golden.json`（八场景，禁止手编）· `migrations/`（0001–0013，两处副本共 36 SQL）· `scripts/gray-switch.sh` · 上游对照：[api/CLAUDE.md](../api/CLAUDE.md)（批任务 Coach / 契约冻结）

---

变更记录: 2026-11-18 V1.4 审查轮核准（init-architect）:① **get_week_plan 已入注册表**（批次 6 后段"最小里程碑"，tools_setup.go L94 实测 6 工具）——V1.3 的"⚠️ 尚未入注册表"过时已改；`plan.go` + `plan_golden_test.go` + `testdata/plan-golden.json` 入目录地图，agent 包 13→**15 .go**；② 数字修正：总文件 ~96→**~100**（54 .go = internal 52 + cmd 2 + testdata×2）、Node 套件 76→**75 test 文件**、"107 端点"澄清（90 路径/100 操作）；③ 批次 6 后段补记：`Tool.ExtractRefs` 动作卡 refs、推进表 AI-Native 行加"后段"（台账八行为准）；④ 四路专项审查修复方案见 `.zcf/沐禾健身-全面审查修复方案-20261118.md`。2026-11-11 V1.3 增量（init-architect，实施后刷新轮）:"AI-Native 排期规划"节改写为"实施现状"——批次 1–6 全落地：`internal/llm`（provider 抽象 + BIGMODEL GLM 客户端，FromEnv 返回接口根治 typed-nil）+ `internal/agent`（loop/tools_setup/review/session/handlers/limits/agentconfig/onerm + golden 4480 案）+ `internal/weknora`（D14 强制 kbIDs）三新包；0013 五表迁移（12→13 版本，两处 36 SQL）；路由 ≈55→≈66（对话面 7 + admin coach 4）；internal 16→19 包、总文件 ~72→~96（52 .go）；测试保护网补 internal/agent 全套；推进表加 AI-Native 批次行；FAQ 新增 typed-nil 陷阱、0007 迁移幂等 bug（批次 2 修复）、Coach 503 off 语义、WeKnora 生效条件四则；如实记缺口——get_week_plan 等计划读取工具未入注册表。2026-11-10 V1.2 增量:新增 AI-Native 排期规划节（当时规划态）；修正漂移——internal 16 包 32 文件、迁移 12 版本、~72 文件、路由 ≈55；补记认证重构；2026-10-28 V1.1 缺口修复（.gitignore 裸 data/ 吞包）；2026-10-27 V1.0 初版
