// CoachChatHome.jsx — AI-Native 对话首页(批次 4,T4 home-ai 旁路目录)。
//
// 上游 /coach(CoachChat.jsx 批任务壳)原样保留;本组件是新对话入口,路由并存
//(/coach-ai),首页默认入口的切换在批次 4 后段接 Home。
// 逻辑全部在 src/lib/coach-chat.js 纯函数(组件只渲染);SSE 用 lib/api-chat.js;
// 确认卡片数据由 proposalToCard 供形——未经确认写入为零(AI 三红线)。
// 文案直接中文(fork-only 文件,托管中文主线;locale 键化在批次 6 统一处理,
// check-locales 不扫本目录)。
import { useEffect, useRef, useState } from 'react'
import { sendChat, streamChatEvents, confirmProposal, fetchCoachUsage } from '../../lib/api-chat.js'
import { initialState, applyEvent, proposalToCard, runChatTurn, decideLocally } from '../../lib/coach-chat.js'
import Icon from '../../components/Icon.jsx'
import { exName } from '../../lib/coach.js'
import '../../coach.css'

export default function CoachChatHome({ initialChat }) {
  // 会话恢复(2026-11-18 批次 B):chatId 落 sessionStorage,切路由/刷新后续聊
  // 同一会话而非静默开新——服务端 PG 里历史与约束的延续才不断。消息流仍只在
  // 内存(历史回填属后续端点)。
  const CHAT_ID_KEY = 'muhe-coach-chat-id'
  const [chat, setChat] = useState(initialChat || initialState)
  const [input, setInput] = useState('')
  const [usage, setUsage] = useState(null)
  const chatId = useRef('')
  const endRef = useRef(null)
  const streamAbort = useRef(null)               // 当前回合 SSE 中断器(卸载即断)
  const [deciding, setDeciding] = useState(null) // 决断中禁用按钮,防双击双回执
  const busy = chat.phase === 'streaming'

  // 卸载即断流(2026-11-18 审查轮:切路由/锁屏后旧连接悬着,后端已能剔除死订阅,
  // 前端也要主动断);chatId 留在 sessionStorage 供重进续聊。
  useEffect(() => () => streamAbort.current?.abort(), [])

  useEffect(() => {
    const saved = sessionStorage.getItem(CHAT_ID_KEY)
    if (saved) chatId.current = saved
    fetchCoachUsage().then(setUsage).catch(() => {}) // 用量条失败静默:不挡对话
  }, [])
  useEffect(() => {
    endRef.current?.scrollIntoView?.({ behavior: 'smooth' })
  }, [chat.messages.length, chat.proposals.length])

  // 2026-11-18 审查轮三处补牢:① AbortController 断流;② 流干净结束但没收到
  // done 帧(代理掐断/写失败)→ 归位 phase,输入框不再永久锁死;③ cap 判断改读
  // e.data.code(api() 的错误形状不含顶层 code,旧判断是死分支,cap 文案从未透出)。
  async function send() {
    const text = input.trim()
    if (!text || busy) return
    setInput('')
    setChat(c => applyUserMessage(c, text))
    const ctl = typeof AbortController === 'function' ? new AbortController() : null
    streamAbort.current = ctl
    let id = chatId.current
    let sawDone = false
    try {
      const r = await sendChat({ id: id || undefined, message: text })
      chatId.current = r.id
      sessionStorage.setItem(CHAT_ID_KEY, r.id) // 重进续聊同一会话(批次 B)
      id = r.id
      await streamChatEvents(r.id, ev => {
        if (ev && ev.kind === 'done') sawDone = true
        setChat(c => applyEvent(c, ev))
      }, ctl ? ctl.signal : undefined)
      if (!sawDone) setChat(c => ({ ...c, phase: c.proposals.length ? 'waiting' : 'idle' }))
    } catch (e) {
      if (e && e.name === 'AbortError') return // 卸载中断:组件已走,无需善后
      // off(503)/cap(429)/网络:统一 system 提示,cap 用服务端原文(明日再来话术)。
      const capped = e && (e.data?.code || e.code) === 'cap'
      const line = capped && e.message ? e.message : 'AI 服务暂时不可用,稍后再试。'
      setChat(c => ({
        ...c,
        messages: [...c.messages, { role: 'system', text: line }],
        phase: 'idle',
      }))
    } finally {
      streamAbort.current = null
    }
  }

  // 决断:404 = 服务端已决断,以本地清理为准;网络/5xx 保留卡片并提示重试
  // (2026-11-18 审查轮:此前吞掉一切错误,弱网下的"已应用"是假回执)。
  async function decide(p, decision) {
    if (deciding) return
    setDeciding(p.id)
    try {
      await confirmProposal(chatId.current, p.id, decision)
      setChat(c => decideLocally(c, p.id, decision))
    } catch (e) {
      if (e && e.status === 404) {
        setChat(c => decideLocally(c, p.id, decision))
      } else {
        setChat(c => ({ ...c, messages: [...c.messages, { role: 'system', text: '操作未送达,请重试。' }] }))
      }
    } finally {
      setDeciding(null)
    }
  }

  return (
    <div className="narrow chat">
      <div className="chat-hdr">
        <div className="chat-av"><Icon name="sparkles" /></div>
        <div className="grow">
          <b>沐禾教练</b>
          <div className="chat-st">{usage ? `今日 ${usage.day.messages}/${usage.limits.messagesPerDay} 条` : ''}</div>
        </div>
      </div>

      <div className="chat-body">
        {!chat.messages.length && (
          <div className="chat-empty">你的 AI 教练。问计划、记偏好、聊训练——任何写入都要你确认。</div>
        )}
        {chat.messages.map((m, i) => (
          <div key={i} className={'bub ' + m.role}>
            <div>{m.text}</div>
            {!!(m.tools && m.tools.length) && <div className="chat-tools">查了: {m.tools.join(', ')}</div>}
            {!!(m.refs && m.refs.length) && (
              <div className="chips-row" data-testid="action-refs">
                {m.refs.map(id => (
                  <a key={id} className="chip" href={`#/library?ex=${id}`} onClick={e => e.stopPropagation()}>
                    {exName(id)} <span className="muted small">#{id}</span>
                  </a>
                ))}
              </div>
            )}
          </div>
        ))}
        {chat.proposals.map(p => {
          const card = proposalToCard(p)
          return (
            <div key={p.id} className="bub assistant" data-testid="proposal-card">
              <b>{card.title}</b>
              <div>{card.text}</div>
              <div className="chat-st">{card.confirm}</div>
              <div className="chips-row">
                <button onClick={() => decide(p, 'accept')} disabled={!!deciding}>应用</button>
                <button onClick={() => decide(p, 'reject')} disabled={!!deciding}>拒绝</button>
              </div>
            </div>
          )
        })}
        <div ref={endRef} />
      </div>

      <div className="composer">
        <input
          className="composer-in"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) send() }}
          placeholder="和教练聊聊…"
          disabled={busy}
          aria-label="消息"
        />
        <button className="composer-go" onClick={send} disabled={busy || !input.trim()} aria-label="发送">
          {busy ? '…' : '→'}
        </button>
      </div>
    </div>
  )
}
