// scripts/check-csp.mjs — 校验 dist/index.html 的内联脚本 hash 与 118 nginx CSP 头一致。
// CSP 按 hash 白名单放行唯一的内联脚本;任何构建导致 hash 漂移而 nginx 未同步,页面会
// 静默白屏——此脚本在部署流程里把这类漂移变成显式失败。用法:
//   node scripts/check-csp.mjs [expected-hash]
// 不带参数时只打印当前 hash(供更新 nginx conf 用)。
import { readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'

const html = readFileSync(new URL('../frontend/dist/index.html', import.meta.url), 'utf8')
const inline = [...html.matchAll(/<script(?![^>]*src=)[^>]*>([\s\S]*?)<\/script>/g)].map(m => m[1])
if (inline.length !== 1) {
  console.error(`expected exactly one inline script in dist/index.html, found ${inline.length}`)
  process.exit(1)
}
const hash = 'sha256-' + createHash('sha256').update(inline[0]).digest('hex')
const expected = process.argv[2]
if (!expected) {
  console.log(hash)
} else if (hash !== expected) {
  console.error(`CSP hash drift: dist/index.html now hashes to ${hash}, nginx allows ${expected}`)
  console.error('update the Content-Security-Policy header in /etc/nginx/sites-available/qingmulife.com.conf (118)')
  process.exit(1)
} else {
  console.log('csp hash in step:', hash)
}
