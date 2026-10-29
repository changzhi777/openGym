// views/admin/Roles.jsx — 角色与权限(fork,批1)
//
// 设计依据 docs/admin-ui-design.md §3.10/3.10.1(桌面三卡+成员表/移动折叠卡)、
// §3.12 授予弹窗、§2.5(角色不足渲染页面内 403 面板,不重定向)。
// 权限矩阵:角色管理仅 super_admin;授予/撤销走 apiAdmin(step-up 403 自动弹验证重放)。
// super_admin 本身不在授予 UI 内 —— 矩阵:仅服务器配置可调整。
// 「最近操作」列(蓝图)暂缺数据源(需按操作者查审计),留待批 2 审计深化后补。
import { useEffect, useState } from 'react'
import { Link, useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { MOBILE } from '../../lib/mobile.js'

const ROLE_KEY = { super_admin: '超级管理员', ops: '运营', support: '客服(只读)' }
const GRANTABLE = ['ops', 'support']

const ROLE_CARDS = [
  { role: 'super_admin', lines: ['全部权限', '删号 · 角色 · 清日志', '仅服务器配置可调整'] },
  { role: 'ops', lines: ['用户 / 配额 / 停用 / 邀请', '敏感操作需二次验证', '不能删号 / 管角色'] },
  { role: 'support', lines: ['只读查看', '不能修改任何数据', '不可见营收与 IP'] },
]

// §2.5:403 面板 —— 操作者需要知道"为什么进不去"。
function Denied({ role }) {
  return (
    <div className="adm-console adm-403">
      <h2>{adminT('需要更高权限')}</h2>
      <p>{adminT('此页面仅超级管理员可见')}{role ? ` — ${adminT('你的角色是')}「${adminT(ROLE_KEY[role] || role)}」` : ''}</p>
      <div className="adm-actions"><Link className="adm-plain" to="/admin">{adminT('返回概览')}</Link></div>
    </div>
  )
}

// §3.12 授予弹窗:搜索选用户 → 选角色(仅 ops/support) → 授予。
function GrantPanel({ onClose, onDone }) {
  const [q, setQ] = useState('')
  const [users, setUsers] = useState(null)
  const [picked, setPicked] = useState(null)
  const [pickRole, setPickRole] = useState(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState(null)
  useEffect(() => {
    // 用户列表搜索属批 2 重写范围;此处全量拉取后客户端过滤(本期量级足够)。
    api('/api/admin/users').then(r => setUsers(r.users)).catch(() => setUsers([]))
  }, [])
  const s = q.trim().toLowerCase()
  const filtered = (users || []).filter(u =>
    !s || u.name.toLowerCase().includes(s) || String(u.email || '').toLowerCase().includes(s)
      || String(u.id || '').toLowerCase().includes(s))
  const submit = async () => {
    if (!picked || !pickRole) return
    setBusy(true); setErr(null)
    try {
      await apiAdmin('/api/admin/roles/grant', { method: 'POST', body: JSON.stringify({ id: picked, role: pickRole }) })
      onDone()
    } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <div className="adm-overlay" role="dialog" aria-modal onClick={onClose}>
      <div className="adm-modal" onClick={e => e.stopPropagation()}>
        <h3>{adminT('授予管理员')}</h3>
        <div className="adm-usub">{adminT('选择用户')}</div>
        <div className="adm-search"><input value={q} onChange={e => setQ(e.target.value)} placeholder={adminT('搜索姓名或邮箱')} /></div>
        <div className="adm-picklist">
          {users === null && <button type="button" disabled>{adminT('加载中…')}</button>}
          {users !== null && filtered.length === 0 && <button type="button" disabled>{adminT('暂无数据')}</button>}
          {filtered.map(u => (
            <button key={u.id} type="button" className={picked === u.id ? 'on' : ''} onClick={() => setPicked(u.id)}>
              <span>{u.name}</span><span className="sub">{u.email || u.id}</span>
            </button>
          ))}
        </div>
        <div className="adm-usub">{adminT('选择角色')}</div>
        <div className="adm-grid-2">
          {GRANTABLE.map(r => (
            <button key={r} type="button" className={'adm-rolepick' + (pickRole === r ? ' on' : '')} onClick={() => setPickRole(r)}>
              <span>{adminT(ROLE_KEY[r])}</span>
              <small>{r === 'ops' ? adminT('敏感操作需二次验证') : adminT('不能修改任何数据')}</small>
            </button>
          ))}
        </div>
        <div className="adm-usub">{adminT('授予后立即生效，并记录到审计日志')}</div>
        {err && <div className="adm-attention" role="alert">{err}</div>}
        <div className="adm-actions">
          <button className="adm-plain" onClick={onClose}>{adminT('取消')}</button>
          <button className="adm-danger" disabled={!picked || !pickRole || busy} onClick={submit}>{adminT('授予角色')}</button>
        </div>
      </div>
    </div>
  )
}

export default function Roles() {
  const { role } = useOutletContext()
  const [rows, setRows] = useState(null)
  const [err, setErr] = useState(null)
  const [granting, setGranting] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = () => {
    if (role !== 'super_admin') return // 矩阵:角色管理仅 super;非 super 不发请求(渲染 403 面板)
    setErr(null)
    api('/api/admin/roles').then(r => setRows(r.roles)).catch(e => setErr(e.message))
  }
  useEffect(load, [role])

  if (role !== 'super_admin') return <Denied role={role} />

  const counts = { super_admin: 0, ops: 0, support: 0 }
  for (const r of rows || []) counts[r.role] = (counts[r.role] || 0) + 1
  const total = (rows || []).length

  const withBusy = fn => async () => {
    setBusy(true)
    try { await fn(); load() } catch (e) { setErr(e.message) } finally { setBusy(false) }
  }
  // 变更角色 = 授予新角色 + 撤销旧角色(两调用;同一 step-up 窗口内只弹一次验证)。
  const changeRole = r => withBusy(async () => {
    const to = r.role === 'ops' ? 'support' : 'ops'
    await apiAdmin('/api/admin/roles/grant', { method: 'POST', body: JSON.stringify({ id: r.user_id, role: to }) })
    await apiAdmin('/api/admin/roles/revoke', { method: 'POST', body: JSON.stringify({ id: r.user_id, role: r.role }) })
  })()
  const revokeAdmin = r => withBusy(async () => {
    await apiAdmin('/api/admin/roles/revoke', { method: 'POST', body: JSON.stringify({ id: r.user_id, role: r.role }) })
  })()

  const rowBtns = r => {
    if (r.role === 'super_admin') return <span className="adm-usub">—</span>
    const other = r.role === 'ops' ? 'support' : 'ops'
    return (
      <span className="adm-rowbtns">
        <button className="adm-plain" disabled={busy} onClick={() => changeRole(r)}>{adminT('变更角色为')}{adminT(ROLE_KEY[other])}</button>
        <button className="adm-danger-ghost" disabled={busy} onClick={() => revokeAdmin(r)}>{adminT('撤销管理员')}</button>
      </span>
    )
  }

  return (
    <div className="adm-console">
      <div className="adm-toolbar">
        <strong>{adminT('角色与权限')}</strong>
        <span className="adm-count">{adminT('成员')} {total} · {adminT('超级管理员')} {counts.super_admin} · {adminT('运营')} {counts.ops} · {adminT('客服(只读)')} {counts.support}</span>
        <button className="adm-plain" onClick={() => setGranting(true)}>{adminT('授予管理员')}</button>
      </div>
      {err && <div className="adm-attention" role="alert">{err}</div>}
      <div className="adm-role-cards">
        {ROLE_CARDS.map(c => (
          <div key={c.role} className="adm-role-card">
            <h4>{adminT(ROLE_KEY[c.role])}</h4>
            <ul>{c.lines.map(l => <li key={l}>{adminT(l)}</li>)}</ul>
          </div>
        ))}
      </div>
      {rows !== null && rows.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
      {rows !== null && rows.length > 0 && (MOBILE ? (
        <div className="adm-card">
          {rows.map(r => (
            <div key={r.user_id + ':' + r.role} className="adm-tr" style={{ display: 'grid', gap: 4, padding: '10px 0', borderBottom: '1px solid var(--hair)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <strong>{r.name}</strong>
                <span className="adm-badge">{adminT(ROLE_KEY[r.role])}</span>
              </div>
              <div className="adm-usub">
                {r.granted_by ? `${adminT('授予人')} ${r.granted_by_name || r.granted_by} · ` : ''}
                {adminT('授予时间')} {adminWhen(r.granted_at)}
              </div>
              {rowBtns(r)}
            </div>
          ))}
        </div>
      ) : (
        <div className="adm-card">
          <table className="adm-table">
            <thead>
              <tr>
                <th>{adminT('用户')}</th>
                <th>{adminT('角色与权限')}</th>
                <th>{adminT('授予人')}</th>
                <th>{adminT('授予时间')}</th>
                <th style={{ textAlign: 'right' }}>{adminT('操作')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(r => (
                <tr key={r.user_id + ':' + r.role} className="adm-tr">
                  <td><div className="adm-ucell"><span>{r.name}</span></div></td>
                  <td><span className="adm-badge">{adminT(ROLE_KEY[r.role])}</span></td>
                  <td>{r.granted_by ? (r.granted_by_name || r.granted_by) : '–'}</td>
                  <td>{adminWhen(r.granted_at)}</td>
                  <td style={{ textAlign: 'right' }}>{rowBtns(r)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="adm-role-note">{adminT('超级管理员只能通过服务器配置调整')}</div>
        </div>
      ))}
      {granting && <GrantPanel onClose={() => setGranting(false)} onDone={load} />}
    </div>
  )
}
