[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **frontend · React 19 PWA + Capacitor 移动壳**

# frontend/ — 沐禾健身前端

> 全部 UI 与**全部训练域逻辑**所在。React 19 + Vite 构建的 PWA（可安装、离线可用、双端同步），同一构建经 Capacitor 7 包成 Android/iOS 原生壳。fork 现状：品牌已全面切换为**沐禾健身 / Muhe Fit**（appId `com.muhe.fit`），并新增管理后台 `views/admin/`。🆕 AI-Native 对话入口**已于 2026-11-11 实施（批次 4/5/6 前端部分）**：`views/home-ai/CoachChatHome`（`/coach-ai` 旁路路由）+ `src/lib/api-chat.js`/`coach-chat.js` + `admin/AgentConfig.jsx` 治理页 + Overview AI 用量卡——见下方"AI-Native 对话入口现状"节。

## 模块职责

- 计划（周表/轮换 A-B-C）、训练（引导式会话：预填重量、休息计时、PR 检测）、统计（热图/1RM/结构平衡/肌肉图）、历史、设置、管理面板、Coach 对话界面（`/coach` 批任务 + `/coach-ai` 对话式）
- **管理后台 `views/admin/`**（9 文件，AI-Native 后）：AdminShell 布局壳（桌面侧栏/移动顶栏、运营/系统两组导航、session 真实角色）+ 概览 v2（Go 聚合 + SSE 实时卡 + 🆕 AI 教练用量卡）+ 用户列表（服务端分页）+ 用户详情四卡 + 审计（搜索/展开/清空）+ **Roles 角色页 + Invites 邀请码页 + 🆕 AgentConfig AI 治理页** + 占位页；三角色模型 + step-up（`lib/admin/stepUp.js`）；手机端只读；**旧版 views/Admin.jsx 已下线删除（2026-10-28）**
- **训练域逻辑全部为纯函数**放 `src/lib/`，MCP 服务器与 Go Agent 内核（经 golden 锚定）共同复用
- Zustand 单店状态 + localStorage 持久化 + 登录后防抖推送服务端（字段级合并）
- 19 语言 i18n + RTL；动作说明全量中文（`instr/zh.js`、`exercise-desc/zh.js` 各 5,632 条，由 catalogue 构建生成）；动作库 6,554 条

## 🆕 AI-Native 对话入口现状（批次 4/5/6 已实施，2026-11-11）

> 方案全文：《沐禾健身-AINative重构方案-v2.md》（`.zcf/`）。批任务 `CoachChat.jsx`（`/coach`，上游原样保留）之外，对话式入口走**旁路目录 `views/home-ai/`**（fork-only 文件，不与上游冲突；文案直用中文，locale 键化属批次 6 后段待办）。**后端实现在 Go 栈**（对话面七端点，Node 侧 501 stub）。

### 已落地清单（2026-11-11 实测）

| 资产 | 内容 | 测试 |
|------|------|------|
| `src/lib/api-chat.js` | 对话面七端点封装（复用 `api()` 既有语义）+ **parseSSEFrame 纯函数**（SSE 帧解析离线可测：半帧缓冲/多帧连发/坏 JSON 不吞帧） | `api-chat.test.js` 6 案 |
| `src/lib/coach-chat.js` | 六事件状态归并纯函数（applyEvent / applyUserMessage）+ **runChatTurn 编排**（依赖注入可离线单测：成功 / cap 原文透出 / 错误统一文案）+ decideLocally + proposalToCard 卡片供形 + memorySummary | `coach-chat.test.js` 20 案（归并 15 + 编排 5） |
| `src/views/home-ai/CoachChatHome.jsx` | 对话主页（旁路目录）：消息流 / 输入框 / 提议确认卡片（应用/拒绝走 confirm 端点）/ usage 头 / 工具标注 | `CoachChatHome.test.jsx` 渲染冒烟 4 案（锁「状态→渲染」接线；DOM 事件交互断言不做——linkedom×React 受控输入不可靠，send/decide 是 lib 薄委派） |
| `src/App.jsx` | `/coach-ai` 路由（1 import + 1 Route，L232–233）；上游 `/coach` 原样保留 | 路由随套件 |
| `src/views/admin/AgentConfig.jsx` | **AI 治理页**（批次 6）：MAX_TURNS（1–8）+ 审核词表（≤200 词）编辑，热生效文案（PUT 后下一回合生效无重启） | 治理页随套件 |
| `src/views/admin/AdminShell.jsx` | 系统组导航新增「AI 治理」项（`/admin/coach/agent`，support 只读 / ops+stepUp 写） | AdminShell 套件 |
| `src/views/admin/Overview.jsx` | **「AI 教练用量」卡**（批次 5）：三池 KPI（chat/review/embedding）+ 今日活跃 + 14 天消息趋势行；Node 501（无数据）时卡片不渲染，不显示假 0 | Overview 套件 |

合计新增 **30 案测试全绿**（台账批次 4 口径）；node26+UTC 全量口径下前端 8 假红基线（Passkeys/PasswordAuth/useStore.tabs-owner 已知偶发/RoutineEdit.drag——与批次 4–6 改动零交集）。

### 仍开放的后续项（台账批次 6 范围收敛记录）

- **P3 Settings 记忆页未建**：`GET /api/coach/memory` + PUT/DELETE `/{id}` 契约已冻结、Go 已实现，但前端"Coach 记住了什么"Settings 页尚未做（记忆查看/删除权 UI 缺口）
- **动作卡渲染**：search_knowledge 结果 ref_id → 动作卡，推批次 6 后段
- **CoachChatHome 文案 locale 键化**：现为中文直写（fork-only 文件托管中文主线），批次 6 后段统一
- **P5 文件导入**（CSV/XLSX）：未做，最末位
- **运营激励触达**（AI 周报推送链路）：依赖推送基建编排，本轮只交付对话内周报能力，推送侧后置

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
| 测试 | Vitest 4（+ coverage-v8、happy-dom、linkedom）；**fork-only `vitest.config.mjs`**：mergeConfig 挂 `src/test/setup-local-storage.mjs`，修 Node≥24 实验性全局 localStorage 遮蔽 happy-dom 注入的问题（不进 vite.config.js 以避上游 rebase 冲突） |
| 移动端 | Capacitor 7：`android/`（含 12 个自定义 Java 插件）+ `ios/`（Swift 壳）；文档 [docs/MOBILE.md](../docs/MOBILE.md) |
| 包名 | `muhe-frontend` |
| 入口 | `index.html` → `src/App.jsx`（路由） |

## 目录地图（src/ ~669 文件）

| 目录 | 内容 |
|------|------|
| `src/lib/`（326+ 文件，约半数为 `*.test.js`） | **训练域核心，纯函数 + 同目录单测**。重点：`progression.js`、`onerm.js`（**算法单源**：mcp 直接 import、Go 内核经 golden 4480 案锚定）、`finish-workout.js`、`recovery.js`/`recovery-view.js`、`workout-model.js`/`supersetFlow.js`、`exercises.js`/`exercises-data.js`、`api.js`（既有唯一后端通信层）、`i18n-core.js`（Node 安全拆分，**品牌单点**）/`i18n.js`、`coach-*.js`（批任务侧）、`demoSeed.js`；🆕 **`api-chat.js` + `coach-chat.js`（AI-Native 对话面：SSE 解析/状态归并/编排纯函数）**；`lib/admin/i18n.js`（adminT 管理后台文案）与 `lib/admin/stepUp.js`（step-up 框架） |
| `src/store/`（41 文件） | `useStore.js` — **单一 Zustand 店**持整个应用状态 `S`，持久化 localStorage(`gym_state_v1`)，登录后防抖 `pushState`；移动端经 `lib/mobile.js` 镜像到文件。`useUI.js` — 易失 UI 态。30+ 个 `*.test.jsx` |
| `src/views/`（~117 文件） | 一屏一文件：Home / Workout / Plan / Library / Stats / History / Settings / Admin / AdminCoach / CoachChat（批任务）/ CoachIntake / CoachSetup / Login / MobileOnboarding / RoutineEdit / Muscles / Measurements / ProgressPhotos / StructuralBalance / CheckIn / FocusView；**`views/home-ai/`（🆕 2 文件：CoachChatHome 对话入口 + 测试，`/coach-ai` 挂载）**；**`views/admin/` 子目录 9 文件**：`AdminShell.jsx`（布局壳+三角色+AI 治理导航）、`Overview.jsx`（含 AI 用量卡）、`Users.jsx`、`UserDetail.jsx`、`Audit.jsx`（游标分页）、`Roles.jsx`、`Invites.jsx`、`AgentConfig.jsx`（🆕 AI 治理页）、`Placeholder.jsx`（对应 Node 501 stub 的占位渲染，不误调）；`settings-pages.js`；样式 `admin-console.css` |
| `src/components/`（65+ 文件） | 共享 UI：图表、BodyMap/MuscleExplorer、RestTimer、Media、Passkeys、PasswordAuth、ServerSync、SwipeCards、QrCanvas、CameraScan 等 |
| `src/locales/` | i18n 字符串目录（19 语言），品牌词不经 locale 键（见品牌改造节） |
| `src/exercise-names/`（18 语言）· `src/instr/zh.js` + `src/exercise-desc/zh.js` | 动作名/说明翻译数据（构建期由 `catalogue/i18n/zh.json` 生成，**勿手编**） |
| `src/test/setup-local-storage.mjs` | Vitest setup：Node≥24 下垫平 localStorage（配合 fork-only vitest.config.mjs） |
| `src/admin.css` / `coach.css` / `index.css` | 样式（无 CSS 框架） |
| `scripts/` | bundle-size、check-locales、check-source-strings、check-mobile-bundle、fatigue-monotonic-probe、sw-stamp 等 CI 检查脚本 |

## 状态与同步模型

1. 客户端唯一真源是 Zustand `S`：localStorage 持久化 → 登录后防抖 `pushState` → 服务端存 `state-<uid>.json`（Go 栈对应 PG `states` 表，契约不变）
2. 两设备同时编辑：**字段级合并**而非整态覆盖（见根 README 与 `docs/diagrams/sync.mmd`）
3. 访客模式：一切留在浏览器，从不触网
4. 移动端：`MOBILE` 标志位门控原生能力；管理后台手机端只读（AdminShell 内提示条）

## 测试

```bash
cd frontend
npm test                      # vitest run（全套；fork 口径 Node 22 + TZ=UTC，见根 CLAUDE.md）
npx vitest run src/lib/coach-chat.test.js      # 单文件
npm run test:fatigue-probe    # 疲劳模型单调性属性探针（CI 专用）
```

- **铁律**：任何"决定下次举什么/回读已记录会话"的改动必须配 `src/lib` 单测（AI-Native 的 SSE 解析/状态归并/编排全部照此执行）
- CI 附加闸门：`npm run build`、`check-locales.mjs`、`check-source-strings.mjs`
- 已知偶发（非改动引起，重跑即绿）：全量并行跑时 `useStore.tabs-owner` / `useStore.adopt-hold` 偶红（多标签 storage 事件并发时序）；node26+UTC 口径另有 Passkeys/PasswordAuth/RoutineEdit.drag 假红基线 8（台账批次 4/6 行）
- 🆕 AI-Native 30 案已进套件（api-chat 6 + coach-chat 20 + CoachChatHome 4，见现状节清单）

## 关键依赖（勿随手加）

运行时：`react` / `react-dom` / `react-router-dom` / `zustand` + Capacitor 系 + `jsqr`/`lean-qr`。**CONTRIBUTING.md 明言：React + Router + Zustand 之外不加**。AI-Native 对话面（SSE 解析、状态归并）全程零新依赖——parseSSEFrame 是手写纯函数；P5 若引 CSV/XLSX 解析（如 SheetJS）需先过依赖极简纪律评审。

## FAQ / 坑

- **找后端调用**：同步域只在 `lib/api.js`；AI-Native 对话域在 `lib/api-chat.js`（封装同语义）。管理后台部分端点（overview/quota/roles/step-up…）与 AI-Native 对话面在 Node 侧是 501 stub、实现在 Go 栈——前端只做占位渲染不调用，误调即 501
- **对话式 UI 改动**：先改 `src/lib` 纯函数（状态归并/编排）配单测，`home-ai/` 组件只做"状态→渲染"接线；事件交互断言不做（linkedom×React 受控输入不可靠）
- **品牌词**：改文案不要绕过 `i18n-core.js` 的 BRAND 单点；文档目录写死 `Documents/MuheFit`；`home-ai/` 中文直写是批次 6 后段待办，新 fork-only 页面同策略
- **i18n 改动**：同时跑 check-locales；`i18n-core.js` 必须保持 Node 可加载（MCP 直接 import）
- **动作数据**：`lib/exercises-data.js`、`instr/zh.js`、`exercise-desc/zh.js` 是 catalogue 构建产物，别手编
- **APK**：必须 `build:mobile` + JDK 21；build:mobile 后 dist 是移动包，部署 web 前重跑 `npm run build`
- **Capacitor 壳**：`android/`/`ios/` 是生成+手改混合体

## 相关文件清单（节选）

`package.json`（muhe-frontend）· `vite.config.js` · `vitest.config.mjs`（fork-only）· `capacitor.config.json`（com.muhe.fit）· `index.html` · `src/App.jsx`（含 `/coach-ai`）· `src/store/useStore.js` · `src/lib/`（含 api-chat/coach-chat）· `src/views/`（含 home-ai/ 2 文件、admin/ 9 文件、CoachChat/CoachIntake/CoachSetup/AdminCoach）· `src/components/` · `src/locales/` · `android/` · `ios/App/` · `scripts/`

---

变更记录: 2026-11-11 V1.3 增量（init-architect，实施后刷新轮）:"AI-Native 对话入口规划"节改写为"现状"——已落地清单七项（api-chat.js 6 案 / coach-chat.js 20 案 / home-ai/CoachChatHome.jsx 4 案 + /coach-ai 路由 / admin/AgentConfig.jsx 治理页 + AdminShell「AI 治理」导航 / Overview AI 用量卡），30 案测试全绿；仍开放项如实入账（P3 Settings 记忆页未建 / 动作卡渲染与文案 locale 键化推批次 6 后段 / P5 导入 / 推送侧）；目录地图更新（src ~669 文件、home-ai/ 新目录、admin/ 6→9 文件）；FAQ 加对话式 UI 纪律。2026-11-10 V1.2 增量:新增"AI-Native 对话入口规划"节（P2/P3 规划态，现状盘点 CoachChat 4+CoachIntake 1 测试文件）；2026-10-27 V1.1 增量:品牌改造（BRAND 单点/com.muhe.fit/图标）、views/admin 管理后台 + lib/admin、vitest.config.mjs fork-only、中文说明生成物；2026-10-20 V1.0 初版
