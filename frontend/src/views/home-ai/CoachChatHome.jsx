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
import { initialState, proposalToCard, runChatTurn, decideLocally } from '../../lib/coach-chat.js'
import Icon from '../../components/Icon.jsx'
import { exName } from '../../lib/coach.js'
import '../../coach.css'
import '../../home-ai.css'

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
  const [sending, setSending] = useState(false)  // 回合在途(批次 C:相位要等编排返回才落,须本地闸防双发)
  const busy = chat.phase === 'streaming' || sending

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

  // 2026-11-18 批次 C:send 收编回 lib 的 runChatTurn 薄委派(组件不再内联
  // 平行编排——此前 lib 有 20 案测试保护、组件那份裸奔,行为早已漂移)。
  async function send() {
    const text = input.trim()
    if (!text || busy) return
    setInput('')
    setSending(true)
    const ctl = typeof AbortController === 'function' ? new AbortController() : null
    streamAbort.current = ctl
    try {
      const final = await runChatTurn(chat, text, {
        sendChat,
        streamEvents: (id, onEvent, signal) => streamChatEvents(id, onEvent, signal),
        signal: ctl ? ctl.signal : undefined,
        onChatId: id => { chatId.current = id; sessionStorage.setItem(CHAT_ID_KEY, id) },
      })
      setChat(final)
    } catch (e) {
      /* AbortError(卸载中断):组件已走,无需善后 */
    } finally {
      streamAbort.current = null
      setSending(false)
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
                  <button onClick={() => decide(p, 'accept')} disabled={!!deciding}>应用</button>
                  <button onClick={() => decide(p, 'reject')} disabled={!!deciding}>拒绝</button>
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
