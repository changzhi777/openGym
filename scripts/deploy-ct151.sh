#!/usr/bin/env bash
# scripts/deploy-ct151.sh — 从本机更新 PVE CT 151 上的 openGym(API + 前端静态)
#
# 用法:
#   ./scripts/deploy-ct151.sh            # 全量:rsync 代码 + API 更新 + 前端构建 + nginx
#   ./scripts/deploy-ct151.sh --api-only # 只更新 API(前端没变时)
#
# 前提:
#   - 本机能 SSH root@10.10.10.10(PVE 宿主机,密钥登录)
#   - CT 151 已存在且 /opt/openGym 已部署(2026-10-20 基线:Node 22 + api npm ci + systemd)
#   - 首次运行会安装 nginx 并写站点配置(幂等,后续运行跳过)
#
# 幂等性:所有步骤可重复执行;rsync 增量同步,不删 CT 侧数据目录。

set -euo pipefail

PVE_HOST="${PVE_HOST:-root@10.10.10.10}"
CT="${CT:-151}"
LOCAL_REPO="$(cd "$(dirname "$0")/.." && pwd)"
API_ONLY=0
[[ "${1:-}" == "--api-only" ]] && API_ONLY=1

pve() { ssh -o ConnectTimeout=8 "$PVE_HOST" "$@"; }
ct()  { pve "pct exec $CT -- $*"; }

echo "==> [1/6] PVE 连通性"
pve "echo ok" >/dev/null || { echo "❌ PVE $PVE_HOST 不可达(需在内网)"; exit 1; }

echo "==> [2/6] rsync 代码(排除 node_modules/.git/data/dist)"
rsync -az --delete \
  --exclude node_modules --exclude .git --exclude data --exclude dist \
  --exclude .env --exclude .zcf --exclude .serena --exclude .spec-workflow \
  --exclude .playwright-mcp --exclude frontend/docs \
  "$LOCAL_REPO/" "$PVE_HOST:/opt/openGym-src/"
pve "pct push $CT /opt/openGym-src /opt/openGym --delete --exclude node_modules --exclude data" \
  || pve "rsync -az --delete --exclude node_modules --exclude data --exclude .env /opt/openGym-src/ 127.0.0.1:/opt/openGym/ 2>/dev/null" \
  || { echo "❌ 代码进 CT 失败(检查 pct push/rsync)"; exit 1; }
pve "rm -rf /opt/openGym-src"

echo "==> [3/6] API 依赖 + 重启"
ct bash -c "cd /opt/openGym/api && npm ci --omit=dev && systemctl restart openGym-api"

if [[ $API_ONLY -eq 1 ]]; then
  echo "==> --api-only:跳过前端"
else
  echo "==> [4/6] 前端构建(Vite 产物)"
  ct bash -c "cd /opt/openGym/frontend && npm ci && npm run build"

  echo "==> [5/6] nginx(首次安装 + 站点配置,幂等)"
  ct bash -c '
    command -v nginx >/dev/null || { apt-get update -qq && apt-get install -y -qq nginx; }
    cat > /etc/nginx/sites-available/opengym <<EOF
server {
    listen 80;
    server_name _;
    root /opt/openGym/frontend/dist;
    index index.html;
    location /api/ { proxy_pass http://127.0.0.1:3000; proxy_set_header Host \$host; }
    location / { try_files \$uri \$uri/ /index.html; }
}
EOF
    ln -sf /etc/nginx/sites-available/opengym /etc/nginx/sites-enabled/opengym
    rm -f /etc/nginx/sites-enabled/default
    systemctl enable --now nginx >/dev/null 2>&1 || true
    systemctl reload nginx
  '
fi

echo "==> [6/6] 健康检查"
sleep 2
ct bash -c "curl -sf http://127.0.0.1:3000/api/health && echo"
[[ $API_ONLY -eq 0 ]] && ct bash -c "curl -sf -o /dev/null -w 'web: HTTP %{http_code}\n' http://127.0.0.1/"
echo "✅ CT $CT 部署完成"
