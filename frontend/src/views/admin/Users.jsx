// views/admin/Users.jsx — 用户管理列表(fork,批2)
//
// 设计 §3.4/3.5:统计头(总数/今日新增/已停用)+ 搜索(300ms 防抖)/角色/状态/排序筛选
// + 经典页码分页(§3.4:用户列表是稳定集合,要"跳页"与总数)+ 行内脱敏邮箱。
// 服务端语义(Go):q/role/status/sort/page/per_page 全参数化;Node 忽略参数答
// page1 全量 → 分页器按 total 缺失回落单页显示(双栈兼容)。
import { useEffect, useState } from 'react'
import { useNavigate, useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { fmtDate } from '../../lib/format.js'

const PER_PAGE = 25

const ROLES = [
  { k: '', label: '全部' },
  { k: 'super_admin', label: '超级管理员' },
  { k: 'ops', label: '运营' },
  { k: 'support', label: '客服(只读)' },
  { k: 'none', label: '普通用户' },
]
const STATUS = [{ k: '', label: '全部' }, { k: 'active', label: '正常' }, { k: 'disabled', label: '已停用' }]
const SORTS = [
  { k: 'last_sync', label: '最近同步' },
  { k: 'workouts', label: '训练数' },
  { k: 'created', label: '加入时间' },
]

const ROLE_NAME = { super_admin: '超级管理员', ops: '运营', support: '客服(只读)' }

export default function Users() {
  const nav = useNavigate()
  const { role: myRole } = useOutletContext()
  const [data, setData] = useState(null)
  const [err, setErr] = useState(null)
  const [qIn, setQIn] = useState('')       // 输入值
  const [q, setQ] = useState('')           // 防抖后的生效值
  const [roleF, setRoleF] = useState('')
  const [statusF, setStatusF] = useState('')
  const [sort, setSort] = useState('last_sync')
  const [page, setPage] = useState(1)

  useEffect(() => {
    const t = setTimeout(() => { setQ(qIn.trim()); setPage(1) }, 300)
    return () => clearTimeout(t)
  }, [qIn])
  useEffect(() => { setPage(1) }, [roleF, statusF, sort])

  const load = () => {
    setErr(null)
    const p = new URLSearchParams({ page: String(page), per_page: String(PER_PAGE), sort })
    if (q) p.set('q', q)
    if (roleF) p.set('role', roleF)
    if (statusF) p.set('status', statusF)
    api('/api/admin/users?' + p).then(setData).catch(e => setErr(e.message))
  }
  useEffect(load, [q, roleF, statusF, sort, page])

  const rows = data?.users || []
  const total = typeof data?.total === 'number' ? data.total : rows.length
  const pages = Math.max(1, Math.ceil(total / PER_PAGE))
  const select = (value, set, opts, label) => (
    <select value={value} onChange={e => set(e.target.value)} aria-label={adminT(label)}>
      {opts.map(o => <option key={o.k} value={o.k}>{adminT(o.label)}</option>)}
    </select>
  )

  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>

  return (
    <>
      <div className="adm-toolbar">
        <strong>{adminT('用户')}</strong>
        <span className="adm-count">
          {total} · {adminT('今日新增')} {data?.today_new ?? '–'} · {adminT('已停用')} {data?.disabled_count ?? '–'}
        </span>
        <button className="adm-plain" onClick={load}>{adminT('刷新')}</button>
      </div>
      <div className="adm-toolbar">
        <input className="adm-search" type="search" placeholder={adminT('搜索姓名或邮箱')} value={qIn}
          onChange={e => setQIn(e.target.value)} aria-label={adminT('搜索姓名或邮箱')} />
        {select(roleF, setRoleF, ROLES, '角色与权限')}
        {select(statusF, setStatusF, STATUS, '状态')}
        {select(sort, setSort, SORTS, '排序')}
      </div>

      <div className="adm-table" role="table" aria-label={adminT('用户')}>
        <div className="adm-tr adm-th" role="row">
          <span role="columnheader">{adminT('用户')}</span>
          <span role="columnheader">{adminT('角色与权限')}</span>
          <span role="columnheader">{adminT('状态')}</span>
          <span role="columnheader">{adminT('训练记录')}</span>
          <span role="columnheader">{adminT('最近同步')}</span>
          <span role="columnheader">{adminT('加入时间')}</span>
        </div>
        {rows.map(u => (
          <button className="adm-tr adm-td" role="row" key={u.id}
            onClick={() => nav('/admin/users/' + encodeURIComponent(u.id))}>
            <span className="adm-ucell">
              <b>{u.name}</b>
              <span className="adm-usub">{u.email || u.id}</span>
              {(u.hasPhone || u.hasWx) && (
                <span className="adm-usub">
                  {u.hasPhone && <span className="adm-badge">{adminT('手机')}</span>}
                  {u.hasWx && <span className="adm-badge">{adminT('微信')}</span>}
                </span>
              )}
            </span>
            <span>{u.role ? <span className="adm-badge">{adminT(ROLE_NAME[u.role] || u.role)}</span>
              : <span className="adm-usub">{adminT('普通用户')}</span>}</span>
            <span className="adm-usub">{u.disabled ? adminT('已停用') : adminT('正常')}</span>
            <span className="num">{u.workouts}</span>
            <span className="adm-usub">{u.lastSync ? adminWhen(typeof u.lastSync === 'number' ? u.lastSync : Date.parse(u.lastSync)) : '–'}</span>
            <span className="adm-usub">{u.created ? fmtDate(u.created.slice(0, 10)) : '–'}</span>
          </button>
        ))}
        {rows.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
      </div>

      {pages > 1 && (
        <div className="adm-pager">
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>‹</button>
          <span>{page} / {pages}</span>
          <button disabled={page >= pages} onClick={() => setPage(p => p + 1)}>›</button>
        </div>
      )}
    </>
  )
}
