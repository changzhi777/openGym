#!/usr/bin/env bash
# kiri-api.sh — 本地 kiri 起 Go API(合并后代码)并冒烟
set -uo pipefail
cd /Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4/backend-go
mkdir -p .zcf-data-dev
OLD=$(lsof -nP -iTCP:3100 -sTCP:LISTEN 2>/dev/null | awk '/LISTEN/{print $2}' | head -1)
if [ -n "${OLD:-}" ]; then echo "kill stale pid $OLD"; kill -9 "$OLD" 2>/dev/null; sleep 2; fi
(set -a; source .env; set +a; DATA_DIR="$PWD/.zcf-data-dev" MIGRATE_ON_START=1 GOCACHE="$PWD/.gocache" nohup go run ./cmd/api > /tmp/muhe-api.log 2>&1 &)
sleep 18
echo "--- health ---"
curl -s -m 5 http://127.0.0.1:3100/api/health; echo
echo "--- captcha ---"
curl -s -o /dev/null -w "%{http_code}\n" -m 5 http://127.0.0.1:3100/api/auth/captcha
echo "--- team plans (401 expected) ---"
curl -s -o /dev/null -w "%{http_code}\n" -m 5 http://127.0.0.1:3100/api/team/plans
echo "--- api log tail ---"
tail -5 /tmp/muhe-api.log
