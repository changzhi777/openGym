# dev-verify · 本地/真机验证工具链（fork）

kiri 本地全栈冒烟、183 测试版登录链、真机 APK 驱动三类可复用验证脚本。
环境假设：本机 kiri（backend-go/.env + compose.go.yml 依赖容器）+ Tailscale/局域网直达 183（qm@100.98.98.183）+ USB 真机（设备序列 c9e5cc0f）。

## kiri 本地全栈

| 脚本 | 用途 |
|------|------|
| `kiri-api.sh` | 杀残留进程后起 Go API（go run，MIGRATE_ON_START=1 自动上库）+ health/captcha/team 冒烟 |
| `kiri-smoke.sh` | 登录链冒烟：captcha 抽码（captcha-code.py）→ sms/send → 抠本地 API 日志验证码 → verify → me/team/plans/data-rev |
| `kiri-team-smoke.sh` | 建团（POST /api/team）→ plans 空列表 → 5173 状态 |
| `kiri-web.sh` | 起前端 dev（5173，W1 反代 /api→3100）+ 首页/反代冒烟（vite 绑 IPv6，用 `localhost` 访问勿用 127.0.0.1） |

前置：`docker compose -f compose.go.yml up -d pg redis`（backend-go/.env 提供连接串与 SECRET）。

## 183 测试版登录链（免手输核心）

| 脚本 | 用途 |
|------|------|
| `captcha-code.py` | 从 /api/auth/captcha 的 SVG 里抽 4 位答案（text 明文） |
| `redis-captcha.sh` | 183 Redis 直读活着的 captcha 答案键（`captcha:id:*`，TTL 5min）——真机页面点图刷新后立刻跑 |
| `login-coach.sh` | 服务器侧 curl 全链登录拿会话 cookie（BASE 默认 http://10.10.10.7:3131） |
| `apk-login-{once,full,rebound,student}.sh` | 真机 APK 一条龙登录四布局态：初始/回弹/学员/通用（点 captcha 刷新 → Redis 读码 → 填码发短信 → 日志抠码 → 填码登录），15 秒内完成绕开验证码 TTL |

## 真机/部署辅助

| 脚本 | 用途 |
|------|------|
| `verify-team.sh` | team 域 e2e（17 断言：建团/邀请/分发/ACK/看板），183 内网口 |
| `ack-pending.sh` | 学员号重登录 → ACK 分发行 → pending 清零 |
| `w2-compare.sh` | W2 数据库双向同步一致性硬对比（本地拉取库 vs 生产逐字段） |
| `media-183.sh` | 183 补动作演示素材（stage-media → scp → dist/exercise-media → curl 验证） |

约定：`.zcf/tmp/` 仍为 gitignored 本地 scratch；可复用脚本收编本目录后相互引用一律走 `scripts/dev-verify/` 相对路径。
