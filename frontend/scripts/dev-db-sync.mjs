// scripts/dev-db-sync.mjs — fork-only(W2,2026-11-25):开发双后端数据库双向同步。
//
// 方向:
//   pull  云端生产(118 muhe-fit-pg-1) → 本地 kiri PG —— 开发时拉真实数据,覆盖本地。
//   push  本地 → 云端生产 —— 覆盖生产,**执行前自动先在 118 备份现库**;仅迁移上线时用。
//
// 通道:ssh -p 22222 root@43.129.201.118 → docker exec muhe-fit-pg-1 pg_dump/psql;
// 本地连接串读 backend-go/.env 的 DATABASE_URL。plain SQL + gzip 流,零临时文件落盘。
// 用法: node scripts/dev-db-sync.mjs pull|push [--dry-run]
//
// 安全:push 前置备份是硬闸(没有备份不推送);UI 侧另有强确认(vite 插件端点)。
import { readFileSync } from 'node:fs'
import { spawn } from 'node:child_process'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const SSH = ['ssh', '-p', '22222', '-o', 'ConnectTimeout=15', '-o', 'StrictHostKeyChecking=accept-new', 'root@43.129.201.118']
const REMOTE_PG = 'muhe-fit-pg-1'
const REMOTE_BACKUP_DIR = '/opt/muhe-fit/backups'

const dryRun = process.argv.includes('--dry-run')
const direction = process.argv[2] || ''
if (!['pull', 'push'].includes(direction)) {
  console.error('用法: node scripts/dev-db-sync.mjs pull|push [--dry-run]')
  process.exit(2)
}

// 本地连接串(backend-go/.env);muhe 库与 postgres 管理库。
function localUrls() {
  const env = readFileSync(resolve(root, 'backend-go/.env'), 'utf8')
  const m = env.match(/^DATABASE_URL=(.+)$/m)
  if (!m) throw new Error('backend-go/.env 里没有 DATABASE_URL')
  const base = m[1].trim()
  const admin = base.replace(/\/muhe\?/, '/postgres?')
  return { base, admin }
}

// stdout 一律 Buffer 收集——gzip 二进制流经字符串累积会损坏(utf8 转换丢字节)。
const run = (cmd, args, input, { silent = false } = {}) => new Promise((res, rej) => {
  const p = spawn(cmd, args, { stdio: ['pipe', 'pipe', 'pipe'] })
  const chunks = []
  let err = ''
  p.stdout.on('data', d => { chunks.push(d); if (!silent) process.stdout.write(d) })
  p.stderr.on('data', d => { err += d; process.stderr.write(d) })
  p.on('close', code => {
    const out = Buffer.concat(chunks)
    if (code === 0) res(out)
    else rej(new Error(`${cmd} 退出 ${code}: ${err.slice(-400)}`))
  })
  if (input) p.stdin.end(input); else p.stdin.end()
})

const step = msg => console.log(`==> ${msg}`)

async function pull() {
  const { base, admin } = localUrls()
  step('从云端 muhe-fit-pg-1 导出(plain SQL + gzip)')
  if (dryRun) { console.log('[dry-run] ' + [...SSH, `docker exec ${REMOTE_PG} pg_dump -U muhe --no-owner muhe | gzip`].join(' ')); return }
  const gz = await run(SSH[0], [...SSH.slice(1), `docker exec ${REMOTE_PG} pg_dump -U muhe --no-owner muhe | gzip`], null, { silent: true })
  console.log(`   导出 ${(gz.length / 1024).toFixed(0)} KiB(gzip)`)
  // DROP DATABASE 不允许在事务块里:psql 多个 -c 会拼进单事务,必须逐条 -c。
  step('重建本地库 muhe(先断活动连接→drop→create)')
  await run('psql', [admin, '-v', 'ON_ERROR_STOP=1', '-c', "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='muhe' AND pid<>pg_backend_pid();"])
  await run('psql', [admin, '-v', 'ON_ERROR_STOP=1', '-c', 'DROP DATABASE IF EXISTS muhe;'])
  await run('psql', [admin, '-v', 'ON_ERROR_STOP=1', '-c', 'CREATE DATABASE muhe;'])
  step('灌入本地(gunzip | psql)')
  await run('bash', ['-c', 'gunzip | psql -v ON_ERROR_STOP=1 -q "' + base + '"'], gz)
  step('pull 完成——本地 muhe-api 重启后会按迁移补齐缺失版本')
}

async function push() {
  const { base } = localUrls()
  const stamp = new Date().toISOString().replace(/[-:T]/g, '').slice(0, 14)
  const remoteBak = `${REMOTE_BACKUP_DIR}/muhe-before-push-${stamp}.sql.gz`
  step(`1/3 云端现库备份 → 118:${remoteBak}(硬闸:没有这一步不推)`)
  const bakCmd = `mkdir -p ${REMOTE_BACKUP_DIR} && docker exec ${REMOTE_PG} pg_dump -U muhe --no-owner muhe | gzip > ${remoteBak} && ls -la ${remoteBak}`
  if (dryRun) { console.log('[dry-run] ' + [...SSH, bakCmd].join(' ')); console.log('[dry-run] 本地 dump + 推送 restore 跳过'); return }
  await run(SSH[0], [...SSH.slice(1), bakCmd])
  step('2/3 本地导出(plain SQL + gzip)')
  const gz = await run('bash', ['-c', `pg_dump --no-owner "${base}" | gzip`])
  step('3/3 推送并在云端 restore(muhe 库重建)')
  const restoreCmd = `docker exec -i ${REMOTE_PG} bash -c 'psql -U postgres -c "DROP DATABASE IF EXISTS muhe;" -c "CREATE DATABASE muhe;" && gunzip | psql -U muhe -v ON_ERROR_STOP=1 -q muhe'`
  await run('bash', ['-c', `gunzip | ${SSH.join(' ')} "${restoreCmd}"`], gz)
  step(`push 完成——云端备份在 ${remoteBak};生产 API 容器重启后会按迁移补齐版本`)
}

;(direction === 'pull' ? pull : push)().catch(e => { console.error('失败:', e.message); process.exit(1) })
