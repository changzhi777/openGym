import { useCallback, useEffect, useMemo, useState } from 'react'
import { useStore } from '../../store/useStore.js'
import { adminT } from '../../lib/admin/i18n.js'
import QrCanvas from '../../components/QrCanvas.jsx'
import { t } from '../../lib/i18n.js'
import {
  createTeam, distributePlan, getTeam, kickMember, mintInvite,
  renameTeam, revokeInvite, revokePlan, teamBoard, teamInsights, teamMembers,
  teamPlans, transferTeam,
} from '../../lib/api-team.js'
import { isPending } from '../../lib/team-load.js'

// 团队 tab(教练位,fork;团队模式):建团 → 邀请码(QR+大字,15min) → 成员管理(踢/转让)
// → 计划分发(从自己档 routines 拼快照,确认制版本链) → 看板 + insights 分析解读。
// 语义真源 muhe-fit api-go/internal/team + coach_console.html;样式复用 admin-console.css。
export default function TeamTab() {
  const S = useStore(s => s.S)
  const [view, setView] = useState('loading') // loading | none | member | owner
  const [team, setTeam] = useState(null)
  const [members, setMembers] = useState([])
  const [invite, setInvite] = useState(null) // {code, expires}
  const [plans, setPlans] = useState([])
  const [board, setBoard] = useState(null) // {live, weeks, rows}
  const [note, setNote] = useState('')

  const refresh = useCallback(async () => {
    try {
      const id = await getTeam()
      if (!id.team) { setView('none'); return }
      if (id.role !== 'owner') { setTeam(id.team); setView('member'); return }
      setTeam({ ...id.team, role: 'owner' })
      setView('owner')
      const [ms, ps] = await Promise.all([teamMembers(), teamPlans()])
      setMembers(ms.members || [])
      setPlans(ps.plans || [])
    } catch { setView('none') }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const loadBoard = useCallback(async () => {
    try {
      const [b, ins] = await Promise.all([teamBoard(), teamInsights()])
      setBoard({ ...b, insightRows: ins.rows || [] })
    } catch { setNote(t('看板加载失败')) }
  }, [])

  useEffect(() => { if (view === 'owner') loadBoard() }, [view, loadBoard])

  async function act(fn, ok) {
    try { await fn(); if (ok) setNote(ok); refresh() } catch (e) { setNote(errMsg(e)) }
  }
  const errMsg = e => {
    const code = e?.code
    if (code === 'team-live-exists') return adminT('你已有一个在带的团队')
    if (code === 'team-owner-only') return adminT('需要团长权限')
    if (code === 'team-code-invalid') return adminT('邀请码无效')
    if (e?.status === 409) return adminT('团队状态已变化,请刷新重试')
    return adminT('操作失败')
  }

  if (view === 'loading') return <p className="adm-note">{adminT('加载中…')}</p>

  if (view === 'none') {
    return (
      <div className="cc-team">
        <h2>{adminT('我的团队')}</h2>
        <p className="adm-note">{adminT('创建团队后生成邀请码,学员扫码加入;你可以在看板查看他们的训练打卡,并把训练计划分发给他们。')}</p>
        <button className="adm-btn" onClick={() => act(() => createTeam(adminT('我的团队')), adminT('团队已创建'))}>{adminT('创建团队')}</button>
        {note && <p className="adm-note">{note}</p>}
      </div>
    )
  }

  if (view === 'member') {
    return (
      <div className="cc-team">
        <h2>{adminT('我的团队')}</h2>
        <p className="adm-note">{adminT('你是团队「{0}」的学员——待接收计划与退团在「我的团队」应用页。', team.name)}</p>
      </div>
    )
  }

  return (
    <div className="cc-team">
      <h2>{adminT('我的团队')} · {team.name}</h2>
      {note && <p className="adm-note">{note}</p>}
      <NameCard name={team.name} onRename={n => act(() => renameTeam(n), adminT('团名已改'))} />
      <InviteCard invite={invite} setInvite={setInvite} onMint={() => act(async () => setInvite(await mintInvite()), adminT('邀请码已生成(15 分钟内有效)'))}
        onRevoke={() => act(() => revokeInvite(), adminT('邀请码已作废'))} />
      <MembersCard members={members} selfUid={members.find(m => m.role === 'owner')?.uid}
        onKick={uid => act(() => kickMember(uid), adminT('已移除成员'))}
        onTransfer={uid => act(() => transferTeam(uid), adminT('团长已转让'))} />
      <DispatchCard members={members} routines={S.routines || []} plans={plans}
        onDistribute={(uid, plan) => act(() => distributePlan(uid, plan), adminT('计划已分发'))}
        onRevoke={id => act(() => revokePlan(id), adminT('分发已撤回'))} />
      <BoardCard board={board} />
    </div>
  )
}

function NameCard({ name, onRename }) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(name)
  if (!editing) return (
    <div className="adm-card cc-name">
      <span className="cc-label">{adminT('团名')}</span>
      <strong>{name}</strong>
      <button className="adm-btn" onClick={() => { setDraft(name); setEditing(true) }}>{adminT('改名')}</button>
    </div>
  )
  return (
    <div className="adm-card cc-name">
      <input value={draft} onChange={e => setDraft(e.target.value)} maxLength={80} />
      <button className="adm-btn" disabled={!draft.trim()} onClick={() => { onRename(draft.trim()); setEditing(false) }}>{adminT('保存')}</button>
      <button className="adm-btn" onClick={() => setEditing(false)}>{adminT('取消')}</button>
    </div>
  )
}

function InviteCard({ invite, onMint, onRevoke }) {
  const [left, setLeft] = useState(0)
  useEffect(() => {
    if (!invite) return
    const tick = () => setLeft(Math.max(0, Math.ceil((invite.expires - Date.now()) / 60000)))
    tick()
    const tm = setInterval(tick, 15000)
    return () => clearInterval(tm)
  }, [invite])
  const link = invite ? `${location.origin}${location.pathname === '/' ? '' : location.pathname}/?team=${invite.code}` : ''
  return (
    <div className="adm-card cc-invite">
      <div className="cc-label">{adminT('邀请码')} · {adminT('一团一码,15 分钟有效,加入即核销')}</div>
      {invite ? (
        <div className="cc-invite-body">
          <QrCanvas value={link} size={180} />
          <div className="cc-invite-meta">
            <div className="cc-code">{invite.code}</div>
            <div className="adm-note">{adminT('剩余约 {0} 分钟 · 扫码或输入码即可加入', left)}</div>
            <button className="adm-btn danger" onClick={onRevoke}>{adminT('作废重出')}</button>
          </div>
        </div>
      ) : (
        <button className="adm-btn" onClick={onMint}>{adminT('生成邀请码')}</button>
      )}
    </div>
  )
}

function MembersCard({ members, selfUid, onKick, onTransfer }) {
  const others = members.filter(m => m.uid !== selfUid)
  return (
    <div className="adm-card">
      <div className="cc-label">{adminT('成员')} · {members.length}</div>
      {others.length === 0 && <p className="adm-note">{adminT('还没有学员——生成邀请码等他们加入。')}</p>}
      {others.map(m => (
        <div className="cc-member" key={m.uid}>
          <span className="cc-name">{m.displayName}</span>
          <span className="adm-note">{m.role === 'owner' ? adminT('团长') : adminT('学员')}</span>
          {m.role !== 'owner' && <>
            <button className="adm-btn" onClick={() => onTransfer(m.uid)}>{adminT('转让团长')}</button>
            <button className="adm-btn danger" onClick={() => onKick(m.uid)}>{adminT('移除')}</button>
          </>}
        </div>
      ))}
    </div>
  )
}

function DispatchCard({ members, routines, plans, onDistribute, onRevoke }) {
  const students = members.filter(m => m.role !== 'owner')
  const [uid, setUid] = useState('')
  const [name, setName] = useState('')
  const [picked, setPicked] = useState([])
  const live = useMemo(() => routines.filter(r => picked.has?.(r.id)), [routines, picked])
  const toggle = id => setPicked(p => { const n = new Set(p); n.has(id) ? n.delete(id) : n.add(id); return n })
  const pend = plans.filter(isPending)
  return (
    <div className="adm-card">
      <div className="cc-label">{adminT('计划分发')} · {adminT('快照中转:学员接收后才写入他的档案')}</div>
      {students.length === 0 ? <p className="adm-note">{adminT('还没有学员。')}</p> : (
        <div className="cc-dispatch">
          <select value={uid} onChange={e => setUid(e.target.value)}>
            <option value="">{adminT('选择学员…')}</option>
            {students.map(m => <option key={m.uid} value={m.uid}>{m.displayName}</option>)}
          </select>
          <input placeholder={adminT('计划名称,如:推腿拉·第一周')} value={name} onChange={e => setName(e.target.value)} />
          <div className="cc-routines">
            {routines.map(r => (
              <label key={r.id}><input type="checkbox" checked={picked.has?.(r.id) || false} onChange={() => toggle(r.id)} /> {r.name || r.id}</label>
            ))}
            {routines.length === 0 && <span className="adm-note">{adminT('你自己的档案还没有计划条目。')}</span>}
          </div>
          <button className="adm-btn" disabled={!uid || !name.trim() || !live.length}
            onClick={() => { onDistribute(uid, { name: name.trim(), routines: live }); setPicked(new Set()); setName('') }}>
            {adminT('分发给所选学员')}
          </button>
        </div>
      )}
      {pend.length > 0 && (
        <div className="cc-pending">
          <div className="cc-label">{adminT('待接收')}</div>
          {pend.map(a => (
            <div className="cc-member" key={a.id}>
              <span>{a.memberDisplayName || a.memberUid}</span>
              <span className="adm-note">{a.planName}</span>
              <button className="adm-btn danger" onClick={() => onRevoke(a.id)}>{adminT('撤回')}</button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function BoardCard({ board }) {
  if (!board) return null
  const rows = board.rows || []
  const ins = Object.fromEntries((board.insightRows || []).map(r => [r.uid, r]))
  const active = rows.filter(r => r.weekDone > 0).length
  const rated = rows.filter(r => r.weekPlanned > 0)
  const avg = rated.length ? Math.round(rated.reduce((s, r) => s + (ins[r.uid]?.rate ?? 0), 0) / rated.length) : null
  const FLAG = { hot: [adminT('全勤'), 'cc-f-hot'], miss: [adminT('缺练'), 'cc-f-miss'], gain: [adminT('1RM↑'), 'cc-f-gain'], drop: [adminT('体量↓'), 'cc-f-drop'] }
  return (
    <div className="adm-card">
      <div className="cc-label">{adminT('看板')} · {adminT('本周')} {active}/{rows.length} {adminT('活跃')}{avg != null ? ` · ${adminT('平均完成率')} ${avg}%` : ''}</div>
      {rows.length === 0 && <p className="adm-note">{adminT('还没有成员数据。')}</p>}
      {rows.map(r => {
        const i = ins[r.uid] || {}
        return (
          <div className="cc-board-row" key={r.uid}>
            <div className="cc-board-head">
              <strong>{r.displayName}</strong>
              {(i.flags || []).map(f => FLAG[f] && <span key={f} className={'cc-flag ' + FLAG[f][1]}>{FLAG[f][0]}</span>)}
            </div>
            <div className="adm-note">{i.verdict || adminT('暂无训练记录')}</div>
            <div className="cc-stats">
              <span>{r.weekDone}/{r.weekPlanned || '–'} {adminT('本周')}</span>
              <span>{r.streakWeeks || 0} {adminT('连续周')}</span>
              <span>{fmtVol(r.trendVolume4w)} {adminT('4 周体量')}</span>
              <span>{r.trendEst1rm > 0 ? Math.round(r.trendEst1rm) : '–'} {adminT('最佳 1RM')}</span>
            </div>
            <WeeklyBars weekly={r.weekly || []} />
          </div>
        )
      })}
    </div>
  )
}

function WeeklyBars({ weekly }) {
  const max = Math.max(...weekly, 1)
  return (
    <svg className="cc-spark" viewBox={`0 0 120 34`} preserveAspectRatio="none">
      {weekly.map((v, i) => {
        const h = v > 0 ? Math.max((v / max) * 30, 3) : 1.5
        return <rect key={i} x={(i * 10 + 1).toFixed(1)} y={(34 - h).toFixed(1)} width="8" height={h.toFixed(1)} rx="1.5" fill={v > 0 ? 'var(--green)' : 'var(--stroke)'} />
      })}
    </svg>
  )
}

const fmtVol = v => v >= 10000 ? (v / 1000).toFixed(1) + 't' : v >= 1000 ? (v / 1000).toFixed(1) + 'k' : Math.round(v || 0)
