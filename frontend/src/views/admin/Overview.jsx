// views/admin/Overview.jsx — 概览大盘(fork,批3 v2;设计 §3.3)
//
// Go 栈:GET /api/admin/overview 一次拿全部 KPI + 14 天趋势 + 需要关注 + 在线数;
// GET /api/admin/live(SSE)推送实时训练。Node 栈:overview 501 → 回落旧路径
// (users 客户端聚合,ai/营收渲染「待后端支持」—— 设计:无数据不显示假 0)。
// 支付未接入 → 营收 KPI 不渲染(§3.3 布局规则,方向 1 决议:营收占位页在批 4)。
import { useEffect, useRef, useState } from 'react'
import { useOutletContext } from 'react-router-dom'
import { api } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { auditLine, fmtWhen } from '../../lib/audit.js'
import { fmtNum } from '../../lib/format.js'
import LineChart from '../../components/LineChart.jsx'

const isToday = (iso, now) => !!iso && new Date(iso).toDateString() === new Date(now).toDateString()

function Kpi({ k, v, sub }) {
  return <div className="adm-kpi"><div className="k">{k}</div><div className="v">{v}</div>{sub && <div className="sub">{sub}</div>}</div>
}

const ATTENTION_LABEL = { quota_near: '配额将满', disabled: '已停用' }

