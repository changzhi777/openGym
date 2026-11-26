#!/usr/bin/env bash
# apk-login-once.sh — 真机 APK 一条龙登录(解锁后跑):
# 点 captcha 图刷新 → Redis 读码 → 填码 → 发短信 → 抠 sms 码 → 清格填码 → 登录 → 截图
# 依赖: 手机已解锁+APK 前台停在登录页(收键盘态布局)
set -euo pipefail
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
D="adb -s c9e5cc0f shell"
SSH="/usr/bin/ssh -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183"

echo "== 1. 点 captcha 图刷新 =="
$D input tap 350 893
sleep 2.5
CAP=$("$WT/scripts/dev-verify/redis-captcha.sh")
echo "CAPTCHA=$CAP"
case "$CAP" in
  [0-9][0-9][0-9][0-9]) ;;
  *) echo "captcha 读码失败"; exit 1 ;;
esac

echo "== 2. 填 captcha 并发短信 =="
$D input tap 206 895; sleep 1.5
$D input text "$CAP"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 540 1056; sleep 2.5

echo "== 3. 抠 sms 码 =="
SMS=$($SSH "cd ~/muhe-open183 && docker compose -p muhe-go-open183 logs api --tail 2 2>/dev/null | grep -oE '[0-9]{6}' | tail -1")
echo "SMS=$SMS"
case "$SMS" in
  [0-9][0-9][0-9][0-9][0-9][0-9]) ;;
  *) echo "sms 抠码失败"; exit 1 ;;
esac

echo "== 4. 清格填 sms 码并登录 =="
$D input tap 206 1280; sleep 2
for i in 1 2 3 4 5 6 7 8; do $D input keyevent 67; done
$D input text "$SMS"; sleep 0.8
$D input keyevent 4; sleep 1
$D input tap 540 1536; sleep 4

adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_login_result.png
sips -Z 1900 /tmp/apk_login_result.png --out /tmp/apk_login_results.png >/dev/null
echo "DONE — 截图 /tmp/apk_login_results.png"
