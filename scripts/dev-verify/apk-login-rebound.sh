#!/usr/bin/env bash
# apk-login-rebound.sh — 回弹布局一条龙(页面已滚动/键盘收起态):
# captcha图827 框815 发码973 短信格1197 登录1539 (1080x2400 原始坐标)
set -euo pipefail
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
D="adb -s c9e5cc0f shell"
SSH="/usr/bin/ssh -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183"

echo "== 1. 点 captcha 图刷新 =="
$D input tap 366 827; sleep 2.5
CAP=$(bash "$WT/scripts/dev-verify/redis-captcha.sh") || true
echo "CAPTCHA=$CAP"
case "$CAP" in
  [0-9][0-9][0-9][0-9]) ;;
  *) echo "captcha 读码失败"; adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_rb_fail.png; sips -Z 1900 /tmp/apk_rb_fail.png --out /tmp/apk_rb_fails.png >/dev/null; exit 1 ;;
esac

echo "== 2. 清填 captcha 发短信 =="
$D input tap 758 815; sleep 1.5
for i in 1 2 3 4 5 6; do $D input keyevent 67; done
$D input text "$CAP"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 537 973; sleep 2.5

echo "== 3. 抠 sms 码 =="
SMS=$($SSH "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 2 2>/dev/null | grep -oE '[0-9]{6}' | tail -1")
echo "SMS=$SMS"
case "$SMS" in
  [0-9][0-9][0-9][0-9][0-9][0-9]) ;;
  *) echo "sms 抠码失败"; exit 1 ;;
esac

echo "== 4. 清填 sms 码并登录 =="
$D input tap 202 1197; sleep 2
for i in 1 2 3 4 5 6 7 8; do $D input keyevent 67; done
$D input text "$SMS"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 537 1539; sleep 4

adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_rb_result.png
sips -Z 1900 /tmp/apk_rb_result.png --out /tmp/apk_rb_results.png >/dev/null
echo "DONE"
