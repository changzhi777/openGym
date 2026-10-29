// views/admin/UserDetail.jsx — 用户详情与账户操作(fork,批2;设计 §3.6 四卡)
//
// 页头脱敏邮箱([显示完整] 走 passkey 验证,§2.4 step-up 能力) + 账户概览 + 配额卡
// (ops,进度条 ± 调节,留空=实例默认) + 明细锁卡(apiAdmin activity:Go 栈 step-up
// 403 自动弹验证重放;Node 501 → 待后端支持) + 账户操作(停用/下载/重置码/删除,
// 删除仅 super_admin)。操作经 apiAdmin(stepUp 包装)。
import { useEffect, useState } from 'react'
import { useNavigate, useParams, useOutletContext } from 'react-router-dom'
import { api, passkeyAssertion } from '../../lib/api.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { fmtDate } from '../../lib/format.js'
import { confirmSheet } from '../../sheets.jsx'
import { useUI } from '../../store/useUI.js'

const whenOf = v => (typeof v === 'number' ? v : v ? Date.parse(v) : 0)

// 邮箱脱敏:首字符 + ***(§3.4 z***@example.com;完整地址是 step-up 能力)。
const maskEmail = e => {
  if (!e) return ''
  const at = e.indexOf('@')
  return at <= 0 ? e : e.slice(0, 1) + '***' + e.slice(at)
}

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

// 配额卡(§3.6/§3.12):进度条 + [－][＋],留空 = 实例默认;独立保存,无"保存全部"。
function QuotaCard({ id, role, toast }) {
  const editable = role === 'super_admin' || role === 'ops'
  const [q, setQ] = useState(null)        // null=未加载;''=留空(实例默认);数字=自定义
  const [used, setUsed] = useState(null)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    api('/api/admin/users/' + encodeURIComponent(id) + '/quota').then(d => {
      setUsed(d.used_ai_calls_month ?? null)
      const v = d.ai_calls_month
      setQ(typeof v === 'number' ? String(v) : '')
    }).catch(() => setQ(''))
  }, [id])
  const save = async () => {
    setSaving(true)
    try {
      const body = {}
      if (q !== '') body.ai_calls_month = parseInt(q, 10) || 0
      await apiAdmin('/api/admin/users/' + encodeURIComponent(id) + '/quota', { method: 'PUT', body: JSON.stringify(body) })
      setDirty(false); toast(adminT('保存'))
    } catch (e) { toast(e.message) } finally { setSaving(false) }
  }
  const limit = q === '' ? null : parseInt(q, 10) || 0
  const pct = limit && used != null ? Math.min(100, Math.round(used / limit * 100)) : 0
  const near = limit && used != null && used / limit > 0.9
  return (
    <div className="adm-card">
      <h3>{adminT('配额')} — {adminT('AI 教练(本月)')}</h3>
      <div className="adm-rowline"><span>{adminT('本月已用')}</span><span className="adm-num">{used ?? '–'}{limit ? ' / ' + limit : ''}</span></div>
      {limit ? (
        <div style={{ height: 6, background: 'var(--hair)', borderRadius: 3, overflow: 'hidden', margin: '6px 0' }}>
          <div style={{ width: pct + '%', height: '100%', background: near ? 'var(--warn-text)' : 'var(--ok-text)' }} />
        </div>
      ) : null}
      {editable ? (
        <>
          <div className="adm-rowline">
            <span className="adm-actions">
              <button className="adm-plain" onClick={() => { setQ(v => String(Math.max(0, (parseInt(v, 10) || 0) - 5))); setDirty(true) }}>－</button>
              <input className="adm-search" style={{ width: 90, textAlign: 'center' }} value={q ?? ''}
                onChange={e => { setQ(e.target.value.replace(/[^\d]/g, '')); setDirty(true) }}
                placeholder={adminT('留空 = 实例默认')} aria-label={adminT('配额')} />
              <button className="adm-plain" onClick={() => { setQ(v => String((parseInt(v, 10) || 0) + 5)); setDirty(true) }}>＋</button>
            </span>
            <span className="adm-usub">{adminT('留空 = 实例默认')}</span>
          </div>
          {dirty && (
            <div className="adm-rowline" style={{ marginTop: 8 }}>
              <span className="adm-usub">{adminT('有未保存的更改')}</span>
              <span className="adm-actions">
                <button className="adm-plain" onClick={() => { setDirty(false); api('/api/admin/users/' + encodeURIComponent(id) + '/quota').then(d => setQ(typeof d.ai_calls_month === 'number' ? String(d.ai_calls_month) : '')).catch(() => {}) }}>{adminT('放弃')}</button>
                <button className="adm-danger" disabled={saving} onClick={save}>{adminT('保存')}</button>
              </span>
            </div>
          )}
        </>
      ) : null}
    </div>
  )
}

