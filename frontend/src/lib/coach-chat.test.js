// coach-chat.test.js — 对话状态归并纯函数单测(六事件 × 状态机全覆盖)。
import { describe, it, expect } from 'vitest'
import { initialState, applyUserMessage, applyEvent, proposalToCard, memorySummary, runChatTurn, decideLocally } from './coach-chat.js'

const ev = (kind, data) => ({ kind, data })

describe('applyUserMessage', () => {
  it('入列 + 进 streaming', () => {
    const s = applyUserMessage(initialState(), '今天练什么')
    expect(s.messages).toEqual([{ role: 'user', text: '今天练什么' }])
    expect(s.phase).toBe('streaming')
  })
  it('空白消息原样返回(纯函数不新增态)', () => {
    const s = initialState()
    expect(applyUserMessage(s, '   ')).toBe(s)
  })
})

describe('applyEvent.token', () => {
  it('追加助手消息并标记流式', () => {
    let s = applyUserMessage(initialState(), '问')
    s = applyEvent(s, ev('token', { text: '今天练胸。' }))
    expect(s.messages[1]).toEqual({ role: 'assistant', text: '今天练胸。', streaming: true })
  })
  it('连续 token 并入同一条(v2 流式预留)', () => {
    let s = applyUserMessage(initialState(), '问')
    s = applyEvent(s, ev('token', { text: '今天' }))
    s = applyEvent(s, ev('token', { text: '练胸。' }))
    expect(s.messages[1].text).toBe('今天练胸。')
    expect(s.messages).toHaveLength(2)
  })
})

describe('applyEvent.tool_call', () => {
  it('挂到当前助手消息(只记名)', () => {
    let s = applyUserMessage(initialState(), '下周怎么练')
    s = applyEvent(s, ev('tool_call', { tool: 'get_week_plan' }))
    expect(s.messages[1].tools).toEqual(['get_week_plan'])
    expect(s.messages[1].text).toBe('')
  })
})

describe('applyEvent.proposal', () => {
  it('入待决列 + 消息标注 + waiting', () => {
    let s = applyUserMessage(initialState(), '记住我喜欢哑铃')
    s = applyEvent(s, ev('proposal', { proposalId: 'p1', tool: 'propose_memory' }))
    expect(s.proposals).toEqual([{ id: 'p1', tool: 'propose_memory', input: {} }])
    expect(s.messages[1].proposal).toBe('p1')
    expect(s.phase).toBe('waiting')
  })
})

describe('applyEvent.blocked', () => {
  it('system 提示 + 回 idle + 无正文流出', () => {
    let s = applyUserMessage(initialState(), '问')
    s = applyEvent(s, ev('blocked', { reason: 'sensitive' }))
    expect(s.messages[1]).toEqual({ role: 'system', text: '这条回复没有通过内容检查,已拦截。' })
    expect(s.phase).toBe('idle')
    expect(s.messages.some(m => m.role === 'assistant')).toBe(false)
  })
  it('未知原因兜底 sensitive 文案', () => {
    const s = applyEvent(initialState(), ev('blocked', { reason: 'zzz' }))
    expect(s.messages[0].text).toContain('内容检查')
  })
})

describe('applyEvent.done', () => {
  it('定稿去流式标记,无提议回 idle', () => {
    let s = applyUserMessage(initialState(), '问')
    s = applyEvent(s, ev('token', { text: '答' }))
    s = applyEvent(s, ev('done', {}))
    expect(s.phase).toBe('idle')
    expect(s.messages[1].streaming).toBe(false)
  })
  it('有待决提议 → waiting', () => {
    let s = applyUserMessage(initialState(), '记住哑铃')
    s = applyEvent(s, ev('proposal', { proposalId: 'p1', tool: 'propose_memory' }))
    s = applyEvent(s, ev('done', {}))
    expect(s.phase).toBe('waiting')
  })
})

describe('applyEvent.confirm_result / 未知事件', () => {
  it('confirm_result 清待决列', () => {
    let s = applyEvent(initialState(), ev('proposal', { proposalId: 'p1', tool: 'propose_memory' }))
    s = applyEvent(s, ev('confirm_result', {}))
    expect(s.proposals).toEqual([])
    expect(s.phase).toBe('idle')
  })
  it('不认识的事件不动状态', () => {
    const s = initialState()
    expect(applyEvent(s, ev('ping', {}))).toBe(s)
  })
})

