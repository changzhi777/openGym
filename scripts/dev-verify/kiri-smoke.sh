#!/usr/bin/env bash
# kiri-smoke.sh — 本地 kiri 全栈登录链冒烟(BASE=127.0.0.1:3100,短信码抠本地 API 日志)
set -uo pipefail
BASE=http://127.0.0.1:3100
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
JAR=/tmp/kiri_jar.txt; rm -f "$JAR"

CAP=$(curl -sf "$BASE/api/auth/captcha") || { echo "captcha FAIL"; exit 1; }
CID=$(echo "$CAP" | python3 -c "import sys,json;sys.stdout.write(json.load(sys.stdin)['captchaId'])")
CC=$(echo "$CAP" | python3 "$WT/scripts/dev-verify/captcha-code.py")
curl -sf -H 'Content-Type: application/json' \
  -d "{\"phone\":\"13800000731\",\"captchaId\":\"$CID\",\"captchaCode\":\"$CC\"}" \
  "$BASE/api/auth/sms/send" >/dev/null && echo "sms/send OK"
sleep 1
SMS=$(grep -oE '验证码 [0-9]{6}' /tmp/muhe-api.log | tail -1 | grep -oE '[0-9]{6}')
echo "SMS=$SMS"
curl -sf -c "$JAR" -H 'Content-Type: application/json' \
  -d "{\"phone\":\"13800000731\",\"code\":\"$SMS\"}" "$BASE/api/auth/sms/verify" >/dev/null && echo "sms/verify OK"

echo "--- me ---"
curl -sf -b "$JAR" "$BASE/api/me" | head -c 120; echo
echo "--- team/plans ---"
curl -sf -b "$JAR" -o /tmp/kiri_plans.json -w "%{http_code}\n" "$BASE/api/team/plans"
head -c 200 /tmp/kiri_plans.json; echo
echo "--- data/rev ---"
curl -sf -b "$JAR" -o /dev/null -w "%{http_code}\n" "$BASE/api/data/rev"
