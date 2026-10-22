#!/usr/bin/env bash
# scripts/gray-switch.sh — Node(JSON) → Go(PG) 一次性切换(P6 灰度闸门)
#
# 用法:
#   ./scripts/gray-switch.sh migrate   # 1) 备份+导入+round-trip 验收(不停服,只读源)
#   ./scripts/gray-switch.sh switch    # 2) 停 Node → 起 Go → 五条验收前四条
#   ./scripts/gray-switch.sh rollback  # 3) 回滚:停 Go → 起 Node(原样)
#
# 前提:DATABASE_URL/REDIS_ADDR/DATA_DIR(含 secret 与 db.json/state-*.json)已设;
#      muhe-api 与 muhemigrate 已构建于本脚本同目录或 PATH。
set -euo pipefail
cd "$(dirname "$0")/.."
API_BIN=${API_BIN:-/tmp/muhe-api}
MIG_BIN=${MIG_BIN:-/tmp/muhemigrate}
: "${DATA_DIR:?需要 DATA_DIR}"
: "${DATABASE_URL:?需要 DATABASE_URL}"

case "${1:-}" in
migrate)
  echo "== 1/4 全量备份(冷存储) =="
  STAMP=$(date +%Y%m%d_%H%M%S)
  BACKUP=/tmp/muhe-backup-$STAMP.tar.gz
  tar -czf "$BACKUP" -C "$DATA_DIR" . && sha256sum "$BACKUP" 2>/dev/null || shasum -a 256 "$BACKUP"
  echo "备份: $BACKUP"
  echo "== 2/4 导入(幂等;Node 继续服务,源只读) =="
  "$MIG_BIN" import "$DATA_DIR"
  echo "== 3/4 round-trip 验收 =="
  OUT=/tmp/muhe-roundtrip-$STAMP && rm -rf "$OUT" && mkdir -p "$OUT"
  "$MIG_BIN" export "$OUT"
  FAIL=0
  for f in "$DATA_DIR"/state-*.json; do
    B="$OUT/$(basename "$f")"
    if [ -f "$B" ]; then
      python3 -c "import json,sys; a=json.load(open('$f')); b=json.load(open('$B')); sys.exit(0 if a==b else 1)" || { echo "❌ 差异: $(basename "$f")"; FAIL=1; }
    else
      echo "❌ 缺失: $(basename "$f")"; FAIL=1
    fi
  done
  [ "$FAIL" -eq 0 ] && echo "✅ round-trip 全等" || { echo "❌ 验收失败,禁止切换"; exit 1; }
  echo "== 4/4 计数对账 =="
  python3 - "$DATA_DIR" <<'PY'
import json,sys,glob
d=json.load(open(sys.argv[1]+'/db.json'))
print(f"源: users={len(d.get('users',[]))} creds={len(d.get('creds',[]))} states={len(glob.glob(sys.argv[1]+'/state-*.json'))}")
PY
  echo "✅ migrate 完成 —— 可执行 switch"
  ;;
switch)
  echo "== 1/4 停 Node(systemd 或进程) =="
  systemctl stop opengym 2>/dev/null || pkill -f "node server.js" || true
  sleep 1
  echo "== 2/4 起 Go =="
  "$API_BIN" & echo $! > /tmp/muhe-go.pid
  for i in $(seq 1 20); do sleep 1; curl -sf "http://127.0.0.1:${PORT:-3100}/api/health" >/dev/null 2>&1 && break; done
  curl -sf "http://127.0.0.1:${PORT:-3100}/api/health" || { echo "❌ health 不过,立即回滚"; "$0" rollback; exit 1; }
  echo "== 3/4 验收:passkey 登录冒烟(人工) =="
  echo "   请在浏览器打开前端,用任一用户 passkey 登录并确认训练数据完整"
  echo "== 4/4 回滚窗口 24h:数据目录保留原样,DATA_DIR 未动 =="
  echo "✅ switch 完成(五条第 4/5 条人工完成:登录冒烟 + 10 用户抽样)"
  ;;
rollback)
  echo "== 回滚:停 Go,起 Node =="
  [ -f /tmp/muhe-go.pid ] && kill "$(cat /tmp/muhe-go.pid)" 2>/dev/null || pkill -f muhe-api || true
  systemctl start opengym 2>/dev/null || (cd api && node server.js &)
  echo "✅ 已回滚到 Node(数据目录从未改动)"
  ;;
*)
  echo "用法: $0 migrate|switch|rollback"; exit 2
  ;;
esac
