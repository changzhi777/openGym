# 会话纪要 — 2026-10-21/22 沐禾健身全栈冲刺（20 小时 / 25 提交）

> 本文档由 Claude Code 会话自动整理，记录 2026-10-21 22:30 → 2026-10-22 13:10 的完整工作历程。
> 目的：任何后续会话/协作者可据此无缝接续；关键决策与踩坑一并留档。

## 一、时间线与主要战役

| 时间段 | 战役 | 关键交付 |
|---|---|---|
| 22:30–00:30 | **品牌替换**：openGym → 沐禾健身 / Muhe Fit | i18n-core 渲染层单点替换（locale 键零改动）；manifest/capacitor(appId=com.muhe.fit)/API RP_NAME/备份目录/分享水印/.fit 软件名；Android 包名 com.muhe.fit |
| 00:30–01:30 | **图标 + APK** | 用户 logo（禾苗+太阳+水波）生成 PWA 三尺寸 + Android 6 密度；自适应图标 72/108 安全区几何修正；**JDK 21 硬要求** |
| 01:00–02:00 | **动作动画无法播放根治** | 根因：`npm run build` 不含 `stage-media.mjs`（215MB 媒体不进 dist）→ 必须 `build:mobile`；APK 35MB→228MB |
| 02:00–03:00 | **5,632 条动作说明全量中文化** | DeepSeek 批量翻译（74 万 tokens ≈2.2 元），术语表对齐 `i18n-terminology-audit`；新建 exercise-desc/zh.js |
| 03:00–08:00 | **后端 10 轮技术选型深挖** | 定稿 Go+Gin+pgx/sqlc+PG17+Redis7+oapi-codegen；方案 `.zcf/plan/current/saas-backend-go.md` |
| 04:00–07:00 | **Admin 前端 P0-P2**（并行） | 设计稿 1570 行；AdminShell/概览/用户管理/删号输入名解锁/step-up 框架/审计页 |
| 08:00–13:10 | **Go 后端 P0→P6** | 见下表 |

## 二、Go 后端分期与提交

| 期 | 内容 | 提交 |
|---|---|---|
| P0 | 契约冻结：openapi.yaml +7 端点；Node 侧 8 个 501 stub（上游双向契约测试强制 yaml↔route 一致） | 4e82b835, 9cf1e8f8 |
| P1 | 骨架：Gin+pgx+migrate 内嵌+16 表+scratch 镜像+compose.go.yml，health 三绿 | 0b55b30f |
| P2 | 认证 18/18 端点；session HMAC cookie 与 Node **字节级兼容** | 90516d89, 68561e78, 2d72641e |
| P3 | /api/data 全协议（baseRev+baseWid/409 携带文档/_wids 链/resetAt）；sync-stamps 移植；双向迁移器 round-trip 达标 | 3fc6dc50, ceec496d |
| P3.5 | Node 测试双跑桥（testmode 独立 PG 库/种子/boot line），39 测通过；三大缺口补（CSRF/CORS/config/BodyGuard） | 16fcf898, ac8fb12e |
| P4 | push 七端点（VAPID 共用 vapid.json/REST 计时）+ media 四端点（sha256/魔数/强缓存） | 8c156b99, 71cb371a |
| P5 | Admin 九端点+三角色+审计 fail-closed+step-up 窗口 | 342e4947 |
| P6 | 灰度脚本 gray-switch.sh(migrate/switch/rollback)+10 用户演练全绿 | d8128af3 |

## 三、关键踩坑清单（照抄会炸）

1. **golang-migrate**：文件名必须 `{v}_{name}.up.sql`（无 `.up.` 静默跳过、报 "first migrations: file does not exist"）；**同版本号改内容会被跳过** → 开发期改初始迁移必须删卷重建
2. **go-redis v9**：`Ping(ctx)` 返回 `*StatusCmd` 不是 error —— `== nil` 恒假
3. **oapi-codegen strict 模式**：binary requestBody 生成重复 `Body` 字段 ×7 —— 退非 strict
4. **users.id 必须是 text**：协议 uid 是 12 字节 base64url，不是 uuid
5. **Go 的 secret 读序**：env SECRET → `DATA_DIR/secret` 文件（灰度两栈同钥 = cookie 互通的前提）
6. **两栈存储分离**（Node=文件 / Go=PG）→ 切换必须**原子**（停机 Big-bang），灰度为演练而非长期双写
7. **Android**：`build:mobile`（媒体）+ JDK 21 + scratch 镜像（gcr 被墙）
8. **Node 测试双跑**：每实例独立库 `muhe_t_<rand>`（SIGKILL 退出需 TTL 清理）；磁盘/日志断言类测试是白名单差异（Go 存 PG 不写 state 文件）

## 四、质量护栏

- `./scripts/test-all.sh`（Node22+TZ=UTC）：api 605 / frontend 5060 / mcp 95 全绿
- Node 测试套件双跑桥（MUHE_GO=<二进制>）：39 测通过（cors 4/4、passkeys 29/29）
- Go 侧单测：stamps 6 测；`go vet` 全绿
- 每期冒烟：health 三绿 / CAS 五步 / round-trip 全等 / 跨栈 cookie 互通

## 五、待办（交接清单）

1. **生产切换**（等内网）：`backend-go/scripts/gray-switch.sh migrate → switch`；五条验收 = 计数对账 / round-trip 全等 / passkey 登录冒烟 / 10 用户抽样 / 回滚演练（24h 窗口）
2. P5 瘦尾：计费细化（usage_counters 异步落账 + 限流）、coach sidecar（Node 包装）、**step-up 的 assertion 验签补全**
3. media 完整语义：配额累计 / mp4 探针 / GC 引用计数（现 sweep 为安全 no-op）
4. 上游 issue #732（demoSeed 时区）/#733（Node≥24 localStorage）回应；#733 的 PR 材料已备（fork 提交 d8c6040b 的 vitest setup）