describe('proposalToCard', () => {
  it('约束/记忆/改计划三类卡片', () => {
    expect(proposalToCard({ tool: 'propose_constraint', input: { text: '膝伤回避深蹲' } })).toMatchObject({ kind: 'constraint', text: '膝伤回避深蹲' })
    expect(proposalToCard({ tool: 'propose_memory', input: { text: '喜欢哑铃' } })).toMatchObject({ kind: 'memory' })
    expect(proposalToCard({ tool: 'propose_change', input: { summary: '卧推 -10kg' } })).toMatchObject({ kind: 'change', text: '卧推 -10kg' })
    expect(proposalToCard({ tool: 'zzz', input: {} }).kind).toBe('unknown')
  })
})

describe('memorySummary', () => {
  it('双类合并 + 总数', () => {
    const s = memorySummary(
      [{ id: 'n1', kind: 'narrative', text: '喜欢哑铃', at: 't1' }],
      [{ id: 'c1', kind: 'constraint', text: '膝伤回避深蹲', at: 't2' }]
    )
    expect(s.total).toBe(2)
    expect(s.narrative[0].id).toBe('n1')
    expect(s.constraints[0].text).toBe('膝伤回避深蹲')
  })
  it('空输入安全', () => {
    expect(memorySummary(null, undefined)).toEqual({ narrative: [], constraints: [], total: 0 })
  })
})

describe('runChatTurn(编排,依赖注入)', () => {
  it('成功流:sendChat→streamEvents 归并→终态带 chatId', async () => {
    const deps = {
      sendChat: async () => ({ id: 'c9', state: 'active' }),
      streamEvents: async (_id, on) => {
        on(ev('token', { text: '答' }))
        on(ev('done', {}))
      },
    }
    const r = await runChatTurn(initialState(), '问', deps)
    expect(r.chatId).toBe('c9')
    expect(r.messages.map(m => m.role)).toEqual(['user', 'assistant'])
    expect(r.messages[1].text).toBe('答')
    expect(r.phase).toBe('idle')
  })
  it('cap 429:服务端原文透出 + 回 idle', async () => {
    const deps = { sendChat: async () => { const e = new Error('今日对话额度已用完,明天再来'); e.code = 'cap'; throw e }, streamEvents: async () => {} }
    const r = await runChatTurn(initialState(), '问', deps)
    expect(r.messages[r.messages.length - 1]).toEqual({ role: 'system', text: '今日对话额度已用完,明天再来' })
    expect(r.phase).toBe('idle')
  })
  it('其他错误:统一文案 + 保留用户消息', async () => {
    const deps = { sendChat: async () => { throw new Error('boom') }, streamEvents: async () => {} }
    const r = await runChatTurn(initialState(), '问', deps)
    expect(r.messages[0]).toEqual({ role: 'user', text: '问' })
    expect(r.messages[1].text).toBe('AI 服务暂时不可用,稍后再试。')
  })
})

describe('decideLocally', () => {
  const withP = () => applyEvent(applyUserMessage(initialState(), '记住哑铃'), ev('proposal', { proposalId: 'p1', tool: 'propose_memory' }))
  it('最后一项清空:回执 + idle', () => {
    const r = decideLocally(withP(), 'p1', 'accept')
    expect(r.proposals).toEqual([])
    expect(r.phase).toBe('idle')
    expect(r.messages[r.messages.length - 1].text).toBe('已应用。')
  })
  it('reject 文案不同', () => {
    const r = decideLocally(withP(), 'p1', 'reject')
    expect(r.messages[r.messages.length - 1].text).toBe('已拒绝,不写入。')
  })
})

describe('tool_call refs 归并(动作卡源,批次 6 后段)', () => {
  it('refs 挂到助手消息并去重累加', () => {
    let s = applyUserMessage(initialState(), '深蹲怎么做')
    s = applyEvent(s, ev('tool_call', { tool: 'search_knowledge', refs: ['1000', '1001'] }))
    s = applyEvent(s, ev('tool_call', { tool: 'search_knowledge', refs: ['1001', '1002'] }))
    expect(s.messages[1].refs).toEqual(['1000', '1001', '1002'])
  })
  it('无 refs 不加字段', () => {
    const s = applyEvent(initialState(), ev('tool_call', { tool: 'get_bodyweight' }))
    expect(s.messages[0].refs).toBeUndefined()
  })
})
