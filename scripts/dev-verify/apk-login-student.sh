#!/usr/bin/env bash
# apk-login-student.sh — 真机登学员 13900000731(登出后初始布局,手机号框已聚焦键盘已开)
set -euo pipefail
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
D="adb -s c9e5cc0f shell"
SSH="/usr/bin/ssh -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183"

echo "== 1. 输学员手机号 =="
$D input text 13900000731; sleep 0.8
$D input keyevent 4; sleep 1

echo "== 2. Redis 读 captcha(登出后 mount 已拉新键) =="
CAP=$(bash "$WT/scripts/dev-verify/redis-captcha.sh") || true
echo "CAPTCHA=$CAP"
case "$CAP" in
  [0-9][0-9][0-9][0-9]) ;;
  *) echo "captcha 读码失败,点图重试"; $D input tap 356 1320; sleep 2.5
     CAP=$(bash "$WT/scripts/dev-verify/redis-captcha.sh") || true
     echo "CAPTCHA=$CAP"
     case "$CAP" in [0-9][0-9][0-9][0-9]) ;; *) exit 1 ;; esac ;;
esac

echo "== 3. 填 captcha 发短信 =="
$D input tap 746 1355; sleep 1.5
$D input text "$CAP"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 540 1522; sleep 2.5

echo "== 4. 抠 sms 码 =="
SMS=$($SSH "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 2 2>/dev/null | grep -oE '[0-9]{6}' | tail -1")
echo "SMS=$SMS"
case "$SMS" in
  [0-9][0-9][0-9][0-9][0-9][0-9]) ;;
  *) echo "sms 抠码失败"; exit 1 ;;
esac

echo "== 5. 填 sms 码登录 =="
$D input tap 204 1692; sleep 2
for i in 1 2 3 4 5 6 7 8; do $D input keyevent 67; done
$D input text "$SMS"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 540 2002; sleep 4

adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_student.png
sips -Z 1900 /tmp/apk_student.png --out /tmp/apk_students.png >/dev/null
echo "DONE"
