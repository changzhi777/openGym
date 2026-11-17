#!/usr/bin/env bash
# scripts/deploy-118.sh — 从本机更新 43.129.201.118 的沐禾健身(Go 栈 + 前端静态)
#
# 用法:
#   ./scripts/deploy-118.sh            # 全量:源码 + compose build + 前端 dist + 验证
#   ./scripts/deploy-118.sh --web-only # 只更新前端 dist(API 没变时)
#
# 前提:
#   - 本机 SSH root@43.129.201.118 密钥免密(2026-10-29 已配)
#   - 118 已有 /opt/muhe-fit(compose.yml + data/ + web/;首装见 FORK.md 台账 2026-10-28 行)
#   - 域名: qingmulife.com(前端)/api.qingmulife.com(API),证书 Certbot 托管
#
# 幂等:tar 增量、compose build 分层缓存、rsync --delete;data 卷与 SECRET 永不触碰。

set -euo pipefail

HOST="${DEPLOY_118:-root@43.129.201.118}"
LOCAL_REPO="$(cd "$(dirname "$0")/.." && pwd)"
WEB_ONLY=0
[[ "${1:-}" == "--web-only" ]] && WEB_ONLY=1

r() { ssh -o BatchMode=yes -o ConnectTimeout=8 "$HOST" "$@"; }

echo "==> [1/5] 连通性"
r "echo ok" >/dev/null || { echo "❌ $HOST 不可达"; exit 1; }

if [[ $WEB_ONLY -eq 0 ]]; then
  echo "==> [2/5] 打包 Go 栈源码(bsdtar 坑:排除项带 ./ 前缀会匹配任意段,勿写 './data') → 上传 → compose build"
  TAR=/tmp/muhe-src.tar.gz
  tar -czf "$TAR" \
    --exclude node_modules --exclude .git --exclude .zcf --exclude .claude \
    --exclude 'backend-go/build' \
    -C "$LOCAL_REPO" backend-go api/openapi.yaml api/coach package.json
  scp -q "$TAR" "$HOST:/opt/muhe-fit/muhe-src.tar.gz" && rm -f "$TAR"
  r bash -c "'cd /opt/muhe-fit && tar -xzf muhe-src.tar.gz && rm muhe-src.tar.gz && docker compose build 2>&1 | tail -2 && docker compose up -d'"
else
  echo "==> [2/5] --web-only 跳过 API"
fi

echo "==> [3/5] 前端构建 + rsync(web 版;APK 走 build:mobile,见 frontend/android/README.md)"
( cd "$LOCAL_REPO/frontend" && npm run build >/dev/null 2>&1 )
rsync -az --delete "$LOCAL_REPO/frontend/dist/" "$HOST:/opt/muhe-fit/web/"

echo "==> [4/5] 验证"
r bash -c "'curl -s -m 6 http://127.0.0.1:3100/api/health'"
# 2026-11-18 审查轮:第一个 curl 曾缺 URL 参数(必 exit 2,set -e 直接掐死脚本,
# bare 域验证从未执行)。三行各自独立验证,不再用 && 串联。
curl -s -m 8 -o /dev/null -w "bare:%{http_code} " https://qingmulife.com/api/health
curl -s -m 8 -o /dev/null -w "api:%{http_code}\n" https://api.qingmulife.com/api/health

echo "==> [5/5] 完成(数据备份 = 服务器 /opt/muhe-fit/data + pg 卷 muhe-fit_pgdata)"
