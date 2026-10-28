[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **scripts · 构建/校验/部署/测试工具链**

# scripts/ — 仓库级工具链

> 36 个文件：目录库构建/校验/批量中文化、coach 资产构建、API 文档生成、媒体打包、翻译名称源数据，🆕 fork 新增部署与测试门脚本。全部为开发期工具，不进任何运行时镜像。

## 目录结构

### catalogue/（目录库流水线，11 个脚本）

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

### 根级（构建、部署与测试门）

| 脚本 | 职责 |
|------|------|
| `build-api-docs.mjs` | `api/openapi.yaml`（title 沐禾健身 API）→ `website/api.html`（改 spec 必跑，501 stub 端点也在契约内） |
| `build-coach-assets.mjs` / `build-coach-library.mjs` | coach 提示词/库资产构建（CI test.yml 盯着；中文动作包 `instructions-zh.js` 随说明中文化同步再生） |
| `build-hevy-id-map.mjs` | Hevy 导入的 id 映射再生（需 `HEVY_API_KEY`） |
| 🆕 `test-all.sh` | **fork 测试门**（FORK.md §6 口径）：钉死 Node 22（自动探测 homebrew node@22）+ `TZ=UTC`，三套全跑 `./scripts/test-all.sh [api|frontend|mcp]`。两个钉死变量不是玄学：Node≥24 的实验性全局 localStorage 遮蔽 happy-dom（frontend 大面积红）；UTC+8 时区让 demoSeed 多生成"今天"训练（mcp/frontend 共 7 红）。基线：api 605 / frontend 5060 / mcp 95 全绿 |
| 🆕 `deploy-ct151.sh` | **fork 生产部署**：从本机更新 PVE CT 151（openGym-allinone @ 10.10.10.151，需在内网）。幂等：tar 打包（排 node_modules/.git/data/dist/.env）→ PVE → CT 解包 → `npm ci` + systemd 重启 → 前端构建 → nginx 首装+站点配置。`--api-only` 跳过前端 |
| `de-name-rules.mjs` | 德语名称规则处理 |
| `ci/fork-mr-pipelines.py` | GitLab 侧 fork MR 流水线辅助 |

### 数据源目录（人工维护的参考数据）

- `exercise-name-sources/` — 各语言动作名源（9 语言 JSON + 俄语术语表 + README）
- `instruction-sources/` — 说明翻译源（含 TRANSLATION_PLAN、SOURCE_DIVERGENCES 差异记录）

## 使用时机

| 你改了… | 必须跑 |
|---------|-------|
| 提交前（任何改动） | `./scripts/test-all.sh`（fork 测试门口径） |
| `api/openapi.yaml` 或 api 路由 | `node scripts/build-api-docs.mjs`（同提交 api.html） |
| `catalogue/**` 或跑了 translate-zh | `node scripts/catalogue/validate.mjs` + `build.mjs` |
| coach prompts / 库 | `build-coach-assets.mjs` / `build-coach-library.mjs`（CI 会验） |
| 上线 | `./scripts/deploy-ct151.sh`（全量或 --api-only） |
| Hevy 映射过期 | `HEVY_API_KEY=… node scripts/build-hevy-id-map.mjs` |

## 相关模块

Go 栈的灰度切换脚本在 [backend-go/scripts/gray-switch.sh](../backend-go/CLAUDE.md)（migrate/switch/rollback），不在本目录。

---

变更记录: 2026-10-27 V1.1 增量（init-architect）:新增 translate-zh.mjs / test-all.sh / deploy-ct151.sh，33→36 文件；2026-10-20 V1.0 初版
