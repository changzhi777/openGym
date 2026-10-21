#!/usr/bin/env bash
# scripts/test-all.sh — fork 测试门(FORK.md §7 口径):Node 22 + TZ=UTC,三套全跑
# 用法: ./scripts/test-all.sh [api|frontend|mcp]   # 不带参数跑全部
#
# 为什么钉死这两个环境变量(2026-10-21 实测):
# - Node >=24 的实验性全局 localStorage(值为 undefined)遮蔽 happy-dom 的注入
#   → frontend 565 个用例红(Cannot read 'getItem' of undefined)
# - demoSeed.js 用本地时区 getHours() 判断"傍晚前不生成今天的训练";
#   UTC+8 下 fake-noon=本地 20 点 → 今天的 demo Push Day 会生成 → coach-week
#   判 done → mcp/frontend 共 7 个用例红。上游 CI(node22/UTC)恰好两者都避开。
set -euo pipefail
cd "$(dirname "$0")/.."

# Node 22 = 上游 CI 版本(.github/workflows/test.yml)
if [[ -x /opt/homebrew/opt/node@22/bin/node ]]; then
  export PATH="/opt/homebrew/opt/node@22/bin:$PATH"
fi
node -e 'process.exit(+process.versions.node.split(".")[0]===22?0:1)' \
  || echo "⚠️  当前 node $(node -v) ≠ 22,frontend 测试可能大面积红(见脚本头注释)" >&2
export TZ=UTC

if [[ $# -eq 0 ]]; then set -- api frontend mcp; fi
for t in "$@"; do
  echo "===== $t ====="
  (cd "$t" && { [[ -d node_modules ]] || npm ci --no-audit --no-fund >/dev/null; } && npm test)
done
