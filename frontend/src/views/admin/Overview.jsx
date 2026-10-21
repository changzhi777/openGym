// views/admin/Overview.jsx — 概览大盘(fork,P0)
//
// 设计依据 docs/admin-ui-design.md §3.1:5 KPI + 最近管理操作 + 需要关注。
// 数据现实:后端(P0/Node)无聚合端点 —— KPI 由 GET /api/admin/users 客户端聚合;
// AI 调用/营收等待 Go 后端,渲染为「待后端支持」占位卡(§设计:无数据不显示假 0)。
import { useEffect, useState } from 'react'
import { useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { auditLine, fmtWhen } from '../../lib/audit.js'

const isToday = (iso, now) => !!iso && new Date(iso).toDateString() === new Date(now).toDateString()

function Kpi({ k, v, sub }) {
  return <div className="adm-kpi"><div className="k">{k}</div><div className="v">{v}</div>{sub && <div className="sub">{sub}</div>}</div>
}

export default function Overview() {
  const { readonly } = useOutletContext()
  const [data, setData] = useState(null)   // {users,now}
  const [audit, setAudit] = useState(null)
  const [err, setErr] = useState(null)

  const load = () => {
    setErr(null)
    api('/api/admin/users').then(setData).catch(e => setErr(e.message))
    api('/api/admin/audit?limit=6').then(setAudit).catch(() => setAudit(null))
  }
  useEffect(load, [])

  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>

  const now = data?.now || Date.now()
  const us = data?.users || []
  const activeToday = us.filter(u => isToday(u.lastSync, now) || isToday(u.lastWorkout, now)).length
  const workouts = us.reduce((n, u) => n + (u.workouts || 0), 0)
  const live = us.filter(u => u.live).length
  const disabled = us.filter(u => u.disabled).length

  return (
    <>
      <div className="adm-kpis">
        <Kpi k={adminT('用户总数')} v={us.length} />
        <Kpi k={adminT('今日活跃')} v={activeToday} />
        <Kpi k={adminT('累计训练')} v={workouts} />
        <Kpi k={adminT('AI 调用(本月)')} v="–" sub={adminT('待后端支持')} />
        {!readonly && <Kpi k={adminT('营收(本月)')} v="–" sub={adminT('待后端支持')} />}
      </div>

      {live > 0 && (
        <div className="adm-card"><h3>{adminT('实时训练中')}</h3>
          {us.filter(u => u.live).map(u => <div key={u.id} className="adm-rowline"><span>{u.name}</span><span className="ok">{adminWhen(u.live)}</span></div>)}
        </div>
      )}

      {disabled > 0 && (
        <div className="adm-attention" role="status">{adminT('需要关注')} · {disabled}</div>
      )}

      <div className="adm-grid-2">
        <div className="adm-card"><h3>{adminT('最近管理操作')}</h3>
          {!audit && <div className="adm-empty">{adminT('暂无数据')}</div>}
          {audit?.events?.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
          {audit?.events?.map(e => {
            const line = auditLine(e)
            return <div key={e.id} className="adm-rowline">
              <span>{line.title}</span>
              <span className={e.ok ? 'ok' : 'fail'}>{fmtWhen(e.ts)}</span>
            </div>
          })}
        </div>
        <div className="adm-card"><h3>{adminT('邀请码')}</h3>
          <div className="adm-empty">{adminT('待后端支持')}</div>
        </div>
      </div>
    </>
  )
}
