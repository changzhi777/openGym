# Fork 说明 — 本仓库的定位与合规边界

> 本文件是本 fork（changzhi777/openGym）的二次发行声明。基于上游
> [DuarteSantos8/openGym](https://github.com/DuarteSantos8/openGym)（AGPL-3.0），
> fork 时间：2026-10-21，基点：上游 `main`（318b57bf，v1.4.1 之后）。

## 1. 与上游的关系

- **跟进策略**：保留完整 git 历史；`upstream` 指向上游原仓库，定期 rebase/merge
  吸收上游 release（上游约两周一个版本）。
- **改动原则**：修改尽量收敛在独立分支与独立文件，减少与上游合并的冲突面。
- **本地 remote 布局**：`origin` = 本 fork；`upstream` = DuarteSantos8/openGym。

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

> 后续改动在此追加；对外提供网络服务时，本节即 §13 源码可用性的入口之一。

## 6. 测试门口径（2026-10-21 基线实测）

```bash
./scripts/test-all.sh          # = Node 22 + TZ=UTC 跑 api/frontend/mcp 三套
```

基线：**api 605 / frontend 5060 / mcp 95 全绿**。

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
