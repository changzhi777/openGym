// TeamTab.test.jsx — 教练团队 tab 渲染冒烟(linkedom + createRoot,同 view 测试栈)。
// mock api-team / store;分发与看板聚合逻辑在服务端与 lib 层已覆盖——本文件只锁
// 「状态 → 渲染」接线:空态建团/邀请码卡/成员行/分发选单/看板成员卡(verdict+flags)。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { parseHTML } from 'linkedom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const m = vi.hoisted(() => ({
  getTeam: vi.fn(),
  createTeam: vi.fn(),
  renameTeam: vi.fn(),
  mintInvite: vi.fn(),
  revokeInvite: vi.fn(),
  teamMembers: vi.fn(),
  kickMember: vi.fn(),
  transferTeam: vi.fn(),
  teamPlans: vi.fn(),
  distributePlan: vi.fn(),
  revokePlan: vi.fn(),
  teamBoard: vi.fn(),
  teamInsights: vi.fn(),
  update: vi.fn(),
}))
vi.mock('../../lib/api-team.js', () => m)
vi.mock('../../store/useStore.js', () => ({
  useStore: vi.fn(selector => selector ? selector({ S: { routines: [{ id: 'r1', name: '推', ex: [{ id: 'bench' }] }] } }) : { S: { routines: [] } }),
}))
vi.mock('../../components/QrCanvas.jsx', () => ({
  default: ({ value }) => <div className="qr-canvas" data-value={value} />,
}))

import TeamTab from './TeamTab.jsx'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

function mount() {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  if (!globalThis.location) globalThis.location = new URL('http://localhost:5173/')
  const host = parsed.window.document.getElementById('root')
  act(() => { createRoot(host).render(<TeamTab />) })
  return host
}
const flush = () => act(async () => { await Promise.resolve() })

beforeEach(() => { vi.clearAllMocks() })
afterEach(() => {
  delete globalThis.document
  delete globalThis.window
  vi.restoreAllMocks()
})

describe('TeamTab', () => {
  it('无团:建团入口', async () => {
    m.getTeam.mockRejectedValue({ status: 404, code: 'team-none' })
    const host = mount()
    await flush()
    expect(host.textContent).toContain('创建团队')
  })

  it('owner:团名 + 邀请码空态 + 成员行 + 看板成员卡(verdict/flags/12 周条形)', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: '一班' }, role: 'owner' })
    m.teamMembers.mockResolvedValue({ role: 'owner', members: [
      { uid: 'coach1', role: 'owner', displayName: '教练甲' },
      { uid: 'stu1', role: 'member', displayName: '沐友5479' },
    ] })
    m.teamPlans.mockResolvedValue({ pending: 1, plans: [
      { id: 'p1', status: 'distributed', supersededBy: null, planName: '推腿拉', memberDisplayName: '沐友5479', plan: {} },
    ] })
    const card = { uid: 'stu1', displayName: '沐友5479', weekDone: 2, weekPlanned: 1, streakWeeks: 2, weekDone: 2, trendVolume4w: 10200, trendEst1rm: 116.7, weekly: [0, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 2] }
    m.teamBoard.mockResolvedValue({ live: 1, weeks: [], rows: [card] })
    m.teamInsights.mockResolvedValue({ rows: [{ uid: 'stu1', rate: 200, verdict: '本周 2/1，计划完成，连续 2 周', flags: ['hot', 'gain'] }] })
    const host = mount()
    await flush()
    expect(host.textContent).toContain('一班')
    expect(host.textContent).toContain('生成邀请码')
    expect(host.textContent).toContain('沐友5479')
    expect(host.textContent).toContain('待接收')
    expect(host.textContent).toContain('推腿拉')
    expect(host.textContent).toContain('计划完成')
    expect(host.textContent).toContain('全勤')
    expect(host.textContent).toContain('1RM↑')
    expect(host.querySelectorAll('.cc-spark rect').length).toBe(12)
  })

  it('邀请码生成后:码大字 + QR 载荷带 team= 参数', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: '一班' }, role: 'owner' })
    m.teamMembers.mockResolvedValue({ role: 'owner', members: [] })
    m.teamPlans.mockResolvedValue({ pending: 0, plans: [] })
    m.teamBoard.mockResolvedValue({ live: 0, weeks: [], rows: [] })
    m.teamInsights.mockResolvedValue({ rows: [] })
    m.mintInvite.mockResolvedValue({ code: 'ABCD-EF23', expires: Date.now() + 15 * 60000 })
    const host = mount()
    await flush()
    const btn = [...host.querySelectorAll('button')].find(b => b.textContent.includes('生成邀请码'))
    await act(async () => { btn.click(); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('ABCD-EF23')
    const qr = host.querySelector('.qr-canvas')
    expect(qr?.getAttribute('data-value')).toContain('team=ABCD-EF23')
  })

  it('member 身份:引导去应用页', async () => {
    m.getTeam.mockResolvedValue({ team: { id: 't1', name: '一班' }, role: 'member' })
    const host = mount()
    await flush()
    expect(host.textContent).toContain('学员')
    expect(host.textContent).not.toContain('生成邀请码')
  })
})
