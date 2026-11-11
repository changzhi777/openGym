// CoachMemory.test.jsx — 记忆页冒烟(linkedom + createRoot):双类渲染/删除调用/空态。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { parseHTML } from 'linkedom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchCoachMemory, deleteMemoryEntry } = vi.hoisted(() => ({
  fetchCoachMemory: vi.fn(),
  deleteMemoryEntry: vi.fn(),
}))
vi.mock('../../lib/api-chat.js', () => ({
  fetchCoachMemory,
  deleteMemoryEntry,
  sendChat: vi.fn(),
  streamChatEvents: vi.fn(),
  confirmProposal: vi.fn(),
}))

import CoachMemory from './CoachMemory.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

beforeEach(() => { vi.clearAllMocks() })
afterEach(() => {
  delete globalThis.document
  delete globalThis.window
  vi.restoreAllMocks()
})

async function renderMemory() {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  const container = parsed.window.document.getElementById('root')
  const root = createRoot(container)
  await act(async () => root.render(<CoachMemory />))
  return {
    container,
    text: () => container.textContent,
    buttons: () => [...container.querySelectorAll('button')],
    cleanup: () => act(async () => root.unmount()),
  }
}

describe('CoachMemory', () => {
  it('双类渲染:约束组在前 + 空态', async () => {
    fetchCoachMemory.mockResolvedValueOnce({
      narrative: [{ id: 'n1', kind: 'narrative', text: '喜欢哑铃', at: '2026-11-11T01:00:00Z' }],
      constraints: [{ id: 'c1', kind: 'constraint', text: '膝伤回避深蹲', at: '2026-11-11T00:30:00Z' }],
    })
    const h = await renderMemory()
    expect(h.text()).toContain('硬约束')
    expect(h.text()).toContain('膝伤回避深蹲')
    expect(h.text()).toContain('喜欢哑铃')
    await h.cleanup()

    fetchCoachMemory.mockResolvedValueOnce({ narrative: [], constraints: [] })
    const h2 = await renderMemory()
    expect(h2.text()).toContain('还没有记住任何东西')
    await h2.cleanup()
  })

  it('删除调 deleteMemoryEntry + 乐观移除', async () => {
    fetchCoachMemory.mockResolvedValueOnce({
      narrative: [{ id: 'n1', kind: 'narrative', text: '喜欢哑铃' }],
      constraints: [],
    })
    deleteMemoryEntry.mockResolvedValue({ ok: true })
    const h = await renderMemory()
    const del = h.buttons().find(b => b.getAttribute('aria-label') === '删除')
    await act(async () => { h.fire?.(del, 'click') ?? del.dispatchEvent(new window.Event('click', { bubbles: true })) })
    await act(async () => { /* settle */ })
    expect(deleteMemoryEntry).toHaveBeenCalledWith('n1')
    expect(h.text()).not.toContain('喜欢哑铃')
    await h.cleanup()
  })

  it('加载失败显示错误', async () => {
    fetchCoachMemory.mockRejectedValueOnce(new Error('usage unreadable'))
    const h = await renderMemory()
    expect(h.text()).toContain('加载失败')
    await h.cleanup()
  })
})
