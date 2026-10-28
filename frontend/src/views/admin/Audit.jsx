// views/admin/Audit.jsx — 审计日志(fork,批2 深化)
//
// 设计 §3.8/3.9:真表格 + 按天分组 + q 搜索(300ms 防抖) + 类别 chips + 游标
// 「加载更多」(§5.7:新事件让页码漂移,不用页码) + 行展开详情(目标名/IP/原始 JSON)
// + 清空(super_admin:输入"清空"确认 + step-up,经 apiAdmin)。
// 数据:GET /api/admin/audit?limit&before&cat&q;q 与 targetName 为 Go 后端扩展,
// Node 忽略 q 答 unfiltered、行无 targetName —— 渲染皆容错。
import { Fragment, useEffect, useMemo, useState } from 'react'
import { useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { apiAdmin } from '../../lib/admin/stepUp.js'
import { adminT } from '../../lib/admin/i18n.js'
import { fmtWhen } from '../../lib/audit.js'
import { confirmSheet } from '../../sheets.jsx'
import { useUI } from '../../store/useUI.js'

const CATS = [
  { k: '', label: '全部' },
  { k: 'auth', label: '认证' },
  { k: 'admin', label: '管理' },
  { k: 'data', label: '数据' },
  { k: 'push', label: '推送' },
  { k: 'fail', label: '失败' },   // 后端特化 cat=fail
]

// 清空审计(§2.4:super_admin + 输入"清空" + step-up)
function ClearSheet({ close, onDone }) {
  const [typed, setTyped] = useState('')
  const toast = useUI(s => s.toast)
  const unlocked = typed.trim() === '清空'
  const go = async () => {
    try {
      await apiAdmin('/api/admin/audit/clear', { method: 'POST', body: '{}' })
      toast(adminT('已复制') && 'OK')
      close(); onDone()
    } catch (e) { toast(e.message) }
  }
  return <>
    <h3>{adminT('清空审计日志')}</h3>
    <div className="muted small" style={{ marginBottom: 10 }}>admin.audit.clear</div>
    <input className="adm-search" style={{ width: '100%' }} placeholder={adminT('输入"清空"以确认')}
      value={typed} onChange={e => setTyped(e.target.value)} aria-label={adminT('清空审计日志')} />
    <div style={{ height: 12 }} />
    <button className="adm-danger" disabled={!unlocked} onClick={go}>{adminT('清空审计日志')}</button>
  </>
}

export default function Audit() {
  const { role } = useOutletContext()
  const openSheet = useUI(s => s.openSheet)
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(null)
  const [retention, setRetention] = useState(null)
  const [cat, setCat] = useState('')
  const [qIn, setQIn] = useState('')
  const [q, setQ] = useState('')
  const [expanded, setExpanded] = useState(null)
  const [err, setErr] = useState(null)
  const [done, setDone] = useState(false)

  useEffect(() => {
    const t = setTimeout(() => setQ(qIn.trim()), 300)
    return () => clearTimeout(t)
  }, [qIn])

  const load = (reset, before) => {
    setErr(null)
    const p = new URLSearchParams({ limit: 50, ...(cat ? { cat } : {}), ...(before ? { before } : {}), ...(q ? { q } : {}) })
    api('/api/admin/audit?' + p).then(d => {
      setRows(prev => reset ? d.events : [...prev, ...d.events])
      setTotal(d.total)
      setRetention(d.retention)
      setDone(d.events.length < 50)
    }).catch(e => { setErr(e.message); if (reset) setRows([]) })
  }
  useEffect(() => { load(true) }, [cat, q])

  const byDay = useMemo(() => {
    const g = new Map()
    for (const e of rows) {
      const d = new Date(e.createdAt || e.ts).toDateString()
      if (!g.has(d)) g.set(d, [])
      g.get(d).push(e)
    }
    return [...g.entries()]
  }, [rows])

  return (
    <>
      <div className="adm-toolbar">
        <input className="adm-search" type="search" placeholder={adminT('搜索操作者、目标或事件')} value={qIn}
          onChange={e => setQIn(e.target.value)} aria-label={adminT('搜索操作者、目标或事件')} />
        {retention?.days ? <span className="adm-usub">{adminT('保留')} {retention.days} {adminT('天')}</span> : null}
        {role === 'super_admin' && (
          <button className="adm-danger-ghost" onClick={() =>
            openSheet(close => <ClearSheet close={close} onDone={() => load(true)} />)
          }>{adminT('清空审计日志')}</button>
        )}
      </div>
      <div className="adm-toolbar" role="tablist" aria-label={adminT('审计')}>
        {CATS.map(c => (
          <button key={c.k} role="tab" aria-selected={cat === c.k}
            className={'adm-plain adm-chip' + (cat === c.k ? ' on' : '')}
            onClick={() => setCat(c.k)}>{c.label}</button>
        ))}
        {total != null && <span className="adm-count">{total}</span>}
      </div>

      {err && <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={() => load(true)}>{adminT('重试')}</button></div>}
      {rows.length === 0 && !err && <div className="adm-empty">{adminT('暂无数据')}</div>}

      {byDay.map(([day, evs]) => (
        <div className="adm-table" key={day}>
          <div className="adm-tr adm-th adm-day" role="row"><span role="columnheader">{day}</span></div>
          {evs.map(e => {
            const open = expanded === e.id
            const target = e.targetName || e.tgt || e.targetId || '—'
            return (
              <Fragment key={e.id}>                <button className="adm-tr adm-audit" role="row"
                  onClick={() => setExpanded(open ? null : e.id)} aria-expanded={open}>
                  <span className="adm-ucell">
                    <b className={e.ok === false ? 'fail' : ''}>{e.action}</b>
                    <span className="adm-usub">
                      {e.admin || '—'} → {target}
                      {e.ip ? ' · ' + e.ip : ''}
                    </span>
                  </span>
                  <span className={'adm-usub adm-num' + (e.ok === false ? ' fail' : '')}>{fmtWhen(e.createdAt || e.ts)}</span>
                </button>
                {open && (
                  <div className="adm-card" style={{ margin: '0 0 8px', fontSize: 13 }}>
                    <div className="adm-rowline"><span className="adm-usub">{adminT('事件')}</span><span>{e.id} · {e.action}</span></div>
                    <div className="adm-rowline"><span className="adm-usub">{adminT('目标')}</span><span>{target}</span></div>
                    {'ip' in e && <div className="adm-rowline"><span className="adm-usub">{adminT('来源 IP')}</span><span>{e.ip || '—'}</span></div>}
                    {e.detail != null && (
                      <pre style={{ whiteSpace: 'pre-wrap', margin: '6px 0 0', fontSize: 12, color: 'var(--label-2)' }}>
                        {JSON.stringify(e.detail, null, 2)}
                      </pre>
                    )}
                  </div>
                )}
              </Fragment>
            )
          })}
        </div>
      ))}

      {!done && rows.length > 0 && (
        <div className="adm-pager">
          <button onClick={() => load(false, rows[rows.length - 1]?.id)}>{adminT('刷新')} +50</button>
        </div>
      )}
    </>
  )
}
