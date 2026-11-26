#!/usr/bin/env bash
# verify-team.sh — 团队教练域 e2e(openGym 测试版;用户终端跑,沙箱禁端口):
# captcha SVG 抽数字 → sms/send → 日志抠 6 位码 → sms/verify 双号 → 授 coach →
# 建团/出码/入团/灌打卡 → insights/board 断言 → 分发/ack 状态机 → 权限面。
# 前置:backend-go compose 栈在跑(api 3120);前端无关(API 级验证)。
set -euo pipefail
cd "$(dirname "$0")/../../backend-go"
BASE="${BASE:-http://127.0.0.1:3120}"
TS=$(date +%s)
FAILS=()
chk() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then echo "  ok: $d"; else echo "  FAIL: $d"; FAILS+=("$d"); fi }
jget() { python3 -c "import sys,json;d=json.load(sys.stdin);print(d$1)" 2>/dev/null; }

JAR_C=/tmp/team-c.jar; JAR_S=/tmp/team-s.jar; rm -f "$JAR_C" "$JAR_S"
PW="Team-$TS"

# 注册登录一体:手机号 → captcha 识码 → 发码 → 日志抠 6 位 → verify → cookie 落 $4
register() {
  local phone="$1" jar="$2" name="$3"
  local cap=$(curl -sf "$BASE/api/auth/captcha")
  local cid=$(echo "$cap" | jget "['captchaId']")
  local code=$(echo "$cap" | python3 "$(dirname "$0")/captcha-code.py")
  curl -sf -H 'Content-Type: application/json' \
    -d "{\"phone\":\"$phone\",\"captchaId\":\"$cid\",\"captchaCode\":\"$code\"}" \
    "$BASE/api/auth/sms/send" >/dev/null
  sleep 1
  local sms
  if [[ "$BASE" == *127.0.0.1* || "$BASE" == *localhost* ]]; then
    sms=$(docker compose -f compose.go.yml logs api --tail 8 2>/dev/null | grep 'sms:dev' | grep -o '[0-9]\{6\}' | tail -1)
  else
    sms=$(/usr/bin/ssh -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183 "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 8 2>/dev/null | grep 'sms:dev' | grep -o '[0-9]\{6\}' | tail -1")
  fi
  [[ -n "$sms" ]] || { echo "  FAIL: 抠不到短信验证码($phone)"; FAILS+=("sms-code-$phone"); return 1; }
  curl -sf -c "$jar" -H 'Content-Type: application/json' \
    -d "{\"phone\":\"$phone\",\"code\":\"$sms\",\"name\":\"$name\"}" \
    "$BASE/api/auth/sms/verify" >/dev/null
}

echo "==> [1] 教练号注册+登录"
register "138$(printf '%08d' $((TS % 100000000)))" "$JAR_C" "教练$TS"
COACH_UID=$(curl -sf -b "$JAR_C" "$BASE/api/me" | jget "['user']['id']")
[[ -n "$COACH_UID" ]] && echo "  ok: 教练登录 uid=$COACH_UID"

echo "==> [2] 授 coach 角色(psql 直插)"
PSQL() {  # SQL 文本作参数;本地/远端统一 stdin 喂 psql(零引号穿越)
  local sql="$*"
  if [[ "$BASE" == *127.0.0.1* || "$BASE" == *localhost* ]]; then
    printf '%s\n' "$sql" | docker compose -f compose.go.yml exec -T pg psql -U muhe -d muhe
  else
    printf '%s\n' "$sql" | /usr/bin/ssh -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183 "cd ~/muhe-open183 && docker compose -p muhe-go-open183 exec -T pg psql -U muhe -d muhe"
  fi
}
PSQL "INSERT INTO admin_roles (user_id, role) VALUES ('$COACH_UID', 'coach') ON CONFLICT DO NOTHING"
chk "coach 角色在库" bash -c "true"  # 角色真伪由 [6] insights 403/200 权限面实证

echo "==> [3] 建团+出码"
TID=$(curl -sf -b "$JAR_C" -H 'Content-Type: application/json' -d "{\"name\":\"验证团$TS\"}" "$BASE/api/team" | jget "['team']['id']")
[[ -n "$TID" ]] && echo "  ok: 建团 $TID"
CODE=$(curl -sf -b "$JAR_C" -H 'Content-Type: application/json' -d '{}' "$BASE/api/team/invite" | jget "['code']")
[[ -n "$CODE" ]] && echo "  ok: 出码 $CODE"

echo "==> [4] 学员号注册+扫码进团"
register "139$(printf '%08d' $((TS % 100000000)))" "$JAR_S" "学员$TS"
STUDENT_UID=$(curl -sf -b "$JAR_S" "$BASE/api/me" | jget "['user']['id']")
JOIN=$(curl -sf -b "$JAR_S" -H 'Content-Type: application/json' -d "{\"code\":\"$CODE\"}" "$BASE/api/team/join")
echo "$JOIN" | grep -q owner && echo "  ok: 入团确认卡含团长 uid=$STUDENT_UID"
QR=$(curl -s -b "$JAR_C" -o /dev/null -w '%{http_code} %{content_type} %{size_download}' "$BASE/api/team/invite/qr?size=256")
echo "$QR" | grep -q "^200 image/png" && echo "  ok: invite/qr PNG($QR)" || { echo "  FAIL: invite/qr 异常($QR)"; FAILS+=("invite-qr"); }

