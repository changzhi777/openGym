// PlanTeamCard.test.jsx — 计划页团队同步卡冒烟(linkedom + createRoot,同 view 测试栈)。
// mock api-team / store / UI;装载语义(mergePlanRoutines)由 team-load.test.js 覆盖——
// 本文件锁「状态 → 渲染」接线:pending 接收卡/在团低调行/owner 与无团不渲染/接收委派。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { parseHTML } from 'linkedom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const m = vi.hoisted(() => ({
  getTeam: vi.fn(),
  joinTeam: vi.fn(),
  leaveTeam: vi.fn(),
  teamPlans: vi.fn(),
  ackPlan: vi.fn(),
  update: vi.fn(),
  toast: vi.fn(),
  nav: vi.fn(),
}))
vi.mock('../../lib/api-team.js', () => ({
  getTeam: m.getTeam, joinTeam: m.joinTeam, leaveTeam: m.leaveTeam,
  teamPlans: m.teamPlans, ackPlan: m.ackPlan,
}))
vi.mock('../../store/useStore.js', () => ({
  useStore: vi.fn(selector => selector ? selector({ update: m.update }) : { update: m.update }),
}))
vi.mock('../../store/useUI.js', () => ({
  useUI: vi.fn(selector => selector ? selector({ toast: m.toast }) : { toast: m.toast }),
}))

import PlanTeamCard from './PlanTeamCard.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

// 固定分发行(P10 语义:distributed + 无 supersededBy = pending)
const ASSIGN = {
  id: 'a1', status: 'distributed', supersededBy: null, planName: 'PPL-Week4',
  plan: { name: 'PPL', routines: [{ name: 'Push Day', ex: [{ id: 'ex1' }] }] },
}

function mount(el) {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  const host = parsed.window.document.getElementById('root')
  document.body.appendChild(host)
  const root = createRoot(host)
  act(() => { root.render(el) })
  return {
    host,
    text: () => host.textContent,
    buttons: () => [...host.querySelectorAll('button')],
    unmount: () => act(() => root.unmount()),
  }
}

async function render() {
  const h = mount(<PlanTeamCard nav={m.nav} />)
  await act(async () => {}) // 放行 useTeamPlans 的 refresh 异步链
  return h
}

beforeEach(() => {
  vi.clearAllMocks()
  m.getTeam.mockResolvedValue({ team: { id: 't1', name: 'kiri 团' }, role: 'member' })
})
afterEach(() => {
  document.body.innerHTML = ''
  delete globalThis.document
  delete globalThis.window
})

describe('PlanTeamCard', () => {
  it('pending>0:接收卡渲染计划名/条目数/团名', async () => {
    m.teamPlans.mockResolvedValue({ pending: 1, plans: [ASSIGN] })
    const h = await render()
    expect(h.text()).toContain('教练发来了新计划')
    expect(h.text()).toContain('PPL-Week4')
    expect(h.text()).toContain('kiri 团')
    expect(h.text()).toContain('个计划条目')
    await h.unmount()
  })

  it('pending>0:接收/婉拒按钮就位(busy 态禁用;ackPlan 委派为 lib 薄委派由评审保证)', async () => {
    m.teamPlans.mockResolvedValue({ pending: 1, plans: [ASSIGN] })
    const h = await render()
    expect(h.buttons().find(b => b.textContent === '接收')).toBeTruthy()
    expect(h.buttons().find(b => b.textContent === '婉拒')).toBeTruthy()
    await h.unmount()
  })

  it('在团无 pending:低调行显示团名并可跳 /team', async () => {
    m.teamPlans.mockResolvedValue({ pending: 0, plans: [] })
    const h = await render()
    expect(h.text()).toContain('教练')
    expect(h.text()).toContain('kiri 团')
    const btn = h.buttons().find(b => b.textContent.includes('计划同步'))
    await act(async () => { btn.click() })
    expect(m.nav).toHaveBeenCalledWith('/team')
    await h.unmount()
  })

  it('owner/无团:不渲染', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: 'own' }, role: 'owner' })
    let h = await render()
    expect(h.text()).toBe('')
    await h.unmount()
    m.getTeam.mockRejectedValue({ status: 404 })
    h = await render()
    expect(h.text()).toBe('')
    await h.unmount()
  })
})
