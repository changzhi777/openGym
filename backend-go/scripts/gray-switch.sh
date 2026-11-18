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
  echo "== 0/4 环境闸门 =="
  # 2026-11-18 审查轮批次 B:两栈同钥是 cookie 互通的硬前提。shell 里残留的
  # SECRET env 会让 Go 用它、Node 仍读 DATA_DIR/secret → 全量用户掉登录。
  if [ -n "${SECRET:-}" ]; then
    echo "❌ 检测到 SECRET env——两栈同钥要求 Go 与 Node 读同一 DATA_DIR/secret,"
    echo "   请 unset SECRET 后重跑(或确认你确实要覆盖式换钥,那是另一套规程)"
    exit 1
  fi
  echo "== 1/4 停 Node(systemd 或进程) =="
  systemctl stop opengym 2>/dev/null || pkill -f "node server.js" || true
  sleep 1
  echo "== 2/4 起 Go(nohup 脱离会话;SSH 断线不再 SIGHUP 直杀刚上线的生产) =="
  nohup "$API_BIN" >/tmp/muhe-go.log 2>&1 &
  echo $! > /tmp/muhe-go.pid
  for i in $(seq 1 20); do sleep 1; curl -sf "http://127.0.0.1:${PORT:-3100}/api/health" >/dev/null 2>&1 && break; done
  curl -sf "http://127.0.0.1:${PORT:-3100}/api/health" || { echo "❌ health 不过,立即回滚"; "$0" rollback; exit 1; }
  echo "== 3/4 验收:登录冒烟(人工) =="
  echo "   请在浏览器打开前端,用任一用户登录并确认训练数据完整"
  echo "== 4/4 回滚窗口 24h:数据目录保留原样,DATA_DIR 未动 =="
  echo "✅ switch 完成(五条第 4/5 条人工完成:登录冒烟 + 10 用户抽样)"
  ;;
rollback)
  echo "== 回滚:停 Go,起 Node =="
  [ -f /tmp/muhe-go.pid ] && kill "$(cat /tmp/muhe-go.pid)" 2>/dev/null || pkill -f muhe-api || true
  sleep 1
  # 2026-11-18 审查轮批次 B:回滚前反向导出 Go 窗口期写入到 JSON。窗口内用户的
  # 训练记录/体重全部落 PG,直接切回 Node 会静默丢掉这些写入。导出幂等且只写
  # 导出目录——Node 侧数据是否采用由操作者对照后决定,脚本不自动覆盖 DATA_DIR。
  if [ -x "$MIG_BIN" ]; then
    STAMP=$(date +%Y%m%d_%H%M%S)
    OUT=/tmp/muhe-go-window-$STAMP
    if "$MIG_BIN" export "$OUT"; then
      echo "↩️ Go 窗口期写入已导出: $OUT(对照无误后再并回 DATA_DIR;含 state-*.json)"
      if command -v python3 >/dev/null && compgen -G "$OUT/state-*.json" >/dev/null; then
        python3 - "$OUT" <<'PY'
import json,sys,glob
n=len(glob.glob(sys.argv[1]+'/state-*.json'))
print(f"   导出 states={n}(0 = 窗口期无状态写入,可直接回滚)")
PY
      fi
    else
      echo "⚠️ muhemigrate export 失败——继续回滚将丢失 Go 窗口期全部写入;"
      echo "   Ctrl-C 中止排查,或 30 秒后脚本继续(export 目录可能部分可用)"
      sleep 30
    fi
  else
    echo "⚠️ 找不到 muhemigrate($MIG_BIN)——无法导出 Go 窗口期写入,继续回滚将丢失之"
  fi
  systemctl start opengym 2>/dev/null || (cd api && nohup node server.js >/tmp/muhe-node.log 2>&1 &)
  # 回滚也要验收(批次 B:此前起完就走,Node 是否真起来无人知道)。
  NODE_OK=0
  for i in $(seq 1 20); do
    sleep 1
    curl -sf "http://127.0.0.1:${NODE_PORT:-3000}/api/health" >/dev/null 2>&1 && { NODE_OK=1; break; }
  done
  [ "$NODE_OK" -eq 1 ] && echo "✅ 已回滚到 Node(health 绿)" || echo "❌ Node health 20s 未绿——立即人工检查 /tmp/muhe-node.log"
  [ "$NODE_OK" -eq 1 ] || exit 1
  ;;
*)
  echo "用法: $0 migrate|switch|rollback"; exit 2
  ;;
esac