// 明细锁卡(§3.6):健康数据默认锁定;验证后展示,可重新锁定。
// Node 栈 activity 是 501 → apiAdmin 抛错显示"待后端支持"(与占位页同语义,不误调)。
function DetailCard({ id, name }) {
  const [unlocked, setUnlocked] = useState(false)
  const [data, setData] = useState(null)
  const [pending, setPending] = useState(false)
  const [err, setErr] = useState(null)
  const unlock = async () => {
    setPending(true); setErr(null)
    try {
      const d = await apiAdmin('/api/admin/users/' + encodeURIComponent(id) + '/activity')
      setData(d); setUnlocked(true)
    } catch (e) {
      setErr(e?.data?.code === 'not-implemented' ? adminT('待后端支持') : e.message)
    } finally { setPending(false) }
  }
  return (
    <div className="adm-card">
      <div className="adm-rowline">
        <h3 style={{ margin: 0 }}>{adminT('训练明细')}</h3>
        {unlocked && (
          <span className="adm-usub">
            {adminT('已验证')} · <button className="adm-plain" onClick={() => { setUnlocked(false); setData(null) }}>{adminT('重新锁定')}</button>
          </span>
        )}
      </div>
      {!unlocked ? (
        <>
          <div className="adm-usub" style={{ margin: '6px 0 10px' }}>{adminT('训练明细包含个人健康数据。验证后可查看。')}</div>
          {err && <div className="adm-usub" style={{ marginBottom: 8 }}>{err}</div>}
          <button className="adm-plain" disabled={pending} onClick={unlock}>{pending ? adminT('加载中…') : adminT('使用通行密钥查看')}</button>
        </>
      ) : (
        <div className="adm-table">
          {(data?.workouts || []).slice(0, 60).map(w => (
            <div className="adm-tr" key={w.id || w.date}>
              <span>{w.d || w.date}</span>
              <span className="adm-usub">{w.routine_id || ''} {w.note || ''}</span>
            </div>
          ))}
          {(data?.workouts || []).length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
        </div>
      )}
    </div>
  )
}

export default function UserDetail() {
  const { id } = useParams()
  const nav = useNavigate()
  const { role, readonly } = useOutletContext()
  const toast = useUI(s => s.toast)
  const openSheet = useUI(s => s.openSheet)
  const [d, setD] = useState(null)
  const [err, setErr] = useState(null)
  const [fullEmail, setFullEmail] = useState(false)

  const load = () => { setErr(null); api('/api/admin/user?id=' + encodeURIComponent(id)).then(setD).catch(e => setErr(e.message)) }
  useEffect(load, [id])
  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>
  if (!d) return <div className="adm-empty">{adminT('加载中…')}</div>

  const u = d.user
  const workouts = Array.isArray(d.workouts) ? d.workouts : []
  const weighIns = Array.isArray(d.bodyweight) ? d.bodyweight : []
  const routines = Array.isArray(d.routines) ? d.routines : []
  const impact = [[adminT('训练记录'), workouts.length], [adminT('体重记录'), weighIns.length]]
  const isSuper = role === 'super_admin'

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

  // 数据导出(§3.6 账户操作"下载数据";契约 adminUserExport,ops+step-up)。
  const download = () => apiAdmin('/api/admin/users/' + encodeURIComponent(id) + '/export').then(r => {
    const blob = new Blob([JSON.stringify(r.export ?? {}, null, 2)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'muhe-export-' + (u.name || id) + '.json'
    a.click()
    URL.revokeObjectURL(a.href)
  }).catch(e => toast(e.message))

  const del = () => openSheet(close => <DeleteSheet user={u} impact={impact} close={close} onDone={() => nav('/admin/users')} />)

  return (
    <>
      <div className="adm-toolbar">
        <button className="adm-plain" onClick={() => nav('/admin/users')}>‹ {adminT('用户')}</button>
      </div>

      {/* 页头(§3.6):不是大卡片,身份 + 脱敏邮箱 + 状态胶囊 */}
      <div className="adm-card">
        <div className="adm-rowline">
          <span className="adm-ucell">
            <b style={{ fontSize: 16 }}>{u.name}</b>
            <span className="adm-usub">
              {u.created ? adminT('加入时间') + ' ' + fmtDate(u.created.slice(0, 10)) : ''}{u.invitedBy ? ' · ' + adminT('邮箱') + ': ' + u.invitedBy : ''}
            </span>
          </span>
          <span className="adm-badges">
            <span className={'adm-badge' + (u.disabled ? ' readonly' : '')}>{u.disabled ? adminT('已停用') : adminT('正常')}</span>
            {u.admin && <span className="adm-badge">{adminT('超级管理员')}</span>}
          </span>
        </div>
        <div className="adm-rowline">
          <span className="adm-usub">{u.email || ''}</span>
          <span className="adm-usub">{adminT('最近同步')} {d.lastSync ? adminWhen(whenOf(d.lastSync)) : '–'}</span>
        </div>
      </div>

      {/* 账户概览(§3.6):计数卡;通行密钥/推送计数待契约扩展后补 */}
      <div className="adm-kpis">
        <div className="adm-kpi"><div className="k">{adminT('训练记录')}</div><div className="v">{workouts.length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('体重记录')}</div><div className="v">{weighIns.length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('角色与权限')}</div><div className="v" style={{ fontSize: 15 }}>{routines.length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('最近同步')}</div>
          <div className="v" style={{ fontSize: 17 }}>{d.lastSync ? adminWhen(whenOf(d.lastSync)) : '–'}</div></div>
      </div>

      {/* 配额卡(§3.6):support 不可见;ops 可改(step-up),super 全权 */}
      {role !== 'support' && <QuotaCard id={id} role={role} toast={toast} />}

      {/* 明细锁卡(§3.6):ops+step-up 解读;support 页内即锁 */}
      {role !== 'support' && <DetailCard id={id} name={u.name} />}

      {/* 账户操作(§3.6):每卡片独立保存;目标=管理员/自己时按设计不渲染对应按钮 */}
      {!readonly && (
        <div className="adm-card"><h3>{adminT('账户操作')}</h3>
          <div className="adm-rowline">
            <span>{u.disabled ? adminT('已停用') : adminT('正常')}</span>
            <span className="adm-actions">
              <button className="adm-plain" onClick={toggleDisable}>{u.disabled ? adminT('启用') : adminT('停用')}</button>
              <button className="adm-plain" onClick={download}>{adminT('下载数据')}</button>
              <button className="adm-plain" onClick={resetCode}>{adminT('重置码')}</button>
              {isSuper && <button className="adm-danger-ghost" onClick={del}>{adminT('删除用户')}</button>}
            </span>
          </div>
          <div className="adm-usub">{adminT('删除用户')} — {adminT('输入账户名以确认')} · step-up</div>
        </div>
      )}
    </>
  )
}
