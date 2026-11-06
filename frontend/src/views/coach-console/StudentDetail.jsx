// views/coach-console/StudentDetail.jsx — 学员白名单详情 + 模板化计划下发(R2)。
// 编辑器三起点:学员当前计划 / 内置四模板 / 空白;结构化微调(routine 名/动作行/周排程)。
// 不复用 RoutineEdit(其深耦合 useStore 学员上下文);独立轻量编辑器产 routines+week。
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { coachStudentDetail, coachPutStudentPlan, coachUnbindStudent, coachStudentExportUrl } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'
import { EXIDX, canonicalExId } from '../../lib/exercises.js'
import { starterPlanTemplates } from '../../lib/starter.js'

const TEMPLATES = starterPlanTemplates()
const TEMPLATE_NAMES = { ppl: 'Push / Pull / Legs', 'upper-lower': 'Upper / Lower', 'full-body': 'Full Body', '5x5': '5×5' }
const DAYN = ['日', '一', '二', '三', '四', '五', '六']
const exName = id => EXIDX[canonicalExId(id)]?.name || id
// datalist 以动作名显示,提交要动作 id:建 名称→id 反查表(EXIDX 同名动作取第一个)。
const NAME2ID = Object.fromEntries(
  Object.entries(EXIDX).filter(([, e]) => e.name).map(([eid, e]) => [e.name, eid]),
)

