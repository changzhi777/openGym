// scripts/dev-api-switcher.mjs — fork-only 开发插件:双后端动态切换(W1,2026-11-25)。
//
// 场景:本地 kiri 栈(Go API :3100)与云端生产(https://app.qingmulife.com)之间切换,
// 同一个 vite dev 前端(:5173)。为什么不在前端覆盖 API base:跨源 fetch 带不走
// SameSite=Lax 的会话 cookie(5173→3100 是跨源),登录态直接死。正解是**同源反代**:
// vite dev 接管 /api 转发,每请求读目标文件,运行中切换即时生效,cookie 仍是 5173
// 自己的——换后端后旧 cookie 因对方 SECRET 不同而 401,重新登录即完成切换。
//
// 用法:
//   - 开关文件 frontend/.dev-api-target 存在 → 插件接管 /api(自带 proxy 被移除);
//     不存在 → vite 原生 proxy 照旧(上游行为零改动)。
//   - POST /__dev-api-target {"target":"..."} 写开关文件;GET 返回当前值。
//   - target 白名单:本地 3100 / 云端生产 / 任意 http(s) URL(校验协议)。
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { resolve, fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import http from 'node:http'
import https from 'node:https'

const here = dirname(fileURLToPath(import.meta.url))
const targetFile = join(here, '..', '.dev-api-target')
const DEFAULT_TARGET = 'http://127.0.0.1:3100'

export const readApiTarget = () => {
  try {
    const v = readFileSync(targetFile, 'utf8').trim()
    return v || DEFAULT_TARGET
  } catch { return DEFAULT_TARGET }
}

// CSRF 闸:API 校验 Origin(见 vite.config.js 的 apiOrigin 注释)。转发时统一改写,
// 与上游 proxy 配置 headers:{ Origin } 同语义。
const apiOriginFor = target => {
  try { return new URL(target).origin } catch { return 'http://localhost:8080' }
}

// 每请求转发:Node 原生管道(零新依赖),http/https 按 target 协议选模块。
function forward(req, res, target) {
  const u = new URL(target + req.url)
  const lib = u.protocol === 'https:' ? https : http
  const headers = { ...req.headers }
  headers.host = u.host
  headers.origin = apiOriginFor(target)
  const upstream = lib.request({
    protocol: u.protocol, hostname: u.hostname, port: u.port || (u.protocol === 'https:' ? 443 : 80),
    path: req.url, method: req.method, headers,
  }, up => {
    res.writeHead(up.statusCode, up.headers)
    up.pipe(res)
  })
  upstream.on('error', err => {
    res.statusCode = 502
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify({ error: `dev-api-switcher: cannot reach ${target} (${err.code || err.message})` }))
  })
  req.pipe(upstream)
}

export default function devApiSwitcher() {
  return {
    name: 'muhe-dev-api-switcher',
    apply: 'serve',
    // 开关文件存在时:移除上游 /api proxy(转发由本插件接管)。
    config(userConfig) {
      if (!existsSync(targetFile)) return
      const proxy = userConfig.server?.proxy
      if (proxy && proxy['/api']) delete proxy['/api']
    },
    configureServer(server) {
      if (!existsSync(targetFile)) return
      // 切换端点(仅 dev server 存在,构建产物无此路由)。
      server.middlewares.use('/__dev-api-target', (req, res) => {
        if (req.method === 'GET') {
          res.setHeader('Content-Type', 'application/json')
          return res.end(JSON.stringify({ target: readApiTarget() }))
        }
        if (req.method === 'POST') {
          let body = ''
          req.on('data', c => { body += c })
          req.on('end', () => {
            try {
              const t = String(JSON.parse(body || '{}').target || '').replace(/\/+$/, '')
              if (!/^https?:\/\//.test(t)) throw new Error('target must be an http(s) URL')
              writeFileSync(targetFile, t + '\n')
              res.setHeader('Content-Type', 'application/json')
              res.end(JSON.stringify({ ok: true, target: t }))
            } catch (e) {
              res.statusCode = 400
              res.end(JSON.stringify({ error: e.message }))
            }
          })
          return
        }
        res.statusCode = 405
        res.end()
      })
      // /api 动态转发:每请求读目标文件(运行中切换,无需重启 dev server)。
      server.middlewares.use((req, res, next) => {
        if (!req.url.startsWith('/api')) return next()
        forward(req, res, readApiTarget())
      })
    },
  }
}
