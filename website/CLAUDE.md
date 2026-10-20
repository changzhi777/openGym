[根目录](../CLAUDE.md) > **website · opengym.ch 官网**

# website/ — 项目官网（opengym.ch）

> 手写 HTML/CSS/JS 静态站，**无构建步骤**，nginx 服务。与产品代码完全解耦，单独部署（GitHub Pages 由 `pages.yml` 发布，主站另有部署）。

## 页面清单

| 文件 | 内容 |
|------|------|
| `index.html` | 首页（hero、特性、demo iframe 区） |
| `docs.html` | 文档页（自托管指南等的站内镜像，带目录抽屉 + scrollspy） |
| `about.html` | 关于页（含 GitHub API 拉取的 release 时间线） |
| `api.html` | **唯一生成物**：`node scripts/build-api-docs.mjs` 从 `../api/openapi.yaml` 生成——**勿手编**，改 spec 后重跑脚本并同提交 |
| `styles.css` / `site.js` | 全站样式 / 五个独立软失败组件：目录抽屉、scrollspy、滚动显现、demo iframe（懒注入、<700px 降级为卡片）、GitHub star/issue 数（requestIdleCallback） |
| `llms.txt` / `robots.txt` / `sitemap.xml` | 机器可读索引 |
| `build-images.sh` | 从 `../assets/screenshots/` 生成多分辨率 PNG/WebP 到部署期 `img/` |
| `README.md` | 本目录机制说明（缓存策略、?v=N bust、可访问性） |

## 部署期注入（不进 git）

`img/`（build-images.sh 产物）、`icon-180/512.png`（来自 `../frontend/public/`）、`openGym.apk`（签名发布构建）、`demo/`（`VITE_DEMO=1` 的浏览器 demo 构建，内嵌 exercise-media，因 X-Frame-Options: SAMEORIGIN 必须同域）。

## 注意

- 截图同名更新 + nginx 图片缓存 7 天 → 需立即生效就改文件名
- `styles.css`/`site.js` 改动必须 bump `?v=N`（防 Cloudflare 边缘旧配新）
- 每页有 "Skip to content"（`.skip`）无障碍入口

---

变更记录: 2026-10-20 V1.0 初版（init-architect）
