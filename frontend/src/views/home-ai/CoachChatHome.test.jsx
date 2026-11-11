// CoachChatHome.test.jsx — 对话首页渲染冒烟(linkedom + createRoot,与现有 view 测试同栈)。
// mock api-chat;编排逻辑(runChatTurn/归并)在 coach-chat.test.js 已全覆盖——
// 本文件只锁「状态 → 渲染」接线:消息流/工具标注/提议卡片/决断按钮/cap 文案。
// DOM 事件交互(linkedom × React 受控输入)不可靠,不做点击流断言——send/decide
// 是对 lib 函数的薄 async 委派,由代码评审保证。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { parseHTML } from 'linkedom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { sendChat, streamChatEvents, confirmProposal, fetchCoachUsage } = vi.hoisted(() => ({
  sendChat: vi.fn(),
  streamChatEvents: vi.fn(),
  confirmProposal: vi.fn(),
  fetchCoachUsage: vi.fn(),
}))
vi.mock('../../lib/api-chat.js', () => ({ sendChat, streamChatEvents, confirmProposal, fetchCoachUsage }))

import CoachChatHome from './CoachChatHome.jsx'
import { initialState, applyUserMessage, applyEvent } from '../../lib/coach-chat.js'

const ev = (kind, data) => ({ kind, data })
globalThis.IS_REACT_ACT_ENVIRONMENT = true

beforeEach(() => { vi.clearAllMocks() })
afterEach(() => {
  delete globalThis.document
  delete globalThis.window
  vi.restoreAllMocks()
})

async function renderHome(initialChat) {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  fetchCoachUsage.mockResolvedValue({ day: { messages: 0, chat: 0, review: 0, embedding: 0 }, limits: { messagesPerDay: 50 } })
  const container = parsed.window.document.getElementById('root')
  const root = createRoot(container)
  await act(async () => root.render(<CoachChatHome initialChat={initialChat} />))
  const fire = (el, type) => act(() => { el.dispatchEvent(new parsed.window.Event(type, { bubbles: true })) })
  return {
    container,
    fire,
    text: () => container.textContent,
    buttons: () => [...container.querySelectorAll('button')],
    cleanup: () => act(async () => root.unmount()),
  }
}

describe('CoachChatHome', () => {
  it('空态欢迎语 + usage 头显示', async () => {
    const h = await renderHome()
    expect(h.text()).toContain('你的 AI 教练')
    expect(h.text()).toContain('0/50 条')
    await h.cleanup()
  })

  it('消息流渲染:用户/助手/工具标注', async () => {
    let s = applyUserMessage(initialState(), '今天练什么')
    s = applyEvent(s, ev('tool_call', { tool: 'get_week_plan' }))
    s = applyEvent(s, ev('token', { text: '今天练胸,4 组。' }))
    s = applyEvent(s, ev('done', {}))
    const h = await renderHome(s)
    expect(h.text()).toContain('今天练什么')
    expect(h.text()).toContain('今天练胸,4 组。')
    expect(h.text()).toContain('get_week_plan')
    await h.cleanup()
  })

  it('提议卡片渲染 + 应用按钮调 confirmProposal', async () => {
    confirmProposal.mockResolvedValue({ id: 'c2', state: 'active', applied: 'memory' })
    let s = applyUserMessage(initialState(), '记住我喜欢哑铃')
    s = applyEvent(s, ev('proposal', { proposalId: 'p1', tool: 'propose_memory' }))
    s = applyEvent(s, ev('done', {}))
    const h = await renderHome(s)
    expect(h.text()).toContain('记住一条偏好')
    expect(h.text()).toContain('喜欢哑铃')
    const accept = h.buttons().find(b => b.textContent === '应用')
    expect(accept).toBeTruthy()
    await act(async () => { h.fire(accept, 'click') })
    expect(confirmProposal).toHaveBeenCalledWith('', 'p1', 'accept')
    expect(h.text()).toContain('已应用。')
    await h.cleanup()
  })

  it('system 文案(cap 拦截等)渲染', async () => {
    const s = { ...initialState(), messages: [{ role: 'system', text: '今日对话额度已用完,明天再来' }] }
    const h = await renderHome(s)
    expect(h.text()).toContain('今日对话额度已用完')
    await h.cleanup()
  })
})
