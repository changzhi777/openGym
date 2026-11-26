#!/usr/bin/env bash
# apk-login-full.sh — 真机 APK 全流程一条龙(重装/冷启后):
# 拉起 APK → 输手机号 → 点 captcha 刷新 → Redis 读码 → 填码 → 发短信 → 抠码 → 填码 → 登录
# 布局 = 初始品牌区完整布局(手机号1134/captcha图1320/captcha框1355/发码1522/短信码1692/登录2002)
set -euo pipefail
WT=/Volumes/user/cz_code/openGym/.claude/worktrees/upbeat-kirch-7f43a4
D="adb -s c9e5cc0f shell"
SSH="/usr/bin/ssh -i $HOME/.ssh/pve_root_ed25519 -o IdentitiesOnly=yes -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183"

echo "== 0. 拉起 APK =="
$D input keyevent KEYCODE_WAKEUP; sleep 1
adb -s c9e5cc0f shell am start -n com.muhe.fit.test/com.muhe.fit.MainActivity >/dev/null 2>&1 || true
sleep 3

echo "== 1. 输手机号 =="
$D input tap 546 1134; sleep 1.5
$D input text 13800000731; sleep 0.8
$D input keyevent 4; sleep 1

echo "== 2. 点 captcha 图刷新 + Redis 读码 =="
$D input tap 356 1320; sleep 2.5
CAP=$(bash "$WT/scripts/dev-verify/redis-captcha.sh")
echo "CAPTCHA=$CAP"
case "$CAP" in
  [0-9][0-9][0-9][0-9]) ;;
  *) echo "captcha 读码失败"; adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_full_fail.png; exit 1 ;;
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

adb -s c9e5cc0f exec-out screencap -p > /tmp/apk_full_result.png
sips -Z 1900 /tmp/apk_full_result.png --out /tmp/apk_full_results.png >/dev/null
echo "DONE — 截图 /tmp/apk_full_results.png"
