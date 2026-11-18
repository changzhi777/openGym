[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **scripts · 构建/校验/部署/测试工具链**

# scripts/ — 仓库级工具链

> 42 个文件（2026-11-18 复核）：目录库构建/校验/批量中文化、coach 资产构建、API 文档生成、媒体打包、翻译名称源数据、AI-Native golden 夹具与 WeKnora ETL，fork 部署（118 现行 + CT151 旧）与测试门脚本。全部为开发期工具，不进任何运行时镜像。

## 目录结构

### catalogue/（目录库流水线，10 个脚本）

| 脚本 | 职责 |
|------|------|
| `validate.mjs` | 校验每个 `catalogue/exercises/*.json`（词表在脚本顶部——**词表即规范**）；PR check 用的就是它 |
| `build.mjs` | 重生成应用数据文件（frontend 消费，含 `instr/zh.js` / `exercise-desc/zh.js`）与 `catalogue/browse/` |
| 🆕 `translate-zh.mjs` | **fork 专属**：批量补全动作中文说明（OpenAI 兼容 API，默认 DeepSeek；`TRANSLATE_API_KEY` 必填，`--limit N` 试跑）。断点续跑——每批写回 `catalogue/i18n/zh.json`（唯一真相源），重跑自动跳过已有译文；术语表与 `frontend/docs/i18n-terminology-audit.md` 对齐。**跑完必须 `build.mjs`** |
| `import-drafts.mjs` / `import-legacy.mjs` / `import-names.mjs` / `drafts-from-pack.py` | 批量导入（草稿/legacy/名称） |
| `check-media.mjs` / `stage-media.mjs` | 媒体一致性核查 / 移动与 demo 构建的媒体暂存（APK 动画全靠 stage-media） |
| `migration-check.mjs` | 目录迁移一致性 |

### media/（2 个 Python）

`build-media.py` / `build-app-webp.py` — 媒体资产打包与 WebP 生成（授权媒体处理，谨慎使用）。

### 🆕 golden/（AI-Native golden 夹具生成，2 脚本）

- `gen-onerm.mjs` — 用 **frontend 真函数**生成 `backend-go/internal/agent/testdata/onerm-golden.json`（4480 案，含 selfcheck 防双端同名回退假绿；改 `frontend/src/lib/onerm.js` 后必跑 + `go test ./internal/agent/`）
- `gen-plan.mjs` — 用 **MCP 真 handler** 生成 `testdata/plan-golden.json`（八场景覆盖 planned_by 六分支 + queue 容错；时钟 monkey-patch 钉死、state.js 单例 DATA_DIR 固定；改 `queue.js`/`history.js` 后必跑）

### 🆕 weknora-import/（catalogue → WeKnora ETL，1 脚本）

`import-catalogue.mjs` — 动作目录灌入 WeKnora 知识库：dry-run / 断点续跑 manifest / token 额度估算打印；**媒体永不进管道**、action id 进文档正文；zh.json 5,632 条资产复用（50 篇试跑 ≈13k token，全量 6,554 篇实施前已核算）。

### 根级（构建、部署与测试门）

| 脚本 | 职责 |
|------|------|
| `build-api-docs.mjs` | `api/openapi.yaml`（title 沐禾健身 API）→ `website/api.html`（改 spec 必跑，501 stub 端点也在契约内） |
| `build-coach-assets.mjs` / `build-coach-library.mjs` | coach 提示词/库资产构建（CI test.yml 盯着；中文动作包 `instructions-zh.js` 随说明中文化同步再生） |
| `build-hevy-id-map.mjs` | Hevy 导入的 id 映射再生（需 `HEVY_API_KEY`） |
| 🆕 `test-all.sh` | **fork 测试门**（FORK.md §6 口径）：钉死 Node 22（自动探测 homebrew node@22）+ `TZ=UTC`，三套全跑 `./scripts/test-all.sh [api|frontend|mcp]`。两个钉死变量不是玄学：Node≥24 的实验性全局 localStorage 遮蔽 happy-dom（frontend 大面积红）；UTC+8 时区让 demoSeed 多生成"今天"训练（mcp/frontend 共 7 红）。基线：api 605 / frontend 5060 / mcp 95 全绿 |
| `deploy-ct151.sh` | **fork 生产部署（旧）**：从本机更新 PVE CT 151（openGym-allinone @ 10.10.10.151，需在内网）。幂等：tar 打包（排 node_modules/.git/data/dist/.env）→ PVE → CT 解包 → `npm ci` + systemd 重启 → 前端构建 → nginx 首装+站点配置。`--api-only` 跳过前端 |
| 🆕 `deploy-118.sh` | **fork 生产部署（现行，qingmulife.com @ 43.129.201.118）**：Go 栈 compose build+up + 前端 dist rsync + 双域 health 验证，幂等。`--web-only` 跳过 API。⚠️ bsdtar 坑：exclude 带 `./` 前缀会匹配任意路径段（曾吞 internal/data，见 FORK.md 台账）。数据备份 = 服务器 /opt/muhe-fit/data + pg 卷 |
| `de-name-rules.mjs` | 德语名称规则处理 |
| `ci/fork-mr-pipelines.py` | GitLab 侧 fork MR 流水线辅助 |

### 数据源与配置目录（人工维护）

- `exercise-name-sources/` — 各语言动作名源（8 语言 JSON + 俄语术语表 + README，10 文件）
- `instruction-sources/` — 说明翻译源（含 TRANSLATION_PLAN、SOURCE_DIVERGENCES 差异记录，5 文件）
- 🆕 `nginx-118/` — 生产 118 双域 nginx 配置（`qingmulife.com.conf` 裸域前端 + `api.qingmulife.com.conf` API 直连）

## 使用时机

| 你改了… | 必须跑 |
|---------|-------|
| 提交前（任何改动） | `./scripts/test-all.sh`（fork 测试门口径） |
| `api/openapi.yaml` 或 api 路由 | `node scripts/build-api-docs.mjs`（同提交 api.html） |
| `catalogue/**` 或跑了 translate-zh | `node scripts/catalogue/validate.mjs` + `build.mjs` |
| coach prompts / 库 | `build-coach-assets.mjs` / `build-coach-library.mjs`（CI 会验） |
| 🆕 frontend `onerm.js` / `queue.js` / `history.js` 算法 | `node scripts/golden/gen-onerm.mjs`（或 `gen-plan.mjs`）再 `go test ./internal/agent/`（golden 双端等价，禁手编夹具） |
| 上线 | `./scripts/deploy-118.sh`（现行生产；`deploy-ct151.sh` 为旧内网实例保留） |
| Hevy 映射过期 | `HEVY_API_KEY=… node scripts/build-hevy-id-map.mjs` |

## 相关模块

Go 栈的灰度切换脚本在 [backend-go/scripts/gray-switch.sh](../backend-go/CLAUDE.md)（migrate/switch/rollback），不在本目录。

---

变更记录: 2026-11-18 V1.2 审查轮核准（init-architect）:37→**42 文件**（补 golden/ 2 + weknora-import/ 1 + nginx-118/ 2——台账批次 2/3/6 后段增量入目录结构）；catalogue 脚本 11→**10**；exercise-name-sources 9→**8** 语言 JSON；使用时机表补 golden 双跑行、上线入口改 **deploy-118.sh（现行）**（CT151 降旧，FORK.md §5 2026-10-29 行为准）。2026-10-27 V1.1 增量（init-architect）:新增 translate-zh.mjs / test-all.sh / deploy-ct151.sh，33→36 文件；2026-10-20 V1.0 初版