export default function Overview() {
  const { readonly } = useOutletContext()
  const [ov, setOv] = useState(null)        // Go overview v2;null = Node(501)
  const [data, setData] = useState(null)    // 回落:GET /api/admin/users
  const [audit, setAudit] = useState(null)
  const [err, setErr] = useState(null)
  const [liveRows, setLiveRows] = useState(null) // SSE 覆盖;null = 未连上
  const [coachUsage, setCoachUsage] = useState(null) // AI-Native 批次5:三池用量;null = 未配置/501
  const esRef = useRef(null)

  const load = () => {
    setErr(null)
    api('/api/admin/overview').then(setOv).catch(() => setOv(null))
    api('/api/admin/audit?limit=6').then(setAudit).catch(() => setAudit(null))
    api('/api/admin/users').then(setData).catch(e => setErr(e.message))
    // AI-Native(批次5):Go 实现回真数据;Node 501 → null → 卡片不渲染(无数据不显示假 0)。
    api('/api/admin/coach/usage-overview').then(setCoachUsage).catch(() => setCoachUsage(null))
  }
  useEffect(load, [])

  // SSE(Go 栈):live 快照覆盖轮询;首错即关(Node 501 不轰炸重连)。
  useEffect(() => {
    const es = new EventSource('/api/admin/live')
    esRef.current = es
    es.addEventListener('live', e => {
      try { setLiveRows(JSON.parse(e.data)?.live || []) } catch { /* 坏帧忽略 */ }
    })
    es.onerror = () => { es.close(); esRef.current = null }
    return () => es.close()
  }, [])

  if (err && !data && !ov) return <div className="adm-empty">{adminT('加载失败')} — {err} <button onClick={load}>{adminT('重试')}</button></div>

  const now = ov?.now || data?.now || Date.now()
  const us = data?.users || []
  const liveFromRows = us.filter(u => u.live)
  const liveRowsEff = liveRows ?? liveFromRows   // SSE 优先,回落 users 行

  // KPI(§3.3):Go 数栈优先;Node 回落客户端聚合。
  const kUsers = ov?.users ?? us.length
  const kActive = ov?.active_today ?? us.filter(u => isToday(u.lastSync, now) || isToday(u.lastWorkout, now)).length
  const kWorkoutsToday = ov?.workouts_today ?? us.filter(u => isToday(u.lastWorkout, now)).reduce((n, u) => n + (u.workouts || 0) * 0, 0)
  const kWorkouts = ov?.workouts ?? us.reduce((n, u) => n + (u.workouts || 0), 0)
  const attention = ov?.attention || (us.filter(u => u.disabled).length ? [{ code: 'disabled', count: us.filter(u => u.disabled).length }] : [])
  const trend = ov?.trend_14d || []
  const trendPts = trend.map(t => ({ t: Date.parse(t.date), y: t.workouts, d: t.date }))
  const trendPtsActive = trend.map(t => ({ t: Date.parse(t.date), y: t.active, d: t.date }))

  return (
    <>
      <div className="adm-kpis">
        <Kpi k={adminT('用户总数')} v={kUsers} sub={ov ? `+${ov.today_new ?? 0}` : null} />
        <Kpi k={adminT('今日活跃')} v={kActive} sub={kUsers ? Math.round(kActive / kUsers * 100) + '%' : null} />
        <Kpi k={adminT('训练记录')} v={kWorkoutsToday} sub={`${adminT('累计训练')} ${kWorkouts}`} />
        <Kpi k={adminT('AI 调用(本月)')} v={ov ? (ov.ai_calls_month ?? '–') : '–'} sub={ov ? null : adminT('待后端支持')} />
        {!readonly && <Kpi k={adminT('营收(本月)')} v="–" sub={adminT('待后端支持')} />}
      </div>

      {attention.length > 0 && (
        <div className="adm-attention" role="status">
          {adminT('需要关注')} · {attention.map(a => `${adminT(ATTENTION_LABEL[a.code] || a.code)} ${a.count}`).join(' · ')}
        </div>
      )}

      <div className="adm-grid-2">
        <div className="adm-card">
          <h3>{adminT('累计训练')} · 14 {adminT('天')}</h3>
          {trendPts.length > 0
            ? <LineChart points={trendPts} h={150} series={[{ points: trendPts, color: 'var(--acc)' }, { points: trendPtsActive, color: 'var(--ok-text)' }]} />
            : <div className="adm-empty">{adminT('待后端支持')}</div>}
        </div>
        <div className="adm-card">
          <h3>{adminT('实时训练中')} <span className="adm-usub">{liveRowsEff.length}</span></h3>
          {liveRowsEff.length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
          {liveRowsEff.map(u => <div key={u.id} className="adm-rowline">
            <span>{u.name}</span>
            <span className="ok">{u.exTotal ? `${u.exIdx}/${u.exTotal} · ` : ''}{u.setsDone}/{u.setsTotal} · {adminWhen(u.startedAt)}</span>
          </div>)}
        </div>
      </div>

      <div className="adm-grid-2">
        <div className="adm-card"><h3>{adminT('最近管理操作')}</h3>
          {(!audit?.events || audit.events.length === 0) && <div className="adm-empty">{adminT('暂无数据')}</div>}
          {audit?.events?.map(e => {
            const line = auditLine(e)
            return <div key={e.id} className="adm-rowline">
              <span>{line.title}</span>
              <span className={e.ok ? 'ok' : 'fail'}>{fmtWhen(e.ts)}</span>
            </div>
          })}
        </div>
        {coachUsage && (
          <div className="adm-card" data-testid="coach-usage">
            <h3>{adminT('AI 教练用量')} <span className="adm-usub">{adminT('今日活跃')} {coachUsage.users}</span></h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8 }}>
              <Kpi k={adminT('今日回合')} v={coachUsage.day.messages} />
              <Kpi k={adminT('今日 chat')} v={fmtNum(coachUsage.day.chat)} sub="tokens" />
              <Kpi k={adminT('今日 review')} v={fmtNum(coachUsage.day.review)} sub="tokens" />
              <Kpi k={adminT('本月合计')} v={fmtNum(coachUsage.month.chat + coachUsage.month.review + coachUsage.month.embedding)} sub="tokens" />
            </div>
            <div className="adm-usub">14 {adminT('天')}趋势: {coachUsage.trend14d?.map(d => d.messages).join(' / ') || '–'}</div>
          </div>
        )}
        <div className="adm-card"><h3>{adminT('邀请码')}</h3>
          <div className="adm-empty">{adminT('待后端支持')}</div>
        </div>
      </div>
    </>
  )
}