export default function StudentDetail() {
  const { id } = useParams()
  const nav = useNavigate()
  const [d, setD] = useState(null)
  const [err, setErr] = useState(null)
  const [editing, setEditing] = useState(false)
  const [routines, setRoutines] = useState([])
  const [week, setWeek] = useState({})
  const [expect, setExpect] = useState(null)
  const [busy, setBusy] = useState(false)
  const [flash, setFlash] = useState(null)

  const load = () => {
    setErr(null)
    coachStudentDetail(id).then(x => {
      setD(x)
      setRoutines(x.routines || [])
      setWeek(x.week || {})
      setExpect(x.rev ?? null) // 下发并发基:学员 state 当前 _rev
    }).catch(e => setErr(e.message))
  }
  useEffect(load, [id])

  const startEdit = source => {
    const base = source === 'starter' ? TEMPLATES.ppl : (d.routines || [])
    const baseWeek = source === 'starter'
      ? Object.fromEntries([1, 3, 5].map((day, i) => [String(day), base[i]?.id]))
      : Object.fromEntries(Object.entries(d.week || {}).map(([k, v]) => [String(k), v]))
    setRoutines(base)
    setWeek(baseWeek)
    setEditing(true)
  }

  const setRoutine = (i, patch) =>
    setRoutines(rs => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const setEx = (ri, xi, patch) =>
    setRoutines(rs => rs.map((r, j) => (j === ri ? { ...r, ex: r.ex.map((e, k) => (k === xi ? { ...e, ...patch } : e)) } : r)))
  const addEx = ri => setRoutines(rs => rs.map((r, j) => (j === ri ? { ...r, ex: [...r.ex, { id: '0025', sets: 3, reps: 10, weight: 0 }] } : r)))
  const delEx = (ri, xi) => setRoutines(rs => rs.map((r, j) => (j === ri ? { ...r, ex: r.ex.filter((_, k) => k !== xi) } : r)))

  const toggleDay = day => setWeek(w => {
    const next = { ...w }
    if (next[day]) delete next[day]
    else next[day] = routines[0]?.id || ''
    return next
  })
  const setDayRoutine = (day, rid) => setWeek(w => ({ ...w, [day]: rid }))

  const push = async () => {
    setBusy(true); setFlash(null)
    try {
      await coachPutStudentPlan(id, routines, week, expect, true)
      setFlash({ ok: true, msg: adminT('计划已下发,学员下次同步即生效') })
      setEditing(false)
      load()
    } catch (e) {
      setFlash({ ok: false, msg: e.message + (e?.data?.code === 'conflict' ? ' — ' + adminT('学员刚同步过,请基于最新计划重发') : '') })
    } finally { setBusy(false) }
  }
  const unbind = async () => {
    if (!window.confirm(adminT('解绑后该学员将从你的工作台移除,确定?'))) return
    try { await coachUnbindStudent(id); nav('/coach-console') } catch (e) { setFlash({ ok: false, msg: e.message }) }
  }

  if (err) return <div className="adm-empty">{adminT('加载失败')} — {err} <button className="adm-plain" onClick={load}>{adminT('重试')}</button></div>
  if (!d) return <div className="adm-empty">{adminT('加载中…')}</div>
  const s = d.student

  return (
    <>
      <div className="adm-toolbar">
        <button className="adm-plain" onClick={() => nav('/coach-console')}>‹ {adminT('我的学员')}</button>
      </div>
      <div className="adm-card">
        <div className="adm-rowline">
          <span className="adm-ucell">
            <b style={{ fontSize: 16 }}>{s.name}</b>
            <span className="adm-usub">{s.phoneTail}{s.planDays ? ' · ' + adminT('每周 {0} 练', s.planDays) : ''}</span>
          </span>
          <span className="adm-badges">
            {s.live ? <span className="adm-badge">{adminT('训练中')}</span> : <span className="adm-badge readonly">{adminT('离线')}</span>}
          </span>
        </div>
        <div className="adm-rowline">
          <span className="adm-usub">{adminT('最近同步')} {d.lastSync ? adminWhen(d.lastSync) : '–'}</span>
          <a className="adm-plain" href={coachStudentExportUrl(id)}>{adminT('下载数据')}</a>
        </div>
      </div>

      <div className="adm-kpis">
        <div className="adm-kpi"><div className="k">{adminT('训练记录')}</div><div className="v">{(d.workouts || []).length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('体重记录')}</div><div className="v">{(d.bodyweight || []).length}</div></div>
        <div className="adm-kpi"><div className="k">{adminT('周计划')}</div><div className="v">{s.planDays ?? 0}</div></div>
      </div>

      <div className="adm-card">
        <div className="adm-rowline">
          <h3 style={{ margin: 0 }}>{adminT('训练计划')}</h3>
          {!editing && <button className="adm-plain" onClick={() => startEdit('current')}>{adminT('编辑并下发')}</button>}
        </div>
        {flash && <div className="adm-attention" role="alert" style={{ color: flash.ok ? 'var(--ok-text, #248a3d)' : undefined }}>{flash.msg}</div>}
        {!editing ? (
          <>
            {(d.routines || []).map(r => (
              <div key={r.id} className="adm-rowline">
                <span><b>{r.name}</b> · {(r.ex || []).length} {adminT('动作')}</span>
                <span className="adm-usub">{(r.ex || []).map(e => exName(e.id)).join(' · ').slice(0, 80)}</span>
              </div>
            ))}
            {(d.routines || []).length === 0 && <div className="adm-empty">{adminT('学员暂无计划')}</div>}
          </>
        ) : (
          <>
            <div className="adm-toolbar">
              <span className="adm-usub">{adminT('起点')}</span>
              <button className="adm-plain" onClick={() => startEdit('current')}>{adminT('学员当前')}</button>
              <button className="adm-plain" onClick={() => startEdit('starter')}>{adminT('内置模板')}</button>
              {Object.entries(TEMPLATES).map(([tid]) => (
                <button key={tid} className="adm-plain" onClick={() => { const rs = TEMPLATES[tid]; setRoutines(rs); setWeek(Object.fromEntries([1, 3, 5].slice(0, rs.length).map((day, i) => [String(day), rs[i]?.id]))) }}>{TEMPLATE_NAMES[tid] || tid}</button>
              ))}
            </div>
            {routines.map((r, ri) => (
              <div key={r.id || ri} className="adm-rowline" style={{ flexDirection: 'column', alignItems: 'stretch', borderBottom: '1px solid var(--hair)', padding: '8px 0' }}>
                <input className="adm-search" value={r.name} onChange={e => setRoutine(ri, { name: e.target.value })} aria-label={adminT('计划名')} />
                {(r.ex || []).map((e, xi) => (
                  <div key={xi} style={{ display: 'flex', gap: 6, alignItems: 'center', marginTop: 4 }}>
                    <input className="adm-search" style={{ flex: 2 }} list="coach-exlist" value={exName(e.id)}
                      onChange={ev => {
                        const name = ev.target.value
                        setEx(ri, xi, { id: NAME2ID[name] || ev.target.value })
                      }} aria-label={adminT('动作')} />
                    <input className="adm-search" style={{ width: 56 }} inputMode="numeric" value={e.sets}
                      onChange={ev => setEx(ri, xi, { sets: +ev.target.value.replace(/\D/g, '') || 0 })} aria-label={adminT('组数')} />
                    <input className="adm-search" style={{ width: 56 }} inputMode="numeric" value={e.reps}
                      onChange={ev => setEx(ri, xi, { reps: +ev.target.value.replace(/\D/g, '') || 0 })} aria-label={adminT('次数')} />
                    <button className="adm-danger-ghost" onClick={() => delEx(ri, xi)}>×</button>
                  </div>
                ))}
                <button className="adm-plain" style={{ marginTop: 4 }} onClick={() => addEx(ri)}>{adminT('加动作')}</button>
              </div>
            ))}
            <div className="adm-toolbar">
              <span className="adm-usub">{adminT('周排程')}</span>
              {DAYN.map((n, day) => (
                <button key={day} className={'adm-plain adm-chip' + (week[day] ? ' on' : '')}
                  onClick={() => toggleDay(String(day))}>{adminT('周' + n)}</button>
              ))}
            </div>
            {Object.keys(week).length > 0 && (
              <div className="adm-toolbar">
                {Object.entries(week).map(([day, rid]) => (
                  <span key={day} className="adm-usub">
                    {adminT('周' + DAYN[+day])}
                    <select className="adm-rolepick" value={rid} onChange={e => setDayRoutine(day, e.target.value)} aria-label={adminT('选计划')}>
                      {routines.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
                    </select>
                  </span>
                ))}
              </div>
            )}
            <div style={{ height: 10 }} />
            <button className="adm-danger" disabled={busy} onClick={push}>{busy ? adminT('下发中…') : adminT('下发计划给学员')}</button>
            <button className="adm-plain" style={{ marginLeft: 8 }} onClick={() => setEditing(false)}>{adminT('取消')}</button>
            <div style={{ height: 12 }} />
          </>
        )}
      </div>

      <div className="adm-card">
        <h3 style={{ margin: '0 0 8px' }}>{adminT('训练历史')}</h3>
        <div className="adm-table">
          {(d.workouts || []).slice(0, 60).map(w => (
            <div className="adm-tr" key={w.id || w.d}>
              <span>{w.d}</span>
              <span className="adm-usub">{w.name} · {adminT('容量')} {Math.round(w.vol || 0)}</span>
            </div>
          ))}
          {(d.workouts || []).length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
        </div>
      </div>

      <div className="adm-card">
        <h3 style={{ margin: '0 0 8px' }}>{adminT('体重曲线')}</h3>
        {(d.bodyweight || []).slice(-30).map(e => (
          <div className="adm-rowline" key={e.d}>
            <span className="adm-usub">{e.d}</span><span>{e.w} {d.unit}</span>
          </div>
        ))}
        {(d.bodyweight || []).length === 0 && <div className="adm-empty">{adminT('暂无数据')}</div>}
        <div style={{ height: 8 }} />
        <button className="adm-danger-ghost" onClick={unbind}>{adminT('解绑学员')}</button>
      </div>

      <datalist id="coach-exlist">
        {Object.entries(EXIDX).map(([eid, e]) => <option key={eid} value={e.name}>{e.name}</option>)}
      </datalist>
    </>
  )
}
