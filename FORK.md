# Fork 说明 — 本仓库的定位与合规边界

> 本文件是本 fork（changzhi777/openGym）的二次发行声明。基于上游
> [DuarteSantos8/openGym](https://github.com/DuarteSantos8/openGym)（AGPL-3.0），
> fork 时间：2026-10-21，基点：上游 `main`（318b57bf，v1.4.1 之后）。

## 1. 与上游的关系

- **跟进策略**：保留完整 git 历史；`upstream` 指向上游原仓库，定期 rebase/merge
  吸收上游 release（上游约两周一个版本）。
- **改动原则**：修改尽量收敛在独立分支与独立文件，减少与上游合并的冲突面。
- **本地 remote 布局**（2026-10-28 迁移后）：`origin` = **cnb.cool/qingmu2026/muhe-fit**（CNB 私有主仓，沐禾健康组织）；`github` = changzhi777/openGym（GitHub 镜像）；`upstream` = DuarteSantos8/openGym。

## 2. 许可证义务（AGPL-3.0）

- 本 fork 的代码沿用 **GNU AGPL v3.0**（见 [LICENSE](LICENSE)）。对本 fork 的任何
  修改同样以 AGPL v3.0 提供源码。
- 若以网络服务形式运行修改后的版本，必须向服务使用者提供该版本完整源码
  （AGPL §13 网络条款）。
- 上游 NOTICE.md 中的 **App Store 例外**（AGPL §7 additional permission）仅由
  上游版权持有人 Duarte Santos 授予；本 fork 发布移动端应用包时的合规性需自行评估。
- 上游项目本身用 Claude Code 开发并经人工审核（见上游 README「How openGym is
  built」节）；本 fork 的工作流与之类似，**测试门是底线**——`frontend/`、`api/`、
  `mcp/` 三套测试必须绿才算改动成立。

## 3. 运动媒体版权（⚠️ 不在 AGPL 覆盖内）

`catalogue/media/` 的动作图片/动画 **© Aliaksandr Makatserchyk, Gym visual
(gymvisual.com)**，仅授权在 openGym 应用内使用，随本 fork 原样保留的边界
（详见 [NOTICE.md](NOTICE.md)）：

- 本 fork 属**非商业用途**：可按上游原条款原样保留 180 px 媒体与本声明。
- **一旦转向商业用途**，必须先从 [gymvisual.com](https://gymvisual.com/) 取得
  自有授权，否则需整体替换素材。
- 禁止将媒体用于训练/提示/投喂任何 AI 系统，或作为生成图像、动画、视频的底稿
  （Gym visual 条款明文）。
- 禁止把媒体从仓库或运行实例中提取、热链或集成到独立应用/网站。

## 4. 其他随仓库分发的第三方内容

- 人体肌肉图（`frontend/src/lib/body-paths.js`）：源自 MuscleMap（MIT）。
- 动作目录文本：openGym 社区维护；其中 1,324 条（`"textSource": "exercisedb"`）
  源自 ExerciseDB v1（MIT）。翻译与增补部分属 openGym（AGPL）。
- 完整清单见 [NOTICE.md](NOTICE.md)。

## 5. 本 fork 的改动记录

| 日期 | 分支/提交 | 内容 |
|---|---|---|
| 2026-10-21 | `zh-terminology` (6a8ad08d) | zh.js 14 处术语规范化（训练量/复盘/减载/预估 1RM），依据 `frontend/docs/i18n-terminology-audit.md` |
| 2026-10-22 | `main` (c8832ca3) | 图标全套（PWA 180/192/512 + Android 6 密度） |
| 2026-10-22 | `main` (8e263b24) | **动作中文说明全量翻译**：5,632 条 description + instructions（DeepSeek 批量，术语表对齐审计口径，74 万 tokens ≈2.2 元）；新建 `exercise-desc/zh.js`、`instr/zh.js` 1,324→5,632 条、AI 教练中文包同步 |
| 2026-10-22 | `main` | **品牌替换 openGym → 沐禾健身 / Muhe Fit**：`i18n-core.js` t()/tn() 渲染层单点替换（含 `an openGym`→`a Muhe Fit` 冠词与 `Documents/openGym`→`Documents/MuheFit` 目录保护，locale 键零改动）；manifest/index.html/capacitor(appId=com.muhe.fit)/api RP_NAME/备份目录与文件名/分享图水印/.fit 软件名同步；图标暂留上游（待 Logo 资产） |
| 2026-10-28 | `claude/nostalgic-austin-94093b` → `main` (4ee87a11..008e1ddc3) | **Admin 后台批 1-4 全量落地 + CNB 建仓迁移**：① 主仓迁 CNB `qingmu2026/muhe-fit`（私有，origin 切换，github 降镜像，68 refs 全历史）；② 修 `internal/data` 包缺失（根因 .gitignore 裸 `data/` 吞包，锚根 `/data/`）；③ 契约批 1a：session/export/settings/alive/live-SSE 五端点 + audit q + quota used + overview 扩形（86 端点）；④ Go 批 1b：session 端点 + 邀请码全链路（0011 表/注册闸门/三管理端点）+ 实例设置 + 审计 IP 读出 + 迁移器修丢码；⑤ 批 1c 角色页 `/admin/roles` + 壳接真实角色；⑥ 批 2：Users 端点重写（搜索/筛选/分页/统计/邮箱脱敏）+ activity/export + 配额 used + 审计深化（q/total/名字解析/清空/90 天清理）+ 前端四卡，修 P5 audit 路由 bug；⑦ 批 3：收编主 checkout P5 瘦尾 WIP（coach 桥/quota/媒体 GC/0007-0008）+ coach 放开 ops + presence（Redis）/live SSE + 概览 v2；⑧ 批 4：邀请码页 + 营收占位 + legacy 下线（Admin.jsx 删除） |
| 2026-10-29 | `main` (0c7841f2a..0c7841f2a+) | **生产部署上线：qingmulife.com @ 43.129.201.118**——Docker Compose 三件套（pg16+redis7+muhe-go:3100，/opt/muhe-fit，data 卷+PG 卷持久化）；nginx 双 server（裸域=前端静态+/api 同源反代；api 子域=API 直连，改写已死的旧 3001 反代，/admin-v2/ 保留）；Certbot 双证书（api 原有+裸域新签）；收编主 checkout 残余（coach-sidecar 过渡进程/jobs.go 数据桥/stepUp 验签修复）；固化 `scripts/deploy-118.sh`（幂等）；DNSPod 裸域 @ A 记录补齐（api 子域原有）。⚠️ 两次"吞包"教训：.gitignore 裸 `data/` 与 bsdtar `--exclude './data'`（无锚定模式匹配任意段） |
| 2026-10-29 | `main` (87ba7cb54+) | **认证重构上线（passkey/密码/device-link → 手机号+短信+微信扫码）**：19 旧端点标 deprecated，新 7 端点（captcha/sms send·verify/wechat qrcode·poll·confirm/bind-phone，契约先行 openapi.yaml 93 端点）；Go 侧新增 `internal/{captcha,sms,wechat,store/identities.go}` + `0012_auth_identities` 迁移（users+auth_identities 身份表）；腾讯云 SMS TC3 签名标准库手写零依赖；前端 Login 全重写（图形码+6 格短信码+微信 tab 全中文）、Settings 账号区三态重写；admin step-up 改短信码。⚠️ 坑：captcha SVG `Sprintf` 参数/占位符错位（`%!s(int=26)`）改命名切片修复；`api.js smsSend` 丢 captcha 参数致"图形码换图但短信不发"；SmsVerify 响应名用手机号派生名覆盖库中真名（cz 显示成"健身者8884"）已修。**生产端到端验收通过**：es2020 target 修手机旧内核黑屏后，真短信→验证码→建号→登录全链路 ego 走查通过（17570348884 → cz 首号） |
| 2026-10-30 | `main` (4d37046fd..d47509cfd) | **认证审计 + admin 用户页三连修**：`store.AuditAuth` best-effort 审计（auth.sms.send/verify、bind.phone、wx.confirm，手机号脱敏，与 admin fail-closed 相反语义）；Users 端点加 hasPhone/hasWx 投影 + 前端身份徽章（契约同步）。⚠️ 三个上线即坏的雷（页面上线以来从未被打开而潜伏）：① `last_workout` 对 text 列 `to_char` 签名不存在致用户列表整页 500（改直接 `max(wk.date)`）；② `created` 模板缺 DD（`2026-10-T…`→Invalid Date）；③ 审计 `inet` 列 pgx 二进制协议无法 Scan 进 `*string` 致审计列表永远空白（`ip::text` 修复）。教训：新页面上线要真的打开走一遍；Scan 静默 `continue` 改打日志后 5 分钟定位。ego 验收坑：SPA hash 路由 goto 不重载文档，部署后验证必须 `reload()` 拿新 bundle |
| 2026-10-30 | `main` (8971df424..ec2fb4d29) | **step-up 短信化全链路收尾**（批 H 后端先行、前端/契约欠账补齐）：契约 step-up 改 `{phone, code}` 短信核验；Go 403 code `passkey-required`→`step-up-required`；新建 `StepUpSheet`（手机号+图形码+发码+短信码），AdminShell 注入 verifier（此前从未注入=敏感操作全卡死）；stepUp.js 摘除 passkey 回落、保留旧 code 兼容。生产端到端验收：邀请制开关→403→短信→开窗→自动重放，审计三连落表。admin 全子页 ego 走查再排三雷：Invites 缺 `useUI`/`confirmSheet` import 整页崩；Go User 端点 created 违约返回 ms（正本清源改 ISO）；fullEmail 残留 passkey 死按钮摘除。**工程化**：新建 `admin-pages.test.jsx` 全子页冒烟（真实路由树 8 页，缺 import/运行时崩 CI 即炸），冒烟首战即抓出 AdminCoach `providers`/`runtime` 无守卫真雷（接口异常整页崩） |
| 2026-11-06 | `main` (分支) | **微信扫码生产点亮 + 教练工作台 v1**：①微信链路四修——MintTicket ticket 超长（wx+UnixNano+hex=33 字符，scene 上限 32 → 40169，改纯随机 28）；getUnlimitedQRCode `check_path` 传字符串 `"false"` 被当 true（改布尔）；access_token 被外部刷新作废后无强刷重试（40001/42001 强刷重试一次）；`WX_MINI_ENV=trial` 体验版出码（联调期，正式发布后改 release）。凭据 `wx8c37d7ac5b7d0a83` 上 118，域名白名单加 app 域，生产出码 113KB ✓。②教练工作台 v1（coach-console）：`coach` 角色枚举 + `coach_assignments` 绑定表（0013）；`/api/coach-console/*` 六端点（绑定学员/白名单详情/下发计划 rev 乐观并发/导出）；前端 `/coach-console` 独立壳（手机可操作）+ 模板化计划编辑器（四内置模板/学员当前/空白三起点）+ Web Push 通知。⚠️ 坑：BodyGuard 中间件预读 body → 新 handler 必须 `c.Get("rawBody")`（ShouldBindJSON 再读是空流 → 400）；HashRouter 应用 URL 必须带 `#`（pathname 路由不匹配）；openapi-routes 测试 TAGS 正则 `\w+` 不匹配连字符键名。**生产验收**：出码/绑定 404 提示/角色闸门/工作台渲染全通；下发链路待首个真实学员（测试 6+冒烟全覆盖） |
| 2026-11-06 | `main` (分支) | **安全对齐：上游 v1.4.2 安全修复语义移植**（git merge 不可行——上游是 Node 实现，Go 栈需语义移植；passkey CSRF/hono GHSA 两项我们免疫/N-A）。落四项：① rest-timer 每账号上限 5（超限淘汰最旧，push.go order 队列）+ 设置限频 60 次/分；② 测试推送限频 10 次/10 分（rateWindow 按账号固定窗口）；③ PathGuard 中间件——反斜杠/点段路径一律 404（URL 解析器归一化而 nginx 不做，`..\` 静态规则绕过面，生产实测 404 ✓）；④ Notify 前查停用账号不投递。118 nginx 加 CSP（script-src 按 index.html 内联脚本 hash 白名单 + connect-src 开放 API 域；`scripts/check-csp.mjs` 校验 hash，deploy-118.sh 挂守护步——hash 漂移即显式失败防静默白屏）。cookie/bearer 分离语义：Go /api/me 无续期逻辑 → N/A 记档。生产验收：pathguard 404/CSP 头生效/页面功能无损/health 全绿 |
| 2026-11-07 | `merge-upstream-v144` → `main` (f6f5a2f7c..d446c9db0) | **上游全量 merge：v1.4.2–v1.4.4（937 commits 领先基线，663+ 合入）**。收：Modules 模块系统（19 locales 全中文化完好）/每组 vs 上次对比/组间对比/肌群周组数/力量等级/Webhooks(INTEGRATIONS)/名字包（瑞典+葡语 5,632）/Garmin/Integrations schemas。冲突 75 文件逐个解决：品牌/登录/admin/coach-console/契约冻结全保我方；Android **rename/add 定性**（上游 ch 路径新插件收进 ch 包 25 个+我方 com 包 7 插件共存，buildConfig+versionCode 31/1.4.4）；openapi 五块融合（info 版本 1.4.4/CSRF 新语义+我方豁免/pairing deprecated+watch tag/AdminQuota 重复段清理/审计清单 account.delete）；**上游 lazyView chunk 架构落地**（17 页拆包+空闲预取，我方独有页保持静态）。⚠️ 三坑：unshallow 必须先行（浅 merge-base 低估冲突面 663→937）；BodyGuard 预读后新 handler 必须 rawBody；HashRouter URL 带 #。**安全对齐前置**（b0054c731 语义移植先行，merge 时安全语义已在）。**生产验收**：新 bundle 上线/CSP 守卫首战拦截 hash 漂移/MODUES 中文引导弹窗实测/三套门全绿。⚠️ 事故与恢复：merge 中 30 个**未跟踪** Java 参考拷贝（上游 ch 文件的本地 com 路径拷贝，未编译未提交）被误清空——定位为上游文件的本地拷贝后从 upstream/main 全量恢复至 ch 正确路径（7 个 test 同源恢复）；教训：**merge 前先 commit/ignore 未跟踪文件** |
| 2026-11-24 | `claude/nostalgic-austin-94093b`（+1 修复提交） | **首页排版错位修复（生产 hotfix，随本次部署上线）**：合并提交 f6f5a2f7c 解决 Home.jsx 冲突时丢了 `.hdr` 闭合 `</div>`、周卡 `<div className="card">` 开标签与 progChip 渲染行——周导航/wkleft/today-row/选择其他训练沦为 `.hdr`（flex+nowrap）直接子元素被挤成一行逐字竖排（真机 WebView DOM 七子元素吻合确诊，adb devtools 直读；线上 CSS `index-CbyV1xnS.css` 哈希一致佐证同源）。按上游 8fb98b168 原文补回，Home.jsx 与上游仅剩品牌名一处差异；本地同源复现+修复验证（Home 测试 45 案、vite build 通过）；部署 app.qingmulife.com 后真机复验 |

> 后续改动在此追加；对外提供网络服务时，本节即 §13 源码可用性的入口之一。

## 6. 测试门口径（2026-10-21 基线实测）

```bash
./scripts/test-all.sh          # = Node 22 + TZ=UTC 跑 api/frontend/mcp 三套
```

基线：**api 605 / frontend 5060 / mcp 95 全绿**。

已知偶发（非本轮改动引起，重跑即绿）：全量并行跑时 `useStore.tabs-owner` / `useStore.adopt-hold` 偶有 1-6 红（多标签 storage 事件的并发时序），单文件连跑稳定；api 偶有 1 红亦为重跑即绿。

两个钉死的环境变量不是玄学（实测记录）：

- **Node 22**（与上游 `.github/workflows/test.yml` 一致）：Node ≥24 的实验性全局
  `localStorage`（值为 undefined）会遮蔽 happy-dom 的注入，frontend 565 用例红。
- **TZ=UTC**：`demoSeed.js` 用本地时区 `getHours()` 决定是否生成"今天"的演示训练。
  UTC+8 下 fake-noon 即本地 20 点 → 今天的 demo Push Day 生成 → coach-week 的
  DONE RULE 命中 → mcp 4 红 + frontend 3 红。上游 CI 在 UTC 下恰好避开；这是
  上游的时区可移植性问题（值得向上游提 issue/PR）。

## 7. 部署与本地开发

- 生产实例：PVE CT 151（openGym-allinone）@ 10.10.10.151 —— 更新用
  `scripts/deploy-ct151.sh`（回内网运行；`--api-only` 只更 API）。
- Docker Compose：`docker-compose up -d` → http://localhost:8080
  （⚠️ ghcr.io 在部分网络下拉取极慢；镜像含全量动作媒体，体积大）。
- **本机轻量全栈**（2026-10-21 验证可用）：
  ```bash
  # API:纯 Node,仅 3 个依赖(注意 3000 若被 SSH 隧道占用则换端口)
  cd api && npm ci
  PORT=3100 DATA_DIR=../data RP_ID=localhost ORIGIN=http://localhost:5173 \
    SESSION_DAYS=90 TRUST_PROXY=1 node server.js
  # 前端:Vite dev(/api 代理与媒体代理均内置,变量名是 API_TARGET 不是 API_ORIGIN)
  cd frontend && npm ci && API_TARGET=http://127.0.0.1:3100 npm run dev
  ```
  验证口径:`curl localhost:5173/api/health` → `{"ok":true,...}`；动作库图片走
  `/exercise-media/*` 由 Vite 从 `catalogue/media/` 直出。

- **Android APK 构建**（2026-10-22 验证）：
  ```bash
  cd frontend && npm run build:mobile      # ⚠️ 不是 npm run build！
  cd android && JAVA_HOME=$(/usr/libexec/java_home -v 21) ./gradlew assembleDebug
  ```
  ⚠️ **必须 `build:mobile`**：它才跑 `stage-media.mjs` 把 `catalogue/media`
  （6,554 静图 + 6,554 动画，215MB）复制进 `dist/exercise-media` 并 cap sync。
  用普通 `npm run build`（web 包）构建的 APK **动作动画全部无法播放**（2026-10-22 实测）。
  媒体 180px 按 Gym visual 授权仅随 App 包分发。
  ⚠️ **必须 JDK 21**（Capacitor 7 的编译源级；JDK 17 报「无效的源发行版：21」）。
  `cap sync` 会尝试 iOS 并报 CocoaPods 缺失（无害，或直接 `npx cap sync android`）。
  ⚠️ `build:mobile` 后 `dist` 是移动包 —— 部署 web 前要重跑 `npm run build`。
  产物 `android/app/build/outputs/apk/debug/app-debug.apk`（debug 包名 `com.muhe.fit.test`；
  release 版需自备 keystore 签名）。
