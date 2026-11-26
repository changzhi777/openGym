// login-servers.js — 登录页服务器选择菜单的逻辑(fork-only,中文直写)。
// 模式与壳 launcher 一致:选服务器=整页跳转到目标 origin 同路径(location.replace),
// cookie 按 origin 天然隔离;不做跨源 API 请求(SameSite/CORS/CSRF 三座山)。
// location 相关一律显式传参,纯函数离线可测(linkedom 教训)。

export const DEFAULT_SERVERS = [
  { name: '183 测试版（局域网）', url: 'http://10.10.10.7:3131' },
  { name: '183 测试版（Tailscale）', url: 'http://100.98.98.183:3131' },
  { name: '生产正式版', url: 'https://app.qingmulife.com' },
]

const STORE_KEY = 'muhe_login_servers'

// normalize:补协议、取 origin+去尾斜杠 path;无效返回 ''
export function normUrl(raw) {
  let u = (raw || '').trim()
  if (!u) return ''
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(u) && !/^https?:\/\//i.test(u)) return ''   // ftp:// 等非 http(s) 直接拒
  if (!/^https?:\/\//i.test(u)) u = 'http://' + u
  try {
    const p = new URL(u)
    if (!/^https?:$/.test(p.protocol) || !p.hostname) return ''
    const path = p.pathname === '/' ? '' : p.pathname.replace(/\/$/, '')
    return p.origin + path
  } catch {
    return ''
  }
}

// 读用户自加的服务器列表(损坏即回默认);预置三条恒在,自定义段排除预置重复并自身去重
export function loadServers(store) {
  let custom = []
  try {
    const v = JSON.parse((store && store.getItem(STORE_KEY)) || 'null')
    if (Array.isArray(v)) {
      const defUrls = new Set(DEFAULT_SERVERS.map(s => s.url))
      const seen = new Set()
      custom = v
        .filter(s => s && typeof s.url === 'string')
        .map(s => ({ name: s.name || normUrl(s.url), url: normUrl(s.url) }))
        .filter(s => s.url && !defUrls.has(s.url) && !seen.has(s.url) && seen.add(s.url))
    }
  } catch { /* 损坏即忽略 */ }
  return [...DEFAULT_SERVERS, ...custom]
}

export function saveServers(store, custom) {
  if (store) store.setItem(STORE_KEY, JSON.stringify(custom))
}

// 加一条自定义服务器(去重;已存在则原样返回),返回 {servers, url|null}
export function addServer(servers, raw) {
  const url = normUrl(raw)
  if (!url) return { servers, url: null }
  if (servers.some(s => s.url === url)) return { servers, url }
  const name = '自定义'
  return { servers: [...servers, { name, url }], url }
}

// 删一条(预置三条不可删)
export function removeServer(servers, url) {
  if (DEFAULT_SERVERS.some(s => s.url === url)) return servers
  return servers.filter(s => s.url !== url)
}


// 跳转目标:目标 origin + 当前路径与 hash(保 /team 等深链;根路径保留单个 /)
export function jumpTarget(targetUrl, currentHref) {
  const t = normUrl(targetUrl)
  if (!t) return ''
  try {
    const cur = new URL(currentHref)
    const path = cur.pathname === '/' ? '/' : cur.pathname.replace(/\/$/, '')
    return t + path + cur.search + cur.hash
  } catch {
    return t
  }
}

// 是当前 origin 吗(同一 origin 的服务器即"已在",不可再选)
export function isCurrent(targetUrl, currentOrigin) {
  return normUrl(targetUrl) === normUrl(currentOrigin)
}

// 预置识别(容忍尾斜杠差异,统一 normalize 后比对)
export const isDefaultServer = url => DEFAULT_SERVERS.some(s => normUrl(s.url) === normUrl(url))
