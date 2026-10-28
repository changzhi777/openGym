[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **backend-go**

# backend-go/ — 沐禾健身 Go 版后端（fork 专属）

> openGym（Node 无框架版）的 **Go 重写栈**：Gin + PostgreSQL(pgx) + Redis，**迁移内嵌随进程跑**，部署面只剩 PG + Redis + 本进程。这是 fork（沐禾健身 / changzhi777/openGym）的 SaaS 化主战场；上游 `api/`（Node）保持可运行作为对照与回滚源。

## 模块职责与推进状态

按 P0 → P6 推进（方案：`.zcf/plan/current/saas-backend-go.md`，fork 本地）：

| 阶段 | 内容 | 状态 |
|------|------|------|
| P0 | 契约冻结：`api/openapi.yaml` 为唯一契约源 | ✅ |
| P1 | 骨架：Gin + pgx + go-redis + `/api/health`（探活含 PG/Redis 双写检） | ✅ |
| P2 | 认证闭环：passkey 注册/登录、密码外配对、passkey 管理、设备配对（~17 端点） | ✅ |
| P3 | 数据核心：`/api/data` GET/PUT + `/api/data/rev` + `/api/config`；`muhemigrate` 双向迁移器（round-trip 达标） | ✅（2026-10-28 缺口已修复，见下） |
| P4 | Web Push 七端点（VAPID 与 Node 共用 `data/vapid.json`）+ 媒体四端点（sha256 校验 + 魔数嗅探 + `<DATA_DIR>/media/<uid>/<sha>` 布局与 Node 兼容） | ✅ |
| P5 | 管理后台：九端点 + 角色分层（support 只读 / ops 运营 / super_admin 全权）+ 敏感操作 step-up + 审计 | ✅ |
| P6 | 灰度演练：`scripts/gray-switch.sh`（备份 → 导入 → round-trip 验收 → 切换 → 回滚） | ✅ 已演练 |

**✅ 编译缺口已修复（2026-10-28）**：`internal/data`（P3a 数据三端点的 `data.Service`）代码早已写完，但被根 `.gitignore` 裸写的 `data/` 规则（无根锚定、任意深度匹配）整包静默吞掉——`git add` 跳过无报错、全历史零踪迹，实体仅存于主 checkout 工作区。已拷回 3 文件（handlers/stamps/stamps_test，592 行）并把规则锚根为 `/data/`；`go build ./...` 与 `go test ./internal/data/` 全绿，2026-10-28 随本仓提交恢复入库。教训：新目录提交前跑 `git check-ignore -v` 或核对 `git status`，ignore 规则一律根锚定。

## 目录地图（41 文件）

| 路径 | 内容 |
|------|------|
| `cmd/api/main.go` | 装配入口：env 读取（与 Node 同名同义：`RP_NAME`(默认"沐禾健身")/`RP_ID`/`ORIGIN`/`SESSION_DAYS`/`SECRET`/`DATA_DIR`/`PORT`(默认 **3100**)/`DATABASE_URL`/`REDIS_ADDR`）+ 中间件（CORS→CSRF→BodyGuard）+ 全部路由表 + testmode 双跑钩子 |
| `cmd/muhemigrate/main.go` | **双向迁移器**（P3b）：`import <dataDir>` 把 Node 的 `db.json`(users/creds) + `state-<uid>.json` 灌进 PG；`export <outDir>` 反向导出同构文件供比对/回滚。按 id upsert 幂等，passkey 材料全程内存不落盘 |
| `internal/auth/`（5 文件） | WebAuthn（go-webauthn 封装）+ 会话（HMAC cookie，与 Node 同钥互认）+ challenge/pair **内存 store** + 各 handler |
| `internal/admin/`（2 文件） | 管理后台 service + handlers：角色三层 + `RequireStepUp()` + 审计 |
| `internal/apigen/gen.go` | **oapi-codegen 生成物，勿手编**（配置 `oapi-codegen.yaml`：从根 `api/openapi.yaml` 生成 models + gin-server，strict-server 关） |
| `internal/cache/` | go-redis 封装。**现状只做健康探针**；业务态（challenge/pair）用内存 store，重启即失 |
| `internal/data/`（3 文件） | P3a 数据三端点 handler：`Get`/`GetRev`/`Put`；`stamps.go` 为文档 `_rev/_wid/_wids/_ts` 盖章与裁剪逻辑（Node sync-stamps 移植），`stamps_test.go` 是其纯函数用例（Node `test/sync-stamps.test.js` 核心用例翻译） |
| `internal/httpx/` | CORS（预检 204 + Capacitor 源，与 Node `APP_ORIGINS` 同集）、CSRF（Node `csrfOk` 移植，Sec-Fetch-Site 优先）、`BodyGuard`（5MiB cap→413，坏 JSON→400）、`ConfigSvc`（`/api/config`） |
| `internal/media/` | 媒体四端点：PUT/GET `:hash`、`missing`、`sweep`；sha256 校验 + 魔数嗅探 |
| `internal/push/` | Web Push 七端点（webpush-go）；休息计时调度 |
| `internal/store/` | pgx 连接池 + users/states 存取 + `Migrate()`（**`go:embed migrations/*.sql` + golang-migrate，启动自动 up，幂等**，无独立 migrate 容器） |
| `internal/testmode/` | Node 测试双跑支撑：`Setup`（每测试独立 PG 库 + secret）/`Seed`/`FreePort`/`BootLine`（启动行与 Node 同序，供测试解析） |
| `internal/store/migrations/` | 6 个迁移：0001_init / 0002_users_sv / 0003_passkeys_meta / 0004_device_links / 0005_push_subs / 0006_admin_extra（顶层 `migrations/` 为同内容副本） |
| `compose.go.yml` | **fork 专属本地栈**（上游 `docker-compose.yml` 不动，两栈并存）：pg:16-alpine + redis:7-alpine + api(3100→3110)。`docker compose -f compose.go.yml up -d --build` |
| `Dockerfile` | 多阶段 → **scratch 运行层**（静态单二进制 + CA，非 root 65534）；`GOPROXY=https://goproxy.cn,direct` |
| `scripts/gray-switch.sh` | P6 灰度闸门：`migrate`（备份+导入+round-trip 全等验收，不停服）/`switch`（停 Node→起 Go→health+人工登录冒烟）/`rollback` |

## 路由清单（main.go 装配，≈45 条已实现）

- `GET /api/health`（写不进 DATA_DIR / PG/Redis 不通 → 503，与 Node 同语义）
- 认证/配对 17：register·options/verify、login·options/verify、me、logout、logout/all、pair·create/redeem、account/passkeys(+options/verify/rename/DELETE)、account/device-link、device-link·options/verify
- 数据 4：`GET /api/data`、`GET /api/data/rev`、`PUT /api/data`、`GET /api/config`
- Push 7：public-key / subscribe / status / unsubscribe / test / rest-timer / rest-timer/cancel
- Media 4：`PUT|GET /api/media/:hash`、missing、sweep
- Admin 12：overview、users、user、user/disable、user/delete、quota GET/PUT、roles、roles/grant、roles/revoke、audit-log、step-up（角色：support 只读 → ops → super_admin；disable/delete/quota PUT/grant/revoke 挂 step-up）
- **NoRoute → 501 `{error:"planned for a later phase", code:"not-implemented"}`** —— 与 Node 侧 `notYetGo` stub 同语义

## 数据模型要点

- `states` 表：`doc` 存**整份** JSON 文档（与 Node `state-<uid>.json` 同构，含 `_rev/_wid/_wids/_ts`），`rev` 列是投影（`GET /api/data/rev` 免解析整文档）
- 乐观并发：`PutStateDoc(doc, expect)` —— expect=0 首建 INSERT … ON CONFLICT DO NOTHING；expect>0 条件 UPDATE；冲突回 `ErrConflict`（409 带文档）
- 会话 HMAC 密钥：env `SECRET` → `DATA_DIR/secret` 文件 → dev 兜底。**灰度切换时两栈必须同钥，cookie 才互通**（gray-switch 的前提之一）

## 与 Node api 的关系（双栈并存，切换未执行）

| 维度 | Node（`api/`） | Go（本模块） |
|------|---------------|--------------|
| 契约 | `api/openapi.yaml`（唯一契约源，title 沐禾健身 API） | oapi-codegen 从同一文件生成 |
| 未实现端点 | fork 新增 8 个 SaaS 端点挂 `notYetGo` 501 | NoRoute 一律 501 |
| 测试 | `npm test`（74 文件） | `internal/data/stamps_test.go` 纯函数保护网；主测法为 Node 套件双跑 |
| 存储 | JSON 文件 `./data` | PG（迁移内嵌）+ 媒体仍走 DATA_DIR 文件 |
| 端口 | 3000 | 3100（compose 映射 3110） |

**双跑机制**：`api/test/` 74 个测试文件中 26 个带 `MUHE_TEST=1` 启动分支；设 `MUHE_GO=1` 跑 `npm test` 时，用例改为拉起 Go 二进制（`testmode.BootLine` 输出端口行，与 Node 同序），**同一批断言对两栈各跑一遍** —— Go 侧的"单测"就在 Node 套件里。前置：本地有 PG+Redis（`compose.go.yml` 起）+ `go build` 出 `muhe-api`。

## 常见问题 (FAQ)

- **`go build` 失败？** `internal/data` 曾被 .gitignore 裸 `data/` 规则吞包（2026-10-28 已修复并锚根 `/data/`）；同类症状先跑 `git check-ignore -v <path>`。
- **pgx5://**：golang-migrate 的 pgx/v5 driver 用 `pgx5://` scheme，`main.go` 的 `pgx5URL()` 负责把习惯的 `postgres://` 转换。
- **为什么 scratch 运行层**：本地拉不到 distroless（gcr），Go 静态编译的正统替代就是 scratch + CA 证书。
- **加新端点流程**：改 `api/openapi.yaml`（契约先行）→ `oapi-codegen` 重生成 `internal/apigen/gen.go` → 在对应 internal 包写 handler → `cmd/api/main.go` 路由表注册（按阶段挂角色/step-up 中间件）→ Node 套件补双跑用例。
- **Redis 到底干嘛**：目前只有健康探针；challenge/pair 是内存 store。若要跨实例部署，先把这两块搬进 Redis。

## 相关文件清单

`cmd/api/main.go` · `cmd/muhemigrate/main.go` · `oapi-codegen.yaml` · `compose.go.yml` · `Dockerfile` · `go.mod`（直接依赖：gin / pgx/v5 / go-redis / go-webauthn / webpush-go / golang-migrate / oapi-codegen runtime）· `internal/`（10 包 24 文件，含 data）· `migrations/`×6 · `scripts/gray-switch.sh` · 上游对照：[api/CLAUDE.md](../api/CLAUDE.md)

---

变更记录: 2026-10-27 V1.0 初版（init-architect，P0–P6 现状扫描；internal/data 缺口如实记录）· 2026-10-28 V1.1 缺口修复（.gitignore 裸 `data/` 吞包根因定位 → 拷回 3 文件 + 锚根 `/data/` → build/test 全绿，见上方✅块）
