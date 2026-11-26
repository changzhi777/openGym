// LoginServersMenu.jsx — 登录卡顶部的服务器选择行(fork-only,中文直写)。
// 选服务器=整页跳转到目标 origin 同路径(见 lib/login-servers.js 头注释);
// linkedom 兼容:无 location 环境渲染 null,跳转走原生 location.replace。
import { useState } from 'react'
import {
  addServer, isCurrent, isDefaultServer, jumpTarget,
  loadServers, normUrl, removeServer, saveServers,
} from '../lib/login-servers.js'

const C = { hair: 'rgba(255,255,255,.10)', sub: 'rgba(235,245,238,.55)', green: '#6fce85', ink: '#f2f2f7' }

const hasLocation = () => typeof location !== 'undefined'

export default function LoginServersMenu() {
  const [open, setOpen] = useState(false)
  const [custom, setCustom] = useState(() => loadServers(safeStore()).filter(s => !isDefaultServer(s.url)))
  const [raw, setRaw] = useState('')
  const [err, setErr] = useState('')

  if (!hasLocation()) return null
  const cur = location.origin
  const servers = loadServers(safeStore())

  const pick = url => {
    if (!hasLocation()) return
    const t = jumpTarget(url, location.href)
    if (t) { try { localStorage.setItem('muhe_last_login_server', url) } catch { /* 无存储跳过 */ } }
    if (t) location.replace(t)
  }
  const onAdd = () => {
    const { servers: all, url } = addServer(servers, raw)
    if (!url) { setErr('地址无效——需要 http(s):// 主机[:端口]'); return }
    setErr('')
    saveServers(safeStore(), all.filter(s => !isDefaultServer(s.url)))
    setCustom(all.filter(s => !isDefaultServer(s.url)))
    setRaw('')
    if (!isCurrent(url, cur)) pick(url)
  }
  const onDel = url => {
    const rest = removeServer(servers, url)
    saveServers(safeStore(), rest.filter(s => !isDefaultServer(s.url)))
    setCustom(rest.filter(s => !isDefaultServer(s.url)))
  }

  const current = servers.find(s => isCurrent(s.url, cur))
  const row = { display: 'flex', alignItems: 'center', gap: 8, width: '100%', textAlign: 'start',
    background: 'transparent', border: 'none', color: C.sub, fontSize: 12.5, cursor: 'pointer', padding: 0 }

  return (
    <div style={{ marginBottom: 14, borderBottom: `1px solid ${C.hair}`, paddingBottom: 12 }}>
      <button type="button" onClick={() => setOpen(o => !o)} style={row}>
        <span aria-hidden>🖥️</span>
        <span>API 服务器：<b style={{ color: C.ink, fontWeight: 600 }}>{current ? current.name : normUrl(cur)}</b></span>
        <span style={{ marginLeft: 'auto', color: C.green }}>{open ? '收起 ▴' : '切换 ▾'}</span>
      </button>
      {open && (
        <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 6 }}>
          {servers.map(s => {
            const here = isCurrent(s.url, cur)
            return (
              <div key={s.url} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <button type="button" disabled={here}
                  onClick={() => pick(s.url)}
                  style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 8, textAlign: 'start',
                    background: here ? 'rgba(47,156,71,.12)' : 'rgba(255,255,255,.04)',
                    border: `1px solid ${here ? 'rgba(47,156,71,.45)' : C.hair}`, borderRadius: 10,
                    padding: '8px 10px', color: C.ink, fontSize: 13,
                    cursor: here ? 'default' : 'pointer', opacity: here ? .85 : 1 }}>
                  <span style={{ width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
                    background: here ? '#2ea567' : 'transparent', border: `1.5px solid ${here ? '#2ea567' : C.hair}` }} />
                  <span style={{ flex: 1, minWidth: 0 }}>
                    <span style={{ display: 'block' }}>{s.name}{here ? '（已在）' : ''}</span>
                    <span style={{ display: 'block', fontSize: 11, color: C.sub, overflow: 'hidden',
                      textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.url}</span>
                  </span>
                </button>
                {!isDefaultServer(s.url) && !here && (
                  <button type="button" aria-label={`删除 ${s.url}`} onClick={() => onDel(s.url)}
                    style={{ background: 'none', border: 'none', color: '#e05252', fontSize: 15, cursor: 'pointer', padding: 4 }}>✕</button>
                )}
              </div>
            )
          })}
          <div style={{ display: 'flex', gap: 6, marginTop: 4 }}>
            <input value={raw} onChange={e => setRaw(e.target.value)} inputMode="url"
              placeholder="自定义地址，如 http://192.168.1.50:3131"
              style={{ flex: 1, minWidth: 0, background: 'rgba(255,255,255,.04)', color: C.ink,
                border: `1px solid ${C.hair}`, borderRadius: 10, padding: '8px 10px', fontSize: 12.5, outline: 'none' }} />
            <button type="button" onClick={onAdd}
              style={{ background: 'rgba(47,156,71,.14)', color: C.green, border: '1px solid rgba(47,156,71,.45)',
                borderRadius: 10, padding: '0 12px', fontSize: 12.5, cursor: 'pointer', flexShrink: 0 }}>添加</button>
          </div>
          {err && <div style={{ color: '#e05252', fontSize: 12 }}>{err}</div>}
        </div>
      )}
    </div>
  )
}

// linkedom/无 localStorage 环境的安全 store
function safeStore() {
  try { return typeof localStorage !== 'undefined' ? localStorage : null } catch { return null }
}
