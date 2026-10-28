// views/admin/Invites.jsx — 邀请码(批4;设计 §3.11;方向2 全链路页面层)
//
// 数据:GET /api/admin/invites(契约 adminInvites,Go 已实现);创建/撤销走 apiAdmin
// (step-up 403 自动重放);邀请制开关 PUT /api/admin/settings(ops+step-up)。
// 矩阵:邀请码管理 support 隐藏 → 页内 403 面板(§2.5)。
import { useEffect, useMemo, useState } from 'react'
import { Link, useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { adminT } from '../../lib/admin/i18n.js'
import { MOBILE } from '../../lib/mobile.js'

const FILTERS = [{ k: '', label: '全部' }, { k: 'open', label: '可用' }, { k: 'used', label: '已使用' }]

function Denied({ role }) {
  return (
    <div className="adm-console adm-403">
      <h2>{adminT('需要更高权限')}</h2>
      <p>{adminT('此页面仅超级管理员可见')}{role ? ` — ${adminT('你的角色是')}「${role}」` : ''}</p>
      <div className="adm-actions"><Link className="adm-plain" to="/admin">{adminT('返回概览')}</Link></div>
    </div>
  )
}

function CreateSheet({ close, onDone }) {
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const toast = useUI(s => s.toast)
  const go = async () => {
    setBusy(true)
    try {
      const r = await apiAdmin('/api/admin/invites/new', { method: 'POST', body: JSON.stringify({ note }) })
      navigator.clipboard?.writeText(r.invite.code).catch(() => {})
      toast(adminT('邀请码已创建并复制'))
      close(); onDone()
    } catch (e) { toast(e.message); setBusy(false) }
  }
  return <>
    <h3>{adminT('创建邀请码')}</h3>
    <input className="adm-search" style={{ width: '100%' }} value={note} maxLength={60}
      placeholder={adminT('输入备注（可选）')} onChange={e => setNote(e.target.value)}
      aria-label={adminT('备注')} />
    <div style={{ height: 12 }} />
    <button className="adm-danger" disabled={busy} onClick={go}>{adminT('创建邀请码')}</button>
  </>
}

export default function Invites() {
  const { role, readonly } = useOutletContext()
  const toast = useUI(s => s.toast)
  const openSheet = useUI(s => s.openSheet)
  const [d, setD] = useState(null)
  const [err, setErr] = useState(null)
  const [f, setF] = useState('')

  const load = () => {
    setErr(null)
    api('/api/admin/invites').then(setD).catch(e => setErr(e.message))
  }
  useEffect(load, [])

  if (role === 'support') return <Denied role={role} />

  const invites = d?.invites || []
  const filtered = useMemo(() => invites.filter(i =>
    f === '' || (f === 'open' ? !i.usedBy && !i.revoked : !!i.usedBy)), [invites, f])
  const openN = invites.filter(i => !i.usedBy && !i.revoked).length

  const toggle = () => apiAdmin('/api/admin/settings', { method: 'PUT', body: JSON.stringify({ invite_only: !d.invite_only }) })
    .then(load).catch(e => toast(e.message))

  const create = () => openSheet(close => <CreateSheet close={close} onDone={load} />)

  const revoke = i => confirmSheet({
    title: i.code,
    message: adminT('仅未使用的码可撤销'),
    confirmText: adminT('撤销'),
    danger: true,
    onConfirm: () => apiAdmin('/api/admin/invites/revoke', { method: 'POST', body: JSON.stringify({ code: i.code }) })
      .then(load).catch(e => toast(e.message)),
  })

  const copy = i => { navigator.clipboard?.writeText(i.code).catch(() => {}); toast(adminT('已复制')) }
  const stateOf = i => i.usedBy ? `${adminT('已使用')} → ${i.usedByName || i.usedBy}` : i.revoked ? adminT('已撤销') : adminT('可用')

  return (
    <div className="adm-console">
      <div className="adm-toolbar">
        <strong>{adminT('邀请码')}</strong>
        <button className="adm-plain" onClick={toggle} disabled={readonly}>
          {adminT('邀请制')} · {d?.invite_only ? adminT('已开启') : adminT('已关闭')}
        </button>
        <span className="adm-count">{adminT('可用')} {openN} · {adminT('已使用')} {invites.length - openN}</span>
        <button className="adm-plain" onClick={create}>{adminT('创建邀请码')}</button>
      </div>
      {err && <div className="adm-attention" role="alert">{err}</div>}
      <div className="adm-toolbar" role="tablist">
        {FILTERS.map(x => (
          <button key={x.k} role="tab" aria-selected={f === x.k}
            className={'adm-plain adm-chip' + (f === x.k ? ' on' : '')}
            onClick={() => setF(x.k)}>{adminT(x.label)}</button>
        ))}
      </div>

      {filtered.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
      {MOBILE ? (
        <div className="adm-card">
          {filtered.map(i => (
            <div key={i.code} className="adm-tr" style={{ display: 'grid', gap: 4, padding: '10px 0', borderBottom: '1px solid var(--hair)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <b style={{ fontFamily: 'monospace' }}>{i.code}</b>
                <span className="adm-usub">{stateOf(i)}</span>
              </div>
              <div className="adm-usub">{i.note || '—'} · {adminT('创建人')} {i.createdBy}</div>
              {!i.usedBy && !i.revoked && (
                <span className="adm-rowbtns">
                  <button className="adm-plain" onClick={() => copy(i)}>{adminT('复制')}</button>
                  <button className="adm-danger-ghost" onClick={() => revoke(i)}>{adminT('撤销')}</button>
                </span>
              )}
            </div>
          ))}
        </div>
      ) : (
        <div className="adm-card">
          <table className="adm-table">
            <thead><tr>
              <th>{adminT('邀请码')}</th><th>{adminT('备注')}</th><th>{adminT('创建人')}</th>
              <th>{adminT('状态')}</th><th style={{ textAlign: 'right' }}>{adminT('操作')}</th>
            </tr></thead>
            <tbody>
              {filtered.map(i => (
                <tr key={i.code} className="adm-tr">
                  <td style={{ fontFamily: 'monospace' }}>{i.code}</td>
                  <td>{i.note || '—'}</td>
                  <td>{i.createdBy}</td>
                  <td>{stateOf(i)}</td>
                  <td style={{ textAlign: 'right' }}>
                    {!i.usedBy && !i.revoked ? (
                      <span className="adm-rowbtns">
                        <button className="adm-plain" onClick={() => copy(i)}>{adminT('复制')}</button>
                        <button className="adm-danger-ghost" onClick={() => revoke(i)}>{adminT('撤销')}</button>
                      </span>
                    ) : <span className="adm-usub">—</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
