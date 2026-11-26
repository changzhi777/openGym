#!/usr/bin/env bash
# ack-pending.sh — 学员号重登录(旧 cookie 因 api Recreate SECRET 重生成失效)→
# 拿 pending 分发行 → ack loaded → 确认 pending 清零。BASE 默认内网口 3131。
set -euo pipefail
BASE="${BASE:-http://10.10.10.7:3131}"
HERE="$(cd "$(dirname "$0")" && pwd)"
JAR=/tmp/team-s3.jar; rm -f "$JAR"

CAP=$(curl -sf "$BASE/api/auth/captcha")
CID=$(echo "$CAP" | python3 -c "import sys,json;print(json.load(sys.stdin)['captchaId'])")
CC=$(echo "$CAP" | python3 "$HERE/captcha-code.py")
curl -sf -H 'Content-Type: application/json' \
  -d "{\"phone\":\"13900000731\",\"captchaId\":\"$CID\",\"captchaCode\":\"$CC\"}" \
  "$BASE/api/auth/sms/send" >/dev/null
sleep 1
SMS=$(/usr/bin/ssh -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes -F none \
  -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183 \
  "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 5 2>/dev/null | grep -o '[0-9]\{6\}' | tail -1")
echo "SMS=$SMS"
curl -sf -c "$JAR" -H 'Content-Type: application/json' \
  -d "{\"phone\":\"13900000731\",\"code\":\"$SMS\"}" "$BASE/api/auth/sms/verify" >/dev/null
echo "学员重登录 OK"

PID=$(curl -sf -b "$JAR" "$BASE/api/team/plans" | python3 -c "
import sys,json
d=json.load(sys.stdin)
pend=[p for p in d['plans'] if p['status']=='distributed' and not p.get('supersededBy')]
print(pend[0]['id'] if pend else '')")
[[ -n "$PID" ]] || { echo "无 pending 分发行——可能已结算"; exit 1; }
echo "pending 分发行 $PID"
curl -sf -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"status":"loaded"}' "$BASE/api/team/plans/$PID/ack" >/dev/null
echo "ack loaded 已结算"
PEND=$(curl -sf -b "$JAR" "$BASE/api/team/plans" | python3 -c "import sys,json;print(json.load(sys.stdin)['pending'])")
echo "ack 后 pending=$PEND"
