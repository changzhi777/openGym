#!/usr/bin/env bash
# redis-captcha.sh — 读 183 Redis 里活着的 captcha 答案键(captcha:id:<id>,TTL 5min)
# 真机页面刷新 captcha 后立刻跑,输出最新 4 位码(按键名时间戳取最新一条)
set -euo pipefail
/usr/bin/ssh -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes -F none \
  -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no qm@100.98.98.183 '
cd ~/muhe-open183
KEYS=$(docker compose -p muhe-go-open183 exec -T redis redis-cli --scan --pattern "captcha:id:*" | sort | tail -3)
NEWEST=$(echo "$KEYS" | sort -t p -k 3 | tail -1)
if [ -z "$NEWEST" ]; then echo "NO_CAPTCHA_KEY"; exit 1; fi
docker compose -p muhe-go-open183 exec -T redis redis-cli GET "$NEWEST"
'
