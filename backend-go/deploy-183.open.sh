#!/usr/bin/env bash
# deploy-183-open.sh — openGym 测试版部署 qm-ub(100.98.98.183,Tailscale 直达):
# backend-go 源 tar + frontend dist tar 双包 scp → 183 解包 → .env 种子 →
# compose(muhe-go-open183 项目,override:api 3130 + nginx 3131 一站式)→ 冒烟。
# dist 打包前自动 stage-media(catalogue 授权动作演示素材 → dist/exercise-media,
# PWA 动作库/训练页的动画依赖它;2026-11-26 真机验证灰占位后补进流程)。
# 参照 muhe-fit deploy-183.sh(qm 密钥免密/三次重试/项目名隔离同机 WeKnora 与 muhe-fit 实例)。
# 用法: bash deploy-183.open.sh [--no-smoke]   (前端 dist 先本地 npm run build;媒体本脚本自动补)
set -euo pipefail

HOST183="${HOST183:-qm@100.98.98.183}"
REMOTE="${REMOTE:-muhe-open183}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"          # worktree 根
SMOKE=1
REDEPLOY=0
for a in "$@"; do
  [[ "$a" == "--no-smoke" ]] && SMOKE=0
  [[ "$a" == "--update" ]] && REDEPLOY=1
done

# -F none + /usr/bin/ssh 绝对路径双保险:绕开 ssh config(per-host 把 183 劫到 22222)
# 与 PATH 里的 ssh wrapper(deploy-118 过 GFW 用的 -p 22222 前置包装器,会劫持脚本内 ssh)。
# 183 走 Tailscale 内网裸连 22。
SSH_BIN=/usr/bin/ssh
SSH_OPTS=(-o ConnectTimeout=25 -i "$HOME/.ssh/pve_root_ed25519" -o IdentitiesOnly=yes \
  -F none -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no)
r() { local i rc; for i in 1 2 3; do
  "$SSH_BIN" "${SSH_OPTS[@]}" "$HOST183" "$@" && return 0
  rc=$?; echo "-- ssh 第 $i 次失败(rc=$rc),5s 后重试"; sleep 5
done; return $rc; }

echo "==> [1/6] qm-ub 连通性${REDEPLOY:+(--update 重部署:跳过首部署端口检查)}"
r "echo ok" >/dev/null || { echo "qm-ub 不可达(Tailscale/密钥)"; exit 1; }
if [[ $REDEPLOY -ne 1 ]]; then
  r "ss -tln 2>/dev/null | grep -Eq ':3130 |:3131 ' && { echo '端口 3130/3131 已被占用'; exit 1; } || echo ports-free"
fi

echo "==> [2/6] 打包 backend-go 源 + frontend dist"
TAR_API=/tmp/open183-api.tar.gz
TAR_WEB=/tmp/open183-dist.tar.gz
tar -czf "$TAR_API" --exclude .env --exclude '*.test' \
  -C "$REPO/backend-go" \
  Dockerfile compose.go.yml deploy-183.override.yml nginx-183.conf go.mod go.sum cmd internal
[[ -f "$REPO/frontend/dist/index.html" ]] || { echo "frontend/dist 缺失——先 npm run build"; exit 1; }
[[ -d "$REPO/catalogue/media" ]] || { echo "catalogue/media 缺失——动作演示素材无从 stage"; exit 1; }
echo "-- stage-media:授权动作素材进 dist/exercise-media(still webp + clip mp4)"
node "$REPO/scripts/catalogue/stage-media.mjs" "$REPO/frontend/dist"
tar -czf "$TAR_WEB" -C "$REPO/frontend" dist

echo "==> [3/6] 传输(源码+dist;Tailscale 直连快,DERP 慢链耐心等)"
scp -q "${SSH_OPTS[@]}" "$TAR_API" "$TAR_WEB" "$HOST183:/tmp/"
r "mkdir -p ~/$REMOTE && tar -xzf /tmp/open183-api.tar.gz -C ~/$REMOTE && mkdir -p ~/$REMOTE/frontend && tar -xzf /tmp/open183-dist.tar.gz -C ~/$REMOTE/frontend && rm -f /tmp/open183-*.tar.gz"

echo "==> [4/6] .env 就位"
if r "test -f ~/$REMOTE/.env"; then
  echo "-- 183 侧 .env 已在,保留(POSTGRES_PASSWORD 沿用)"
else
  echo "-- 种子 .env(随机口令)"
  PW=$(openssl rand -hex 12)   # tr|head 会吃 SIGPIPE 触发 pipefail 静默退出——openssl 免管道
  r "echo 'POSTGRES_PASSWORD=$PW' > ~/$REMOTE/.env"
fi

echo "==> [5/6] compose build + up(muhe-go-open183:pg16+redis7+api+nginx;迁移内嵌)"
r "cd ~/$REMOTE && docker compose -f compose.go.yml -f deploy-183.override.yml -p muhe-go-open183 up -d --build api pg redis nginx"

echo "==> [6/6] 冒烟"
sleep 5
chk() { local d="$1" c="$2"; if [ "$c" = "200" ]; then echo "  ok: $d"; else echo "  FAIL: $d(得 $c)"; FAIL=1; fi }
FAIL=0
chk "api 直连 health(3130)" "$(r "curl -s -o /dev/null -w '%{http_code}' -m 8 http://127.0.0.1:3130/api/health")"
chk "nginx PWA index(3131)" "$(r "curl -s -o /dev/null -w '%{http_code}' -m 8 http://127.0.0.1:3131/")"
chk "nginx 反代 /api/health(3131)" "$(r "curl -s -o /dev/null -w '%{http_code}' -m 8 http://127.0.0.1:3131/api/health")"
chk "动作演示素材 still(3131)" "$(r "curl -s -o /dev/null -w '%{http_code}' -m 8 http://127.0.0.1:3131/exercise-media/still/0001.webp")"
chk "动作演示素材 clip(3131)" "$(r "curl -s -o /dev/null -w '%{http_code}' -m 8 http://127.0.0.1:3131/exercise-media/clip/0001.mp4")"
if [[ $FAIL -eq 1 ]]; then echo "冒烟有 FAIL——查日志: ssh $HOST183 'cd ~/$REMOTE && docker compose -p muhe-go-open183 logs api --tail 20'"; exit 1; fi
[[ $SMOKE -eq 1 ]] || echo "--no-smoke:已跳过深度冒烟"
echo "部署完成:qm-ub ~/$REMOTE(compose 项目 muhe-go-open183)
  API 直连  http://100.98.98.183:3130
  一站式口  http://100.98.98.183:3131  (PWA + /api + /coach-console)"
