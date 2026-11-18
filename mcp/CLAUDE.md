[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **mcp · MCP 只读桥（算法单源三实现之一）**

# mcp/ — 沐禾健身 MCP 服务器

> Model Context Protocol stdio 桥：让 Claude Desktop / Cursor 等 LLM 客户端**只读**访问一个用户的训练数据。无新容器、无新认证、无网络调用——文件系统即边界。🆕 AI-Native 已实施（2026-11-11 六批次完毕）：原 P1 规划"工具注册表提升为 Coach 内核工具运行时（同一注册表两消费者）"**已按方案 v2 R3 修订作废**，实际落地为**算法单源、三实现**——本模块 JS 实现照旧不动（外部 LLM 桥），Go 内核独立实现 + golden 等价验收。本模块代码零改动，见下方"AI-Native 后定位"节。

## 模块职责

- 由 LLM 客户端拉起的 stdio 进程，直接读 API 写的同一 `./data`（`state-<uid>.json`）
- 暴露 9 个只读工具：`list_routines` / `get_routine` / `preview_session` / `get_week_plan` / `list_workouts` / `get_workout` / `get_bodyweight` / `estimate_1rm` / `muscle_balance`（`src/tools.js` L675 `TOOLS` 数组实测 9 个；⚠️ 该文件头注释仍写 "eight read-only tools"，属源码内注释漂移，以 TOOLS 数组为准）
- 计算复用 **frontend 的同一套纯函数**（`estimate1RM`、`loadOfWorkouts` 等），答案与 Stats 屏完全一致
- LLM 永远看不到 passkey/VAPID/会话密钥

## 技术栈与入口

| 项 | 值 |
|----|----|
| 运行时依赖 | `@modelcontextprotocol/sdk` + `zod`，仅此两个 |
| 入口 | `src/index.js`（bin: `opengym-mcp`）；`npm start` |
| 环境变量 | `OPENGYM_DATA`（data 目录）、`OPENGYM_UID`（单用户实例自动探测，可省） |
| 测试 | Vitest（基线 95 用例，夹具 `frontend/src/lib/demoSeed.js`，`vi.useFakeTimers` 钉死"今天"；**必须 TZ=UTC**，UTC+8 会假红 4 例——见根 CLAUDE.md 测试门） |

## 文件地图

| 文件 | 职责 |
|------|------|
| `src/index.js` | 装配：stdio transport + 工具注册 |
| `src/tools.js` | 9 个 MCP 工具定义（zod schema 校验；`TOOLS` 数组是注册表单点） |
| `src/state.js` | 加载/派生用户数据（读 data 目录） |
| `src/labels.js` | 内部键 → 人类可读标签（LLM 免再解读） |
| `scripts/check-node-loadable.mjs` | Node 可加载性检查（CI） |
| `test/` | labels / state / tools 三套测试 |

## 🆕 AI-Native 后定位（R3 修订，2026-11-11 实施完毕）

**v1 方案（作废）**：工具注册表从 `mcp/src/tools.js` 提炼进 Coach 内核（`api/coach/core/agent-tools.js`），"同一注册表、两个消费者"。

**实际落地（方案 v2 R3 裁决）**：**算法单源、三实现**——不复用注册表，只锚定同一算法源：

| 实现 | 通道 | 算法来源 | 状态 |
|------|------|----------|------|
| frontend UI（Stats 屏 / Coach） | 进程内直接 import `src/lib` | `frontend/src/lib`（**唯一算法源**：onerm.js 等，纯函数 + 同目录单测） | ✅ 原生 |
| mcp（本模块） | stdio（外部 LLM 客户端拉起） | Node ESM import 同一批 `src/lib` 纯函数 | ✅ 照旧不动 |
| Coach Agent 内核 | Go 进程内（`backend-go/internal/agent`） | `internal/agent/onerm.go` **独立翻译实现**，经 **golden 4480 案 JS↔Go 等价验收**锚定（批次 2）；🆕 `plan.go`（get_week_plan 组装层，批次 6 后段）经 **plan golden 八场景**锚定——夹具即用本模块真 handler 生成 | ✅ 已实施 |

golden 等价验收机制（防双端漂移）：

- `scripts/golden/gen-onerm.mjs` 用 **frontend 真函数**生成夹具 `backend-go/internal/agent/testdata/onerm-golden.json`（头注写明"期望值由 frontend/src/lib/onerm.js 真函数生成，禁止手编"）
- `onerm_golden_test.go` 比对 Go 实现输出；内置 **selfcheck**（防"某侧公式名拼错双双回退同一慢路径"的假绿）与夹具厚度下限断言（≥1000 案 + ≥50 精度样）
- 🆕 `scripts/golden/gen-plan.mjs` 用 **MCP 真 handler**（get_week_plan 组装层）生成 `testdata/plan-golden.json` 八场景夹具（时钟钉死；golden 对照连组装层一起锚定的教训见 FORK.md 台账）
- 改 `frontend/src/lib/onerm.js` 后必须重跑 `node scripts/golden/gen-onerm.mjs` 再跑 Go 测试

对本模块的含义：**代码零改动、定位不变**（外部 LLM 只读桥）；"Coach 内核工具运行时"职责由 Go 侧 `internal/agent/tools_setup.go` 自建工具集承担（estimate_1rm / propose_constraint / propose_memory / **get_week_plan** / get_bodyweight / search_knowledge 共 **6 工具**，2026-11-18 复核；get_week_plan 批次 6 后段入表，与本模块 get_week_plan 口径对齐 + golden 锚定，但独立实现）。

## 设计约束（README 明言）

1. MCP SDK 之外零运行时依赖——无 DB 驱动、无 HTTP 框架
2. 不加容器（stdio 由客户端拉起，不进 docker-compose）
3. 不加认证（文件系统即边界）
4. 无遥测、无网络

## 与 frontend 的复用关系（改前端时注意）

- 直接以 Node ESM import `frontend/src/lib/*.js`（`history.js`、`onerm.js`、`muscles.js`、`exercises.js`）
- 为此 `i18n.js` 拆成 `i18n-core.js`（纯、Node 安全，**现为品牌单点**：MCP 输出的品牌名随其 BRAND 渲染为沐禾健身/Muhe Fit）+ `i18n.js`（Vite/React 位）
- **改这些文件时不得破坏 Node 可加载性**（CI 的 check:node-loadable 盯着）
- 🆕 `src/lib` 纯函数现有**三个消费者**：frontend UI、本模块、Coach Agent 内核（Go 侧经翻译 + golden 锚定，非直接 import）——改算法必须同跑三处验证（前端 vitest / mcp vitest / `node scripts/golden/gen-onerm.mjs` 或 `gen-plan.mjs` + `go test ./internal/agent/`）

## Roadmap（README）

- Phase 2（未发）：读写工具 + 长效 token（`data/tokens.json`）+ state 写锁
- Phase 3（未发）：Streamable HTTP 传输、可选第 4 容器
- （AI-Native 与本 Roadmap 互不阻塞：Coach 内核经 Go 侧自建工具集已拿到只读"感官"，本模块 Phase 2 读写工具仍不进 Coach 内核）

## 客户端接入

Claude Desktop `claude_desktop_config.json`：`mcpServers.opengym = { command: "node", args: ["<绝对路径>/mcp/src/index.js"], env: { OPENGYM_DATA, OPENGYM_UID } }`。详见 [mcp/README.md](./README.md)。

---

变更记录: 2026-11-18 V1.4 审查轮核准（init-architect）:Go 侧工具清单 5→**6 工具**（get_week_plan 已于批次 6 后段入表，tools_setup.go L94 实测；与本模块 get_week_plan 口径对齐 + plan golden 八场景锚定）；三实现表 Go 行补 `plan.go`/gen-plan.mjs；三处验证纪律补 gen-plan 分支。本模块代码依旧零改动（9 工具 / TOOLS 数组 / "eight" 头注漂移均沿袭 V1.2 记录）。2026-11-11 V1.3 增量（init-architect，实施后刷新轮）:"定位升级规划"节改写为"AI-Native 后定位"——v1 方案"同一注册表两消费者"（mcp/tools.js 提炼进 api coach/core/agent-tools.js）**按方案 v2 R3 修订作废**，实际落地 = 算法单源三实现（frontend src/lib 唯一算法源；本模块 JS 照旧；Go 内核 internal/agent/onerm.go 独立实现 + golden 4480 案等价验收，gen-onerm.mjs 产夹具 + selfcheck 防假绿）；本模块代码零改动；复用关系更新为三消费者与三处验证纪律。2026-11-10 V1.2 增量:新增"定位升级规划"节（AI-Native P1，当时规划态）；实测核对 9 工具（tools.js 头注释 "eight" 属源码注释漂移）；2026-10-27 V1.1 核对式更新:品牌名/包名 muhe-mcp/测试口径 TZ=UTC 注记，无代码变化；2026-10-20 V1.0 初版
