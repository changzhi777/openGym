[根目录](../CLAUDE.md) > **scripts · 构建/校验工具链**

# scripts/ — 仓库级工具链

> 33 个文件：目录库构建与校验、coach 资产构建、API 文档生成、媒体打包、翻译名称源数据。全部为开发期工具，不进任何运行时镜像。

## 目录结构

### catalogue/（目录库流水线，11 个脚本）

| 脚本 | 职责 |
|------|------|
| `validate.mjs` | 校验每个 `catalogue/exercises/*.json`（词表在脚本顶部——**词表即规范**）；PR check 用的就是它 |
| `build.mjs` | 重生成应用数据文件（frontend 消费）与 `catalogue/browse/` |
| `import-drafts.mjs` / `import-legacy.mjs` / `import-names.mjs` / `drafts-from-pack.py` | 批量导入（草稿/legacy/名称） |
| `check-media.mjs` / `stage-media.mjs` | 媒体一致性核查 / 移动与 demo 构建的媒体暂存 |
| `migration-check.mjs` | 目录迁移一致性 |

### media/（2 个 Python）

`build-media.py` / `build-app-webp.py` — 媒体资产打包与 WebP 生成（授权媒体处理，谨慎使用）。

### 根级（构建与生成）

| 脚本 | 职责 |
|------|------|
| `build-api-docs.mjs` | `api/openapi.yaml` → `website/api.html`（改 spec 必跑） |
| `build-coach-assets.mjs` / `build-coach-library.mjs` | coach 提示词/库资产构建（CI test.yml 盯着这三个 build-*） |
| `build-hevy-id-map.mjs` | Hevy 导入的 id 映射再生（需 `HEVY_API_KEY`，见 .env.example 尾部） |
| `de-name-rules.mjs` | 德语名称规则处理 |
| `ci/fork-mr-pipelines.py` | GitLab 侧 fork MR 流水线辅助 |

### 数据源目录（人工维护的参考数据）

- `exercise-name-sources/` — 各语言动作名源（9 语言 JSON + 俄语术语表 + README）
- `instruction-sources/` — 说明翻译源（含 TRANSLATION_PLAN、SOURCE_DIVERGENCES 差异记录）

## 使用时机

| 你改了… | 必须跑 |
|---------|-------|
| `api/openapi.yaml` 或 api 路由 | `node scripts/build-api-docs.mjs`（同提交 api.html） |
| `catalogue/**` | `node scripts/catalogue/validate.mjs` + `build.mjs`（或留给维护者） |
| coach prompts / 库 | `build-coach-assets.mjs` / `build-coach-library.mjs`（CI 会验） |
| Hevy 映射过期 | `HEVY_API_KEY=… node scripts/build-hevy-id-map.mjs` |

---

变更记录: 2026-10-20 V1.0 初版（init-architect）
