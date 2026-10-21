// views/admin/UserDetail.jsx — 用户详情与账户操作(fork,P1)
//
// 设计 §3.3:脱敏页头 + 统计 + 配额卡(待 Go 后端) + 账户操作。
// 操作经 apiAdmin(stepUp 包装):Node 后端现阶段不返回 step-up 403 直接执行;
// Go 后端强制后同一代码自动弹验证并重放(lib/admin/stepUp.js)。
// 删号按设计 §5.5:输入账户名完全匹配才解锁 + 影响清单计数。
import { useEffect, useState } from 'react'
import { useNavigate, useParams, useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { fmtDate } from '../../lib/format.js'
import { confirmSheet } from '../../sheets.jsx'
import { useUI } from '../../store/useUI.js'

const whenOf = v => (typeof v === 'number' ? v : v ? Date.parse(v) : 0)

// 一次性展示重置码(设计:唯一一次可见,locked 防误触丢失)
function ResetCodeSheet({ name, code, expires, close }) {
  const toast = useUI(s => s.toast)
  const copy = () => { navigator.clipboard?.writeText(code).catch(() => {}); toast(adminT('已复制')) }
  return <>
    <h3>{name}</h3>
    <button className="adm-code" style={{ fontSize: 22, width: '100%', padding: '14px 0' }} onClick={copy} aria-label={adminT('重置码')}>{code}</button>
    <div className="adm-usub" style={{ margin: '8px 0 14px' }}>{new Date(expires).toLocaleString()}</div>
    <button className="btn primary" style={{ width: '100%' }} onClick={close}>OK</button>
  </>
}

// 删号(设计 §5.5):影响清单计数 + 输入账户名完全匹配解锁 + 实心红底按钮
function DeleteSheet({ user, impact, close, onDone }) {
  const [typed, setTyped] = useState('')
  const toast = useUI(s => s.toast)
  const unlocked = typed.trim() === user.name && typed.length > 0
  const go = async () => {
    try {
      await apiAdmin('/api/admin/user/delete', { method: 'POST', body: JSON.stringify({ id: user.id }) })
      toast(user.name)
      close(); onDone()
    } catch (e) { toast(e.message) }
  }
  return <>
    <h3>{user.name}</h3>
    <div className="muted small" style={{ marginBottom: 10 }}>{adminT('删除后不可恢复')}</div>
    {impact.map(([k, n]) => <div key={k} className="adm-rowline"><span className="who">{k}</span><span>{n}</span></div>)}
    <div style={{ height: 10 }} />
    <input className="adm-search" style={{ width: '100%' }} placeholder={adminT('输入账户名以确认') + ': ' + user.name}
      value={typed} onChange={e => setTyped(e.target.value)} aria-label={adminT('删除用户')} />
    <div style={{ height: 12 }} />
    <button className="adm-danger" disabled={!unlocked} onClick={go}>{adminT('删除用户')}</button>
  </>
}

export default function UserDetail() {
  const { id } = useParams()
  const nav = useNavigate()
  const { readonly } = useOutletContext()
  const toast = useUI(s => s.toast)
  const openSheet = useUI(s => s.openSheet)
  const [d, setD] = useState(null)
  const [err, setErr] = useState(null)

  const load = () => { setErr(null); api('/api/admin/user?id=' + encodeURIComponent(id)).then(setD).catch(e => setErr(e.message)) }
  useEffect(load, [id])
  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>
  if (!d) return <div className="adm-empty">{adminT('加载中…')}</div>

  const u = d.user
  const workouts = Array.isArray(d.workouts) ? d.workouts : []
  const weighIns = Array.isArray(d.bodyweight) ? d.bodyweight : []
  const impact = [[adminT('训练记录'), workouts.length], [adminT('体重记录'), weighIns.length]]

  const toggleDisable = () => confirmSheet({
    title: u.name,
    message: u.disabled ? adminT('启用后该用户可重新登录') : adminT('停用后该用户无法登录'),
    confirmText: u.disabled ? adminT('启用') : adminT('停用'),
    danger: !u.disabled,
    onConfirm: () => apiAdmin('/api/admin/user/disable', { method: 'POST', body: JSON.stringify({ id: u.id, disabled: !u.disabled }) })
      .then(load).catch(e => toast(e.message)),
  })

  const resetCode = () => confirmSheet({
    title: u.name,
    message: adminT('重置码'),
    confirmText: adminT('创建重置码'),
    onConfirm: () => apiAdmin('/api/admin/user/password-reset', { method: 'POST', body: JSON.stringify({ id: u.id }) })
      .then(r => openSheet(done => <ResetCodeSheet name={r.name} code={r.code} expires={r.expires} close={done} />, { locked: true }))
      .catch(e => toast(e.message)),
  })

  const del = () => openSheet(close => <DeleteSheet user={u} impact={impact} close={close} onDone={() => nav('/admin/users')} />)

  return (
    <>
      <div className="adm-toolbar">
        <button className="adm-plain" onClick={() => nav('/admin/users')}>‹ {adminT('用户')}</button>
      </div>
      <div className="adm-kpis">
        <div className="adm-kpi"><div className="k">{u.name}</div>
          <div className="v" style={{ fontSize: 17 }}>{u.created ? fmtDate(u.created) : '–'}</div>
          <div className="sub">{u.disabled ? adminT('停用') : ''} {u.admin ? '· ' + adminT('超级管理员') : ''}</div>
        </div>
        <div className="adm-kpi"><div className="k">{adminT('训练记录')}</div><div className="v">{workouts.length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('体重记录')}</div><div className="v">{weighIns.length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('最近同步')}</div>
          <div className="v" style={{ fontSize: 17 }}>{d.lastSync ? adminWhen(whenOf(d.lastSync)) : '–'}</div></div>
      </div>

      {!readonly && (
        <div className="adm-card"><h3>{adminT('刷新')}</h3>
          <div className="adm-rowline">
            <span>{u.disabled ? adminT('停用') : adminT('暂无数据')}</span>
            <span className="adm-actions">
              <button className="adm-plain" onClick={toggleDisable}>{u.disabled ? adminT('启用') : adminT('停用')}</button>
              <button className="adm-plain" onClick={resetCode}>{adminT('重置码')}</button>
              <button className="adm-danger-ghost" onClick={del}>{adminT('删除用户')}</button>
            </span>
          </div>
        </div>
      )}
    </>
  )
}