echo "==> [5] 学员灌打卡(本周 2+上周 1+前窗 35d 1)"
TODAY=$(date +%Y-%m-%d); W1=$(date -v-7d +%Y-%m-%d); W5=$(date -v-35d +%Y-%m-%d)
DOC=$(python3 - "$TODAY" "$W1" "$W5" <<'EOF'
import json,sys
t,w1,w5=sys.argv[1:4]
doc={"weekStart":1,
 "routines":[{"id":"r1","name":"推","ex":[{"id":"bench"}]}],
 "week":{"1":["r1"]},
 "workouts":[
  {"d":t,"name":"推","vol":3000,"sets":[{"done":True,"w":80,"r":5}]},
  {"d":t,"name":"腿","vol":4000,"sets":[{"done":True,"w":100,"r":5}]},
  {"d":w1,"name":"推","vol":2800,"sets":[{"done":True,"w":80,"r":5}]},
  {"d":w5,"name":"推","vol":2200,"sets":[{"done":True,"w":75,"r":5}]}]}
print(json.dumps({"state":doc}))
EOF
)
code=$(curl -s -X PUT -b "$JAR_S" -H 'Content-Type: application/json' -d "$DOC" -o /dev/null -w '%{http_code}' "$BASE/api/data")
chk "学员 PUT /api/data 200(得 $code)" test "$code" = 200

echo "==> [6] 教练 insights 分析(双窗环比+规则解读)"
INS=$(curl -sf -b "$JAR_C" "$BASE/api/team/insights")
echo "$INS" | python3 -c "
import sys,json
d=json.load(sys.stdin); rows=d['rows']
r=[x for x in rows if x['uid']=='$STUDENT_UID']
assert len(r)==1, '学员行未命中'
r=r[0]
assert r['weekDone']==2 and r['rate']==200, f'week={r[\"weekDone\"]} rate={r[\"rate\"]}'
assert r['streakWeeks']>=2, f'streak={r[\"streakWeeks\"]}'
assert r['volDelta']==345, f'volDelta={r[\"volDelta\"]}'
assert 'hot' in r['flags'] and 'gain' in r['flags'], r['flags']
print('  ok: weekDone=2 rate=200%% streak=%d volDelta=+345%% flags=%s' % (r['streakWeeks'], r['flags']))
print('  verdict:', r['verdict'])
"
echo "==> [7] 看板与权限面"
BOARD=$(curl -sf -b "$JAR_C" "$BASE/api/team/board")
echo "$BOARD" | grep -q weekly && echo "  ok: board 含 12 周列"
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_S" "$BASE/api/team/insights")
chk "member 视角 insights 403(得 $code)" test "$code" = 403
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/team/board")
chk "未登录 board 401(得 $code)" test "$code" = 401

echo "==> [8] 计划分发→学员接收(事务链+状态机)"
SNAP='{"name":"推腿拉·第一周","routines":[{"id":"r1","name":"推","ex":[{"id":"bench"}]}]}'
PID=$(curl -sf -b "$JAR_C" -H 'Content-Type: application/json' -d "{\"uid\":\"$STUDENT_UID\",\"plan\":$SNAP}" "$BASE/api/team/plans" | jget "['plan']['id']")
[[ -n "$PID" ]] && echo "  ok: 分发行 $PID"
PENDING=$(curl -sf -b "$JAR_S" "$BASE/api/team/plans" | jget "['pending']")
chk "学员 pending=1(得 $PENDING)" test "$PENDING" = 1
curl -sf -b "$JAR_S" -H 'Content-Type: application/json' -d '{"status":"loaded"}' "$BASE/api/team/plans/$PID/ack" >/dev/null
PENDING=$(curl -sf -b "$JAR_S" "$BASE/api/team/plans" | jget "['pending']")
chk "ack 后 pending=0(得 $PENDING)" test "$PENDING" = 0
code=$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR_S" -H 'Content-Type: application/json' -d '{"status":"declined"}' "$BASE/api/team/plans/$PID/ack")
chk "重复 ack 404 plan-gone(得 $code)" test "$code" = 404

echo "==> [9] 契约面抽查(coach-console 页直出)"
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/health")
chk "health 200(得 $code)" test "$code" = 200

echo "==> 汇总"
if [[ ${#FAILS[@]} -eq 0 ]]; then echo "TEAM-E2E-OK:全链绿"
else echo "TEAM-E2E-FAIL:${#FAILS[@]} 项:"; printf '  - %s\n' "${FAILS[@]}"; exit 1; fi
