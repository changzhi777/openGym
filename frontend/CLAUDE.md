[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **frontend · React 19 PWA + Capacitor 移动壳**

# frontend/ — 沐禾健身前端

> 全部 UI 与**全部训练域逻辑**所在。React 19 + Vite 构建的 PWA（可安装、离线可用、双端同步），同一构建经 Capacitor 7 包成 Android/iOS 原生壳。fork 现状：品牌已全面切换为**沐禾健身 / Muhe Fit**（appId `com.muhe.fit`），并新增管理后台 `views/admin/`。

## 模块职责

- 计划（周表/轮换 A-B-C）、训练（引导式会话：预填重量、休息计时、PR 检测）、统计（热图/1RM/结构平衡/肌肉图）、历史、设置、管理面板、Coach 对话界面
- 🆕 **管理后台 `views/admin/`**：AdminShell 布局壳（桌面侧栏/移动顶栏、运营/系统两组导航）+ 概览 + 用户 + 用户详情 + 审计（游标分页）+ 占位页；三角色模型（super_admin / ops / support 只读）+ step-up 框架（`lib/admin/stepUp.js`）；后端未供 adminRoles 时回落 super_admin；手机端只读
- **训练域逻辑全部为纯函数**放 `src/lib/`，MCP 服务器也直接复用
- Zustand 单店状态 + localStorage 持久化 + 登录后防抖推送服务端（字段级合并）
- 19 语言 i18n + RTL；🆕 动作说明全量中文（`instr/zh.js`、`exercise-desc/zh.js` 各 5,632 条，由 catalogue 构建生成）；动作库 6,554 条

## 品牌改造（fork，FORK.md §5）

- **单点替换**：`src/lib/i18n-core.js` 的 `BRAND = { zh: '沐禾健身', 'zh-TW': '沐禾健身' }`，其余语言回落 `Muhe Fit`；t()/tn() 渲染层替换（含 `an openGym`→`a Muhe Fit` 冠词、`Documents/openGym`→`Documents/MuheFit` 目录保护），**locale 键零改动**
- `capacitor.config.json`：appId `com.muhe.fit`、appName 沐禾健身；Android 包名同源（debug 后缀 `.test`）
- 登录页/移动端首启页标题走 `t('openGym')` 品牌键；manifest / index.html / 分享图水印 / 备份目录名同步
- 图标全套已换（PWA 180/192/512 + Android 6 密度 mipmap，2026-10-22）

## 技术栈与入口

| 项 | 值 |
|----|----|
| 框架 | React 19 + react-router-dom 7 + Zustand 5（**依赖白名单就这三个生态**） |
| 构建 | Vite 8；`npm run dev`（/api 代理到 :3000）/ `npm run build` / `npm run build:mobile`（VITE_MOBILE=1 + stage-media + cap sync + 检查；**APK 必须 build:mobile，普通 build 动画全黑**；JDK 21） |
| 测试 | Vitest 4（+ coverage-v8、happy-dom、linkedom）；🆕 **fork-only `vitest.config.mjs`**：mergeConfig 挂 `src/test/setup-local-storage.mjs`，修 Node≥24 实验性全局 localStorage 遮蔽 happy-dom 注入的问题（不进 vite.config.js 以避上游 rebase 冲突） |
| 移动端 | Capacitor 7：`android/`（含 12 个自定义 Java 插件）+ `ios/`（Swift 壳）；文档 [docs/MOBILE.md](../docs/MOBILE.md) |
| 包名 | `muhe-frontend` |
| 入口 | `index.html` → `src/App.jsx`（路由） |

## 目录地图（src/ ~663 文件）

| 目录 | 内容 |
|------|------|
| `src/lib/`（320+ 文件，约半数为 `*.test.js`） | **训练域核心，纯函数 + 同目录单测**。重点：`progression.js`、`onerm.js`、`finish-workout.js`、`recovery.js`/`recovery-view.js`、`workout-model.js`/`supersetFlow.js`、`exercises.js`/`exercises-data.js`、`api.js`（**唯一**后端通信层）、`i18n-core.js`（Node 安全拆分，**品牌单点**）/`i18n.js`、`coach-*.js`、`demoSeed.js`；🆕 `lib/admin/i18n.js`（adminT 管理后台文案）与 `lib/admin/stepUp.js`（step-up 框架） |
| `src/store/`（41 文件） | `useStore.js` — **单一 Zustand 店**持整个应用状态 `S`，持久化 localStorage(`gym_state_v1`)，登录后防抖 `pushState`；移动端经 `lib/mobile.js` 镜像到文件。`useUI.js` — 易失 UI 态。30+ 个 `*.test.jsx` |
| `src/views/`（~114 文件） | 一屏一文件：Home / Workout / Plan / Library / Stats / History / Settings / Admin / AdminCoach / CoachChat / CoachIntake / CoachSetup / Login / MobileOnboarding / RoutineEdit / Muscles / Measurements / ProgressPhotos / StructuralBalance / CheckIn / FocusView；🆕 **`views/admin/` 子目录 6 文件**：`AdminShell.jsx`（布局壳+三角色+403 面板）、`Overview.jsx`、`Users.jsx`、`UserDetail.jsx`、`Audit.jsx`（游标分页）、`Placeholder.jsx`（对应 Node 501 stub 的占位渲染，不误调）；`settings-pages.js`；样式 `admin-console.css` |
| `src/components/`（65+ 文件） | 共享 UI：图表、BodyMap/MuscleExplorer、RestTimer、Media、Passkeys、PasswordAuth、ServerSync、SwipeCards、QrCanvas、CameraScan 等 |
| `src/locales/` | i18n 字符串目录（19 语言），品牌词不经 locale 键（见品牌改造节） |
| `src/exercise-names/`（18 语言）· 🆕 `src/instr/zh.js` + `src/exercise-desc/zh.js` | 动作名/说明翻译数据（构建期由 `catalogue/i18n/zh.json` 生成，**勿手编**） |
| `src/test/setup-local-storage.mjs` | 🆕 Vitest setup：Node≥24 下垫平 localStorage（配合 fork-only vitest.config.mjs） |
| `src/admin.css` / `coach.css` / `index.css` | 样式（无 CSS 框架） |
| `scripts/` | bundle-size、check-locales、check-source-strings、check-mobile-bundle、fatigue-monotonic-probe、sw-stamp 等 CI 检查脚本 |

## 状态与同步模型

1. 客户端唯一真源是 Zustand `S`：localStorage 持久化 → 登录后防抖 `pushState` → 服务端存 `state-<uid>.json`（Go 栈对应 PG `states` 表，契约不变）
2. 两设备同时编辑：**字段级合并**而非整态覆盖（见根 README 与 `docs/diagrams/sync.mmd`）
3. 访客模式：一切留在浏览器，从不触网
4. 移动端：`MOBILE` 标志位门控原生能力；🆕 管理后台手机端只读（AdminShell 内提示条）

## 测试

```bash
cd frontend
npm test                      # vitest run（全套；fork 口径 Node 22 + TZ=UTC，见根 CLAUDE.md）
npx vitest run src/lib/progression.test.js    # 单文件
npm run test:fatigue-probe    # 疲劳模型单调性属性探针（CI 专用）
```

- **铁律**：任何"决定下次举什么/回读已记录会话"的改动必须配 `src/lib` 单测
- CI 附加闸门：`npm run build`、`check-locales.mjs`、`check-source-strings.mjs`
- 已知偶发（非改动引起，重跑即绿）：全量并行跑时 `useStore.tabs-owner` / `useStore.adopt-hold` 偶红（多标签 storage 事件并发时序）

## 关键依赖（勿随手加）

运行时：`react` / `react-dom` / `react-router-dom` / `zustand` + Capacitor 系 + `jsqr`/`lean-qr`。**CONTRIBUTING.md 明言：React + Router + Zustand 之外不加**。

## FAQ / 坑

- **找后端调用**：只在 `lib/api.js`；管理后台新端点（overview/quota/roles/step-up…）当前是契约 + Go 实现，Node 侧 501——前端只做占位渲染不调用，误调即 501
- **品牌词**：改文案不要绕过 `i18n-core.js` 的 BRAND 单点；文档目录写死 `Documents/MuheFit`
- **i18n 改动**：同时跑 check-locales；`i18n-core.js` 必须保持 Node 可加载（MCP 直接 import）
- **动作数据**：`lib/exercises-data.js`、`instr/zh.js`、`exercise-desc/zh.js` 是 catalogue 构建产物，别手编
- **APK**：必须 `build:mobile` + JDK 21；build:mobile 后 dist 是移动包，部署 web 前重跑 `npm run build`
- **Capacitor 壳**：`android/`/`ios/` 是生成+手改混合体

## 相关文件清单（节选）

`package.json`（muhe-frontend）· `vite.config.js` · `vitest.config.mjs`（fork-only）· `capacitor.config.json`（com.muhe.fit）· `index.html` · `src/App.jsx` · `src/store/useStore.js` · `src/lib/` · `src/views/`（含 admin/ 6 文件）· `src/components/` · `src/locales/` · `android/` · `ios/App/` · `scripts/`

---

变更记录: 2026-10-27 V1.1 增量（init-architect）:品牌改造（BRAND 单点/com.muhe.fit/图标）、views/admin 管理后台 6 文件 + lib/admin、vitest.config.mjs fork-only、中文说明生成物；2026-10-20 V1.0 初版
