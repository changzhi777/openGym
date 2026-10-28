#!/usr/bin/env bash
# coach-sidecar/smoke.sh — 教练 sidecar 桥的端到端冒烟(fork)。
# 一条命令验证:Go 反代 → sidecar(fixture provider)→ state 桥回读 PG → 提案就绪。
#
# 用法: ./coach-sidecar/smoke.sh [PG_PORT]   # 默认 55432;需 docker 的测试 PG/Redis 在跑
#   docker run -d --name muhe-test-pg -e POSTGRES_USER=muhe -e POSTGRES_PASSWORD=muhe \
#     -e POSTGRES_DB=muhe -p 55432:5432 postgres:16-alpine
#   docker run -d --name muhe-test-redis -p 56379:6379 redis:7-alpine
set -euo pipefail
cd "$(dirname "$0")/.."

PG_PORT="${1:-55432}"
PG="postgres://muhe:muhe@127.0.0.1:${PG_PORT}/muhe?sslmode=disable"
REDIS="127.0.0.1:56379"
DATA=$(mktemp -d /tmp/muhe-coach-smoke.XXXX)
TOKEN="smoke-token-$$"
BIN=/tmp/muhe-build/muhe-api-darwin

trap 'kill $GO_PID $SC_PID 2>/dev/null || true; docker exec muhe-test-pg psql -U muhe -d muhe -c "DROP DATABASE IF EXISTS smoke_coach WITH (FORCE)" >/dev/null 2>&1 || true; rm -rf "$DATA"' EXIT

[[ -x $BIN ]] || (cd backend-go && go build -o "$BIN" ./cmd/api)

echo "==> [1/7] 建库 + 启 Go(fixture:COACH_SIDECAR_ADDR/INTERNAL_TOKEN)"
docker exec muhe-test-pg psql -U muhe -d muhe -c "DROP DATABASE IF EXISTS smoke_coach WITH (FORCE)" >/dev/null 2>&1 || true
docker exec muhe-test-pg psql -U muhe -d muhe -c "CREATE DATABASE smoke_coach" >/dev/null
printf 'smoke-secret-%s' "$$" > "$DATA/secret"
DATABASE_URL="$(echo "$PG" | sed 's#/muhe?#/smoke_coach?#')" REDIS_ADDR=$REDIS PORT=3110 DATA_DIR="$DATA" \
  COACH_SIDECAR_ADDR=http://127.0.0.1:3111 INTERNAL_TOKEN=$TOKEN "$BIN" >"$DATA/go.log" 2>&1 &
GO_PID=$!

echo "==> [2/7] 启 sidecar(数据在 PG,走桥)"
PORT=3111 DATA_DIR="$DATA" COACH_STATE_FROM=go GO_API=http://127.0.0.1:3110 \
  INTERNAL_TOKEN=$TOKEN node coach-sidecar/server.mjs >"$DATA/sidecar.log" 2>&1 &
SC_PID=$!
sleep 3

COOKIE=$(node -e '
const c=require("crypto");
const payload="u1:"+(Date.now()+86400000)+":0";
console.log("gymsid="+payload+"."+c.createHmac("sha256",process.argv[1]).update(payload).digest("base64url"));' "smoke-secret-$$")

echo "==> [3/7] 种子:用户 + 有 consent 的 state"
DB="docker exec muhe-test-pg psql -U muhe -d smoke_coach -t -c"
$DB "INSERT INTO users (id,name) VALUES ('u1','Smoke')" >/dev/null
$DB "INSERT INTO states (user_id,doc,rev) VALUES ('u1','{\"unit\":\"kg\",\"workouts\":[],\"routines\":[],\"coach\":{\"consent\":{\"agreedAt\":\"1790000000\",\"version\":1}}}'::jsonb,0)" >/dev/null

echo "==> [4/7] 启用 fixture provider(admin 角色)"
$DB "INSERT INTO admin_roles (user_id,role) VALUES ('u1','super_admin')" >/dev/null
r=$(curl -sf -m 5 -X POST http://127.0.0.1:3110/api/admin/coach/config -H "Cookie: $COOKIE" \
  -H 'Content-Type: application/json' -d '{"enabled":true,"provider":"fixture"}')
[[ $r == *ok* ]] || { echo "❌ config: $r"; exit 1; }

echo "==> [5/7] state 桥(consent 必须从 PG 读回)"
s=$(curl -sf -m 5 http://127.0.0.1:3110/internal/coach/state/u1 -H "Authorization: Bearer $TOKEN")
[[ $s == *consent* ]] || { echo "❌ state 桥: $s"; exit 1; }

echo "==> [6/7] plan 入队 → fixture 执行"
curl -sf -m 8 -X POST http://127.0.0.1:3110/api/coach/plan -H "Cookie: $COOKIE" \
  -H 'Content-Type: application/json' -d '{"intake":{"days":3,"minutes":45}}' >/dev/null
sleep 2

echo "==> [7/7] 提案就绪"
st=$(curl -sf -m 5 http://127.0.0.1:3110/api/coach/status -H "Cookie: $COOKIE")
[[ $st == *opengym_plan* || $st == *\"pending\":\{\"id\"*bundle* ]] || { echo "❌ status: $st"; exit 1; }
echo "✅ coach sidecar 桥全链路冒烟通过(fixture provider)"
