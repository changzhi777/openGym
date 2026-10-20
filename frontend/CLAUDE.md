[根目录](../CLAUDE.md) > **frontend · React 19 PWA + Capacitor 移动壳**

# frontend/ — openGym 前端

> 全部 UI 与**全部训练域逻辑**所在。React 19 + Vite 构建的 PWA（可安装、离线可用、双端同步），同一构建经 Capacitor 7 包成 Android/iOS 原生壳。

## 模块职责

- 计划（周表/轮换 A-B-C）、训练（引导式会话：预填重量、休息计时、PR 检测）、统计（热图/1RM/结构平衡/肌肉图）、历史、设置、管理面板、Coach 对话界面
- **训练域逻辑全部为纯函数**放 `src/lib/`（见下），MCP 服务器也直接复用这些函数
- Zustand 单店状态 + localStorage 持久化 + 登录后防抖推送服务端（字段级合并）
- 19 语言 i18n + RTL；动作名/说明翻译；动作库 6,554 条（数据由 `catalogue/` 构建生成）

## 技术栈与入口

| 项 | 值 |
|----|----|
| 框架 | React 19 + react-router-dom 7 + Zustand 5（**依赖白名单就这三个生态**） |
| 构建 | Vite 8；`npm run dev`（/api 代理到 :3000）/ `npm run build` / `npm run build:mobile`（VITE_MOBILE=1 + stage-media + cap sync + 检查） |
| 测试 | Vitest 4（+ @vitest/coverage-v8、happy-dom、linkedom） |
| 移动端 | Capacitor 7：`android/`（含 12 个自定义 Java 插件）+ `ios/`（Swift 壳）；文档 [docs/MOBILE.md](../docs/MOBILE.md) |
| 入口 | `index.html` → `src/App.jsx`（路由） |

## 目录地图（src/ 651 文件）

| 目录 | 内容 |
|------|------|
| `src/lib/`（320 文件，约半数为 `*.test.js`） | **训练域核心，纯函数 + 同目录单测**。重点：`progression.js`（进阶引擎：linear/Greyskull LP/double/triple/time-based，策略插件式）、`onerm.js`（1RM 估计）、`finish-workout.js`（完赛归约回状态）、`recovery.js`/`recovery-view.js`（疲劳/恢复模型）、`workout-model.js`/`supersetFlow.js`（会话状态机含超级组）、`exercises.js`/`exercises-data.js`（动作库）、`api.js`（**唯一**后端通信层）、`i18n-core.js`（Node 安全拆分）/`i18n.js`（Vite 位）、`coach-*.js`（Coach 客户端）、`demoSeed.js`（公开 demo 的确定性夹具，MCP 测试同源） |
| `src/store/`（41 文件） | `useStore.js` — **单一 Zustand 店**持整个应用状态 `S`，持久化到 localStorage(`gym_state_v1`)，登录后防抖 `pushState`；移动端经 `lib/mobile.js` 镜像到文件（WebView 存储可被逐出）。`useUI.js` — 易失 UI 态（模态/活动 sheet）。30+ 个 `*.test.jsx` 覆盖同步/导入/恢复/多标签页 |
| `src/views/`（109 文件） | 一屏一文件：Home / Workout / Plan / Library / Stats / History / Settings / Admin / AdminCoach / CoachChat / CoachIntake / CoachSetup / Login / MobileOnboarding / RoutineEdit / Muscles / Measurements / ProgressPhotos / StructuralBalance / CheckIn / FocusView；`settings-pages.js`。行为测试按维度分文件（如 `Settings.passkeys.test.jsx`） |
| `src/components/`（65+ 文件） | 共享 UI：图表（LineChart/Heatmap）、BodyMap/MuscleExplorer、RestTimer、Media、Passkeys、PasswordAuth、ServerSync、SwipeCards/SwipeRow、QrCanvas、CameraScan 等 |
| `src/locales/` | i18n 字符串目录（19 语言），`lib/i18n.js`/`i18n-core.js` 消费 |
| `src/exercise-names/`（18 语言）· `src/instr/`（多语言动作说明）· `src/exercise-desc/en.js` | 动作名/说明翻译数据（源数据来自 `catalogue/i18n/`，构建期生成） |
| `src/admin.css` / `coach.css` / `index.css` | 样式（无 CSS 框架） |
| `scripts/` | bundle-size、check-locales、check-source-strings、check-mobile-bundle、fatigue-monotonic-probe、sw-stamp 等 CI 检查脚本 |

## 状态与同步模型

1. 客户端唯一真源是 Zustand `S`：localStorage 持久化 → 登录后防抖 `pushState`（`lib/api.js`）→ 服务端存 `state-<uid>.json`
2. 两设备同时编辑：**字段级合并**而非整态覆盖（见根 README "How sync works" 与 `docs/diagrams/sync.mmd`）
3. 访客模式：一切留在浏览器，从不触网（服务端只透出开关）
4. 移动端：`MOBILE` 标志位门控原生能力（文件持久化、本地通知、wake lock、健康连接）

## 测试

```bash
cd frontend
npm test                      # vitest run（全套）
npm run test:watch
npx vitest run src/lib/progression.test.js    # 单文件
npx vitest run -t "test name"                 # 单用例
npm run test:fatigue-probe    # 疲劳模型单调性属性探针（CI 专用，本地慢）
```

- **铁律**：任何"决定下次举什么/回读已记录会话"的改动必须配 `src/lib` 单测（progression 引擎历史上两个 bug 只有测试抓到）
- CI 附加闸门：`npm run build`（坏 import 早期暴露）、`check-locales.mjs`（语言包互相一致）、`check-source-strings.mjs`（扫 src 的漏翻译，report-only）

## 关键依赖（勿随手加）

运行时：`react` / `react-dom` / `react-router-dom` / `zustand` + Capacitor 系（app/core/filesystem/local-notifications/share/secure-storage/mlkit barcode）+ `jsqr`/`lean-qr`（二维码）。**CONTRIBUTING.md 明言：React + Router + Zustand 之外不加**。

## FAQ / 坑

- **找后端调用**：只在 `lib/api.js`；别的文件不应直接 fetch
- **i18n 改动**：同时跑 check-locales；`i18n-core.js` 必须保持 Node 可加载（MCP 直接 import 它）
- **动作数据**：`lib/exercises-data.js` 是 `catalogue/` 构建产物，别手编；hevy-id-map.js 由 `HEVY_API_KEY= node scripts/build-hevy-id-map.mjs` 再生
- **Capacitor 壳**：`android/`/`ios/` 是生成+手改混合体；APK 签名构建在 GitLab CI（`docs/MOBILE.md`）

## 相关文件清单（节选）

`package.json` · `vite.config.js` · `capacitor.config.json` · `index.html` · `src/App.jsx` · `src/store/useStore.js` · `src/lib/`（320）· `src/views/`（109）· `src/components/` · `src/locales/` · `android/`（110 文件，含 12 个 Java 插件）· `ios/App/`（Swift 壳）· `scripts/`（9）

---

变更记录: 2026-10-20 V1.0 初版（init-architect）
