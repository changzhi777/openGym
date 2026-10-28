[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **mcp · MCP 只读桥**

# mcp/ — 沐禾健身 MCP 服务器

> Model Context Protocol stdio 桥：让 Claude Desktop / Cursor 等 LLM 客户端**只读**访问一个用户的训练数据。无新容器、无新认证、无网络调用——文件系统即边界。本轮核对：**无代码改动**，仅品牌名与包名更新（包名 `muhe-mcp`）。

## 模块职责

- 由 LLM 客户端拉起的 stdio 进程，直接读 API 写的同一 `./data`（`state-<uid>.json`）
- 暴露 9 个只读工具：`list_routines` / `get_routine` / `preview_session` / `get_week_plan` / `list_workouts` / `get_workout` / `get_bodyweight` / `estimate_1rm` / `muscle_balance`
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
| `src/tools.js` | 9 个 MCP 工具定义（zod schema 校验） |
| `src/state.js` | 加载/派生用户数据（读 data 目录） |
| `src/labels.js` | 内部键 → 人类可读标签（LLM 免再解读） |
| `scripts/check-node-loadable.mjs` | Node 可加载性检查（CI） |
| `test/` | labels / state / tools 三套测试 |

## 设计约束（README 明言）

1. MCP SDK 之外零运行时依赖——无 DB 驱动、无 HTTP 框架
2. 不加容器（stdio 由客户端拉起，不进 docker-compose）
3. 不加认证（文件系统即边界）
4. 无遥测、无网络

## 与 frontend 的复用关系（改前端时注意）

- 直接以 Node ESM import `frontend/src/lib/*.js`（`history.js`、`onerm.js`、`muscles.js`、`exercises.js`）
- 为此 `i18n.js` 拆成 `i18n-core.js`（纯、Node 安全，**现为品牌单点**：MCP 输出的品牌名随其 BRAND 渲染为沐禾健身/Muhe Fit）+ `i18n.js`（Vite/React 位）
- **改这些文件时不得破坏 Node 可加载性**（CI 的 check:node-loadable 盯着）

## Roadmap（README）

- Phase 2（未发）：读写工具 + 长效 token（`data/tokens.json`）+ state 写锁
- Phase 3（未发）：Streamable HTTP 传输、可选第 4 容器

## 客户端接入

Claude Desktop `claude_desktop_config.json`：`mcpServers.opengym = { command: "node", args: ["<绝对路径>/mcp/src/index.js"], env: { OPENGYM_DATA, OPENGYM_UID } }`。详见 [mcp/README.md](./README.md)。

---

变更记录: 2026-10-27 V1.1 核对式更新（init-architect）:品牌名/包名 muhe-mcp/测试口径 TZ=UTC 注记，无代码变化；2026-10-20 V1.0 初版
