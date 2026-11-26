#!/usr/bin/env bash
# kiri-team-smoke.sh — 建团冒烟 + plans + 前端 5173 状态
set -uo pipefail
JAR=/tmp/kiri_jar.txt
echo "--- POST /api/team (create) ---"
curl -sf -b "$JAR" -H 'Content-Type: application/json' -d '{"name":"kiri-smoke-team"}' \
  http://127.0.0.1:3100/api/team | head -c 150; echo
echo "--- GET /api/team/my ---"
curl -sf -b "$JAR" -o /tmp/kiri_team.json -w "%{http_code}\n" http://127.0.0.1:3100/api/team/my
head -c 150 /tmp/kiri_team.json 2>/dev/null; echo
echo "--- GET /api/team/plans ---"
curl -sf -b "$JAR" -o /tmp/kiri_plans.json -w "%{http_code}\n" http://127.0.0.1:3100/api/team/plans
head -c 120 /tmp/kiri_plans.json 2>/dev/null; echo
echo "--- frontend 5173 ---"
curl -s -o /dev/null -w "%{http_code}\n" -m 4 http://127.0.0.1:5173/ || echo "not running"
