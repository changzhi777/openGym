// views/admin/Audit.jsx — 审计日志(fork,P2)
//
// 设计 §3.4:真表格 + 按天分组;成功安静(label-2)、失败才红(--bad-text);
// 游标式「加载更多」(新事件会让页码漂移,不用页码 —— 设计 §5.7);筛选本地类别 chips。
// 数据:GET /api/admin/audit?limit&before&cat(现有 Node 端点),渲染复用 lib/audit.js。
import { useEffect, useMemo, useState } from 'react'
import { api } from '../../lib/api.js'
import { adminT } from '../../lib/admin/i18n.js'
import { auditCat, auditLine, fmtWhen } from '../../lib/audit.js'

const CATS = [
  { k: '', label: '全部' },
  { k: 'auth', label: '认证' },
  { k: 'admin', label: '管理' },
  { k: 'data', label: '数据' },
  { k: 'push', label: '推送' },
  { k: 'fail', label: '失败' },   // 后端特化 cat=fail
]

export default function Audit() {
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(null)
  const [cat, setCat] = useState('')
  const [err, setErr] = useState(null)
  const [done, setDone] = useState(false)

  const load = (reset, before) => {
    setErr(null)
    const q = new URLSearchParams({ limit: 50, ...(cat ? { cat } : {}), ...(before ? { before } : {}) })
    api('/api/admin/audit?' + q).then(d => {
      setRows(prev => reset ? d.events : [...prev, ...d.events])
      setTotal(d.total)
      setDone(d.events.length < 50)
    }).catch(e => { setErr(e.message); if (reset) setRows([]) })
  }
  useEffect(() => { load(true) }, [cat])

  const byDay = useMemo(() => {
    const g = new Map()
    for (const e of rows) {
      const d = new Date(e.ts).toDateString()
      if (!g.has(d)) g.set(d, [])
      g.get(d).push(e)
    }
    return [...g.entries()]
  }, [rows])

  return (
    <>
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
            const line = auditLine(e)
            return <div className="adm-tr adm-audit" role="row" key={e.id}>
              <span className="adm-ucell">
                <b className={e.ok ? '' : 'fail'}>{line.title}</b>
                <span className="adm-usub">{line.sub}</span>
              </span>
              <span className={'adm-usub adm-num' + (e.ok ? '' : ' fail')}>{fmtWhen(e.ts)}</span>
            </div>
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
