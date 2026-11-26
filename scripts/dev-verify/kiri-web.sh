#!/usr/bin/env bash
# kiri-web.sh — 起前端 dev(5173, W1 反代 /api→3100)并冒烟首页
set -uo pipefail
cd /Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4/frontend
OLD=$(lsof -nP -iTCP:5173 -sTCP:LISTEN 2>/dev/null | awk '/LISTEN/{print $2}' | head -1)
if [ -n "${OLD:-}" ]; then echo "5173 already on pid $OLD"; else
  nohup npm run dev > /tmp/vite-dev.log 2>&1 &
  sleep 7
fi
echo "--- index ---"
curl -s -o /dev/null -w "%{http_code}\n" -m 6 http://localhost:5173/
echo "--- vite log tail ---"
tail -4 /tmp/vite-dev.log
echo "--- api proxy via 5173 ---"
curl -s -o /dev/null -w "%{http_code}\n" -m 6 http://localhost:5173/api/health
