[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **website · 项目官网**

# website/ — 项目官网（opengym.ch 源站结构）

> 手写 HTML/CSS/JS 静态站，**无构建步骤**，nginx 服务。与产品代码完全解耦，单独部署（GitHub Pages 由上游 `pages.yml` 发布）。本轮核对：结构无变化；`api.html` 已随"沐禾健身 API"契约再生。

## 页面清单

| 文件 | 内容 |
|------|------|
| `index.html` | 首页（hero、特性、demo iframe 区） |
| `docs.html` | 文档页（自托管指南等的站内镜像，带目录抽屉 + scrollspy） |
| `about.html` | 关于页（含 GitHub API 拉取的 release 时间线） |
| `api.html` | **唯一生成物**：`node scripts/build-api-docs.mjs` 从 `../api/openapi.yaml` 生成（契约 title 现为**沐禾健身 API**，含 fork 新增的 SaaS 后台端点）——**勿手编**，改 spec 后重跑脚本并同提交 |
| `styles.css` / `site.js` | 全站样式 / 五个独立软失败组件：目录抽屉、scrollspy、滚动显现、demo iframe、GitHub star/issue 数 |
| `llms.txt` / `robots.txt` / `sitemap.xml` | 机器可读索引 |
| `build-images.sh` | 从 `../assets/screenshots/` 生成多分辨率 PNG/WebP 到部署期 `img/` |
| `README.md` | 本目录机制说明（缓存策略、?v=N bust、可访问性） |

## 部署期注入（不进 git）

`img/`（build-images.sh 产物）、`icon-180/512.png`（来自 `../frontend/public/`，已是沐禾健身图标）、`openGym.apk`（签名发布构建；fork APK 为 com.muhe.fit）、`demo/`（`VITE_DEMO=1` 浏览器 demo 构建，必须同域）。

## 注意

- 截图同名更新 + nginx 图片缓存 7 天 → 需立即生效就改文件名
- `styles.css`/`site.js` 改动必须 bump `?v=N`（防边缘旧配新）
- 每页有 "Skip to content"（`.skip`）无障碍入口
- fork 生产不走本目录（生产是 PVE CT 151 的 Node+nginx 栈）；本目录服务于上游官网/文档发布链

---

变更记录: 2026-10-27 V1.1 核对式更新（init-architect）:api.html 契约品牌、APK/图标品牌注记；2026-10-20 V1.0 初版
