// views/admin/Users.jsx — 用户管理列表(fork,P1)
//
// 设计 §3.2:桌面表格 + 移动(<900px)卡片;搜索本地过滤(自托管量级,§设计性能原则:
// 不全量轮询,手动刷新 + 进入页面拉取);分页客户端 50/页。
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { fmtDate } from '../../lib/format.js'

export default function Users() {
  const nav = useNavigate()
  const { role } = useOutletContext()
  const [data, setData] = useState(null)
  const [err, setErr] = useState(null)
  const [q, setQ] = useState('')
  const [page, setPage] = useState(0)

  const load = () => { setErr(null); api('/api/admin/users').then(setData).catch(e => setErr(e.message)) }
  useEffect(load, [])
  useEffect(() => { setPage(0) }, [q])

  const rows = useMemo(() => {
    const us = data?.users || []
    const s = q.trim().toLowerCase()
    return s ? us.filter(u => (u.name || '').toLowerCase().includes(s) || u.id.includes(s)) : us
  }, [data, q])
  const pages = Math.max(1, Math.ceil(rows.length / 50))
  const view = rows.slice(page * 50, page * 50 + 50)

  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>

  return (
    <>
      <div className="adm-toolbar">
        <input className="adm-search" type="search" placeholder={adminT('搜索')} value={q}
          onChange={e => setQ(e.target.value)} aria-label={adminT('用户')} />
        <span className="adm-count">{rows.length}</span>
        <button className="adm-plain" onClick={load}>{adminT('刷新')}</button>
      </div>

      <div className="adm-table" role="table" aria-label={adminT('用户')}>
        <div className="adm-tr adm-th" role="row">
          <span role="columnheader">{adminT('用户')}</span>
          <span role="columnheader">{adminT('累计训练')}</span>
          <span role="columnheader">{adminT('刷新')}</span>
          <span role="columnheader"></span>
        </div>
        {view.map(u => (
          <button className="adm-tr adm-td" role="row" key={u.id}
            onClick={() => nav('/admin/users/' + encodeURIComponent(u.id))}>
            <span className="adm-ucell">
              <b>{u.name}</b>
              <span className="adm-usub">{u.created ? fmtDate(u.created) : ''}</span>
            </span>
            <span className="num">{u.workouts}</span>
            <span className="adm-usub">{u.lastSync ? adminWhen(typeof u.lastSync === 'number' ? u.lastSync : Date.parse(u.lastSync)) : '–'}</span>
            <span className="adm-badges">
              {u.admin && <span className="adm-badge">{role === 'support' ? adminT('客服(只读)') : adminT('超级管理员')}</span>}
              {u.live && <span className="adm-badge">{adminT('实时训练中')}</span>}
              {u.disabled && <span className="adm-badge readonly">{adminT('需要关注')}</span>}
            </span>
          </button>
        ))}
        {rows.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
      </div>

      {pages > 1 && (
        <div className="adm-pager">
          <button disabled={page === 0} onClick={() => setPage(p => p - 1)}>‹</button>
          <span>{page + 1} / {pages}</span>
          <button disabled={page >= pages - 1} onClick={() => setPage(p => p + 1)}>›</button>
        </div>
      )}
    </>
  )
}
