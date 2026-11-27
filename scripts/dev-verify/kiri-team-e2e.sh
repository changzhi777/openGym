#!/usr/bin/env bash
# kiri-team-e2e.sh — kiri 本地造 team 端到端数据:owner 出码→学员登录入团→owner 分发
set -uo pipefail
BASE=http://127.0.0.1:3100
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
LOG=/tmp/muhe-api.log

login() { # login <phone> <jar>
  local phone="$1" jar="$2"
  local CAP CID CC SMS
  CAP=$(curl -sf "$BASE/api/auth/captcha")
  CID=$(echo "$CAP" | python3 -c "import sys,json;sys.stdout.write(json.load(sys.stdin)['captchaId'])")
  CC=$(echo "$CAP" | python3 "$WT/scripts/dev-verify/captcha-code.py")
  curl -sf -H 'Content-Type: application/json' \
    -d "{\"phone\":\"$phone\",\"captchaId\":\"$CID\",\"captchaCode\":\"$CC\"}" \
    "$BASE/api/auth/sms/send" >/dev/null
  sleep 1
  SMS=$(grep -oE '验证码 [0-9]{6}' "$LOG" | tail -1 | grep -oE '[0-9]{6}')
  curl -sf -c "$jar" -H 'Content-Type: application/json' \
    -d "{\"phone\":\"$phone\",\"code\":\"$SMS\"}" "$BASE/api/auth/sms/verify" >/dev/null && echo "login $phone OK"
}

rm -f /tmp/kiri_owner.jar /tmp/kiri_student.jar
login 13800000731 /tmp/kiri_owner.jar
echo "--- invite ---"
CODE=$(curl -sf -b /tmp/kiri_owner.jar -X POST -d '{}' "$BASE/api/team/invite" | python3 -c "import sys,json;d=json.load(sys.stdin);sys.stdout.write(d.get('code') or d.get('invite',{}).get('code') or '')")
echo "CODE=$CODE"
login 13900000731 /tmp/kiri_student.jar
echo "--- join ---"
curl -sf -b /tmp/kiri_student.jar -X POST -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}" "$BASE/api/team/join" | head -c 120; echo
echo "--- student uid ---"
SUID=$(curl -sf -b /tmp/kiri_student.jar "$BASE/api/me" | python3 -c "import sys,json;sys.stdout.write(json.load(sys.stdin)['user']['id'])")
echo "SUID=$SUID"
echo "--- distribute ---"
curl -sf -b /tmp/kiri_owner.jar -X POST -H 'Content-Type: application/json' \
  -d "{\"uid\":\"$SUID\",\"plan\":{\"name\":\"PPL-Week5\",\"routines\":[{\"name\":\"Push Day\",\"ex\":[{\"id\":\"ex-push-1\"},{\"id\":\"ex-push-2\"}]},{\"name\":\"Pull Day\",\"ex\":[{\"id\":\"ex-pull-1\"}]}]}}" \
  "$BASE/api/team/plans" | head -c 100; echo
echo "--- student team/plans ---"
curl -sf -b /tmp/kiri_student.jar "$BASE/api/team/plans" | head -c 200; echo
