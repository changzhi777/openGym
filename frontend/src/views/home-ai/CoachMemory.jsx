// CoachMemory.jsx — 「Coach 记住了什么」页(批次 4 后段补 UI 缺口,PIPL 查询/删除权)。
//
// 旁路路由 /coach-ai/memory(fork-only,不经 Settings 上游注册);入口在
// CoachChatHome 头部。双类记忆:narrative(叙事偏好)+ constraints(结构化约束,
// 即刻生效于计划)。删除 = 先约束后叙事的跨存储一致性(后端 D17);本页只做
// 列表与删除——编辑(PUT)接口已备,UI 待需要时加。
// 文案中文直写(与 home-ai/ 同策略,locale 键化批次 6 后段统一)。
import { useEffect, useState } from 'react'
import Icon from '../../components/Icon.jsx'
import { fetchCoachMemory, deleteMemoryEntry } from '../../lib/api-chat.js'
import '../../coach.css'

export default function CoachMemory() {
  const [data, setData] = useState(null) // {narrative, constraints}
  const [err, setErr] = useState('')
  const [busyId, setBusyId] = useState('')

  const load = () => {
    setErr('')
    fetchCoachMemory().then(setData).catch(e => setErr(e.message))
  }
  useEffect(load, [])

  async function remove(id) {
    setBusyId(id)
    try {
      await deleteMemoryEntry(id)
      // 乐观移除(后端已删;失败重载兜底)。
      setData(d => ({
        narrative: (d.narrative || []).filter(e => e.id !== id),
        constraints: (d.constraints || []).filter(e => e.id !== id),
      }))
    } catch (e) {
      if (e.status === 404) {
        load() // 已不在:回真值
      } else {
        setErr(e.message)
      }
    } finally {
      setBusyId('')
    }
  }

  const row = (e, kindLabel) => (
    <div key={e.id} className="bub assistant" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
      <div className="grow">
        <div>{e.text}</div>
        <div className="chat-st">{kindLabel}{e.at ? ` · ${e.at.slice(0, 10)}` : ''}</div>
      </div>
      <button
        className="iconbtn"
        aria-label="删除"
        disabled={busyId === e.id}
        onClick={() => remove(e.id)}
      >
        {busyId === e.id ? '…' : <Icon name="trash" />}
      </button>
    </div>
  )

  return (
    <div className="narrow chat">
      <div className="chat-hdr">
        <a className="iconbtn" href="#/coach-ai" aria-label="返回">
          <Icon name="chevronLeft" />
        </a>
        <div className="grow"><b>Coach 记住了什么</b></div>
      </div>

      <div className="chat-body">
        {err && <div className="chat-empty">加载失败 — {err}</div>}
        {!err && !data && <div className="chat-empty">加载中…</div>}
        {data && (
          <>
            {!!data.constraints.length && (
              <>
                <div className="chat-st">硬约束(计划生成强制遵守)</div>
                {data.constraints.map(e => row(e, '约束'))}
              </>
            )}
            {!!data.narrative.length && (
              <>
                <div className="chat-st" style={{ marginTop: 12 }}>偏好与习惯</div>
                {data.narrative.map(e => row(e, '偏好'))}
              </>
            )}
            {!data.constraints.length && !data.narrative.length && (
              <div className="chat-empty">
                还没有记住任何东西。聊天中对教练说「记住…」,它会先征求你的确认。
              </div>
            )}
            {(data.constraints.length || data.narrative.length) ? (
              <div className="chat-empty">删除即刻生效。全部记忆只属于你,不进任何推荐或共享数据。</div>
            ) : null}
          </>
        )}
      </div>
    </div>
  )
}
