// views/coach-console/Students.jsx — 学员列表 + 手机号绑定(教练工作台主视图)。
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { coachStudents, coachBindStudent } from '../../lib/api.js'
import { adminT, adminWhen } from '../../lib/admin/i18n.js'

export default function Students() {
  const nav = useNavigate()
  const [rows, setRows] = useState(null)
  const [err, setErr] = useState(null)
  const [phone, setPhone] = useState('')
  const [msg, setMsg] = useState(null)

  const load = () => {
    setErr(null)
    coachStudents().then(d => setRows(d.students)).catch(e => setErr(e.message))
  }
  useEffect(load, [])

  const bind = async () => {
    setMsg(null)
    if (!/^1\d{10}$/.test(phone)) { setMsg(adminT('请输入有效的中国大陆手机号')); return }
    try {
      await coachBindStudent(phone)
      setPhone('')
      setMsg(null)
      load()
    } catch (e) { setMsg(e.message) }
  }

  return (
    <>
      <div className="adm-toolbar">
        <strong>{adminT('我的学员')}</strong>
        <input className="adm-search" inputMode="numeric" maxLength={11}
          placeholder={adminT('学员手机号绑定')} value={phone}
          onChange={e => setPhone(e.target.value.replace(/\D/g, '').slice(0, 11))}
          aria-label={adminT('学员手机号')} />
        <button className="adm-plain" onClick={bind}>{adminT('绑定')}</button>
      </div>
      {msg && <div className="adm-attention" role="alert">{msg}</div>}
      {err && <div className="adm-attention" role="alert">{adminT('加载失败')} — {err}</div>}
      <div className="adm-table" role="table" aria-label={adminT('我的学员')}>
        <div className="adm-tr adm-th" role="row">
          <span role="columnheader">{adminT('学员')}</span>
          <span role="columnheader">{adminT('状态')}</span>
          <span role="columnheader">{adminT('训练记录')}</span>
          <span role="columnheader">{adminT('最近训练')}</span>
          <span role="columnheader">{adminT('周计划')}</span>
          <span role="columnheader">{adminT('绑定于')}</span>
        </div>
        {(rows || []).map(s => (
          <button key={s.id} className="adm-tr adm-td" role="row"
            onClick={() => nav('/coach-console/students/' + encodeURIComponent(s.id))}>
            <span className="adm-ucell">
              <b>{s.name}</b>
              <span className="adm-usub">{s.phoneTail}</span>
            </span>
            <span>{s.live ? <span className="adm-badge">{adminT('训练中')}</span>
              : <span className="adm-usub">{adminT('离线')}</span>}</span>
            <span className="num">{s.workouts}</span>
            <span className="adm-usub">{s.lastWorkout || '–'}</span>
            <span className="adm-usub">{s.planDays ? adminT('每周 {0} 练', s.planDays) : '–'}</span>
            <span className="adm-usub">{s.boundAt ? adminWhen(Date.parse(s.boundAt)) : ''}</span>
          </button>
        ))}
        {rows?.length === 0 && <div className="adm-empty">{adminT('暂无学员——用上方手机号绑定已注册的会员')}</div>}
      </div>
    </>
  )
}
