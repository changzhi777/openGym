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
import '../../home-ai.css'

export default function CoachChatHome({ initialChat }) {
  const [chat, setChat] = useState(initialChat || initialState)
  const [input, setInput] = useState('')
  const [usage, setUsage] = useState(null)
  const chatId = useRef('')
  const endRef = useRef(null)
  const busy = chat.phase === 'streaming'

  useEffect(() => {
    fetchCoachUsage().then(setUsage).catch(() => {}) // 用量条失败静默:不挡对话
  }, [])
  useEffect(() => {
    endRef.current?.scrollIntoView?.({ behavior: 'smooth' })
  }, [chat.messages.length, chat.proposals.length])

  async function send() {
    const text = input.trim()
    if (!text || busy) return
    setInput('')
    setChat(c => applyUserMessage(c, text))
    let id = chatId.current
    try {
      const r = await sendChat({ id: id || undefined, message: text })
      chatId.current = r.id
      id = r.id
      await streamChatEvents(r.id, ev => setChat(c => applyEvent(c, ev)))
    } catch (e) {
      // off(503)/cap(429)/网络:统一 system 提示,cap 用服务端原文(明日再来话术)。
      const line = e.code === 'cap' && e.message ? e.message : 'AI 服务暂时不可用,稍后再试。'
      setChat(c => ({
        ...c,
        messages: [...c.messages, { role: 'system', text: line }],
        phase: 'idle',
      }))
    }
  }

  async function decide(p, decision) {
    try { await confirmProposal(chatId.current, p.id, decision) } catch { /* 404 等以本地清理为准 */ }
    setChat(c => decideLocally(c, p.id, decision))
  }

  return (
    <div className="narrow chat">
      <div className="chat-hdr">
        <div className="chat-av"><Icon name="sparkles" /></div>
        <div className="grow">
          <b>沐禾教练</b>
          <div className="chat-st">{usage ? `今日 ${usage.day.messages}/${usage.limits.messagesPerDay} 条` : ''}</div>
        </div>
        <a className="iconbtn" href="#/coach-ai/memory" aria-label="记住了什么">
          <Icon name="list" />
        </a>
      </div>

      <div className="msgs">
        {!chat.messages.length && (
          <div className="chat-empty">你的 AI 教练。问计划、记偏好、聊训练——任何写入都要你确认。</div>
        )}
        {chat.messages.map((m, i) => {
          const role = m.role === 'user' ? 'user' : m.role === 'system' ? 'sys' : 'coach'
          return (
            <div key={i} className={'msg ' + role}>
              <div className={'bub' + (role === 'sys' ? ' sys' : '')}>
                <div>{m.text}</div>
                {!!(m.tools && m.tools.length) && <div className="chat-tools">查了: {m.tools.join(', ')}</div>}
                {!!(m.refs && m.refs.length) && (
                  <div className="chips-row">
                    {m.refs.map(id => (
                      <a key={id} className="chip" href={`#/library?ex=${id}`}>
                        {exName(id)} <span className="muted small">#{id}</span>
                      </a>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )
        })}
        {chat.proposals.map(p => {
          const card = proposalToCard(p)
          return (
            <div key={p.id} className="msg coach">
              <div className="bub" data-testid="proposal-card">
                <b>{card.title}</b>
                <div style={{ marginTop: 4 }}>{card.text}</div>
                <div className="chat-st">{card.confirm}</div>
                <div className="chips-row">
                  <button onClick={() => decide(p, 'accept')}>应用</button>
                  <button onClick={() => decide(p, 'reject')}>拒绝</button>
                </div>
              </div>
            </div>
          )
        })}
        <div ref={endRef} />
      </div>

      <div className="composer">
        <div className="composer-in">
          <textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }}
            placeholder="和教练聊聊…"
            disabled={busy}
            rows={1}
            aria-label="消息"
          />
          <button className="send" onClick={send} disabled={busy || !input.trim()} aria-label="发送">
            {busy ? '…' : '→'}
          </button>
        </div>
      </div>
    </div>
  )
}
