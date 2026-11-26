#!/usr/bin/env bash
# login-coach.sh — 服务器侧走完整短信登录链(183 测试版)拿教练会话 cookie
# 用法: bash login-coach.sh [phone]  默认 13800000731;cookie 写 /tmp/coach_jar.txt
set -euo pipefail
BASE="${BASE:-http://10.10.10.7:3131}"
PHONE="${1:-13800000731}"
HERE="$(cd "$(dirname "$0")" && pwd)"
JAR=/tmp/coach_jar.txt; rm -f "$JAR"

CAP=$(curl -sf "$BASE/api/auth/captcha")
CID=$(echo "$CAP" | python3 -c "import sys,json;sys.stdout.write(json.load(sys.stdin)['captchaId'])")
CC=$(echo "$CAP" | python3 "$HERE/captcha-code.py")
curl -sf -H 'Content-Type: application/json' \
  -d "{\"phone\":\"$PHONE\",\"captchaId\":\"$CID\",\"captchaCode\":\"$CC\"}" \
  "$BASE/api/auth/sms/send" >/dev/null
sleep 1
SMS=$(/usr/bin/ssh -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes -F none \
  -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183 \
  "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 2 2>/dev/null | grep -oE '[0-9]{6}' | tail -1")
echo "SMS=$SMS"
curl -sf -c "$JAR" -H 'Content-Type: application/json' \
  -d "{\"phone\":\"$PHONE\",\"code\":\"$SMS\"}" "$BASE/api/auth/sms/verify"
echo
echo "--- cookie jar ---"
cat "$JAR"
