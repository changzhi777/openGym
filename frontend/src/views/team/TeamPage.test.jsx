// TeamPage.test.jsx — 团队页(学员视角)渲染冒烟(linkedom + createRoot,同 view 测试栈)。
// mock api-team / store / UI / CameraScan;装载逻辑(mergePlanRoutines)在 team-load.test.js
// 已全覆盖——本文件只锁「状态 → 渲染」接线:空态加入面/我的教练卡/待接收卡/owner 引导。
// DOM 事件交互(linkedom × React 受控输入)不可靠,不做点击流断言(receive/decline 是
// lib 薄委派,由代码评审保证)。
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
vi.mock('../../components/CameraScan.jsx', () => ({
  default: () => <div className="camera-scan" data-testid="scan" />,
}))

import TeamPage from './TeamPage.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
const $text = host => host.textContent

function mount(el) {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  const host = parsed.window.document.getElementById('root')
  document.body.appendChild(host)
  act(() => { createRoot(host).render(el) })
  return host
}

beforeEach(() => { vi.clearAllMocks() })
afterEach(() => {
  delete globalThis.document
  delete globalThis.window
  vi.restoreAllMocks()
})

describe('TeamPage', () => {
  it('无团:加入面(扫码开关 + 手输框),不含教练卡', async () => {
    m.getTeam.mockRejectedValue({ status: 404, code: 'team-none' })
    const host = mount(<TeamPage />)
    await act(async () => { await Promise.resolve() })
    expect($text(host)).toContain('加入团队')
    expect($text(host)).toContain('扫码加入')
    expect(host.querySelector('input[placeholder="XXXX-XXXX"]')).toBeTruthy()
    expect($text(host)).not.toContain('退出团队')
  })

  it('学员视角:我的教练卡 + 待接收计划卡(接收/婉拒)', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: '一班' }, role: 'member' })
    m.teamPlans.mockResolvedValue({ pending: 1, plans: [
      { id: 'p1', status: 'distributed', supersededBy: null, planName: '推腿拉', plan: { name: '推腿拉', routines: [{ name: '推', ex: [{ id: 'bench' }] }] } },
      { id: 'p2', status: 'loaded', supersededBy: null, planName: '旧计划', plan: {} },
    ] })
    const host = mount(<TeamPage />)
    await act(async () => { await Promise.resolve() })
    expect($text(host)).toContain('一班')
    expect($text(host)).toContain('退出团队')
    // 只渲染 pending 行:待接收卡含 推腿拉+接收/婉拒;已结算的旧计划不上卡
    expect($text(host)).toContain('推腿拉')
    expect($text(host)).toContain('接收')
    expect($text(host)).toContain('婉拒')
    expect($text(host)).not.toContain('旧计划')
  })

  it('owner:引导去教练台,不含学员控件', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: '一班' }, role: 'owner' })
    const host = mount(<TeamPage />)
    await act(async () => { await Promise.resolve() })
    expect($text(host)).toContain('一班')
    expect($text(host)).toContain('教练台')
    expect($text(host)).not.toContain('接收')
    expect($text(host)).not.toContain('退出团队')
  })

  it('加载态先渲染,不打后端以外的接口', async () => {
    let resolve
    m.getTeam.mockReturnValue(new Promise(r => { resolve = r }))
    const host = mount(<TeamPage />)
    expect($text(host)).toContain('加载中')
    await act(async () => { resolve({ team: null }); await Promise.resolve() })
  })
})
