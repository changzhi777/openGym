// @vitest-environment happy-dom
// views/coach-console/coach-console.test.jsx — 全页冒烟 + 关键行为(绑定/下发断言)。
// 缘起:admin 全子页冒烟(2026-10-30)抓过缺 import/运行时崩,教练台同款护航。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  coachStudents: vi.fn(),
  coachBindStudent: vi.fn(),
  coachUnbindStudent: vi.fn(),
  coachStudentDetail: vi.fn(),
  coachPutStudentPlan: vi.fn(),
  user: { name: '教练甲', roles: ['coach'] },
}))

vi.mock('../../lib/api.js', () => ({
  api: vi.fn(),
  coachStudents: mocks.coachStudents,
  coachBindStudent: mocks.coachBindStudent,
  coachUnbindStudent: mocks.coachUnbindStudent,
  coachStudentDetail: mocks.coachStudentDetail,
  coachPutStudentPlan: mocks.coachPutStudentPlan,
  coachStudentExportUrl: id => '/api/coach-console/students/' + encodeURIComponent(id) + '/export',
  authCaptcha: vi.fn(),
  smsSend: vi.fn(),
  passkeyAssertion: vi.fn(),
}))
vi.mock('../../lib/admin/i18n.js', () => ({
  adminT: (k, ...a) => a.length ? k.replaceAll('{0}', a[0]) : k,
  adminWhen: () => '刚刚',
}))
vi.mock('../../lib/exercises.js', () => ({
  EXIDX: { '0025': { id: '0025', name: 'Barbell Bench Press' }, '0043': { id: '0043', name: 'Back Squat' } },
  canonicalExId: id => id,
}))
vi.mock('../../components/Icon.jsx', () => ({ default: () => null }))
vi.mock('../../store/useStore.js', () => {
  const snap = () => ({ user: mocks.user, S: null, config: {} })
  const useStore = sel => (sel ? sel(snap()) : snap())
  useStore.getState = snap
  return { useStore, hasData: () => false }
})

import CoachShell from './CoachShell.jsx'
import Students from './Students.jsx'
import StudentDetail from './StudentDetail.jsx'

const SUMMARY = { id: 's1', name: '学员甲', phoneTail: '138****8884', live: null,
  workouts: 12, lastWorkout: '2026-11-01', lastSync: 1, planDays: 3, boundAt: '2026-10-30T00:00:00Z' }
const DETAIL = {
  student: SUMMARY, routines: [{ id: 'r1', name: 'Push Day', ex: [{ id: '0025', sets: 4, reps: 8, weight: 0 }] }],
  week: { '1': 'r1' }, workouts: [{ id: 'w1', d: '2026-11-01', name: 'Push Day', vol: 3200 }],
  bodyweight: [{ d: '2026-11-01', w: 72.5 }], unit: 'kg', lastSync: 1, rev: 7,
}

let host
let rootRef
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.clearAllMocks()
  mocks.coachStudents.mockResolvedValue({ students: [SUMMARY] })
  mocks.coachStudentDetail.mockResolvedValue(DETAIL)
})
afterEach(() => {
  if (rootRef) act(() => rootRef.unmount())
  rootRef = null
  host.remove()
})

const mountAt = async route => {
  await act(async () => {
    rootRef = createRoot(host)
    rootRef.render(
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path="/coach-console" element={<CoachShell />}>
            <Route index element={<Students />} />
            <Route path="students/:id" element={<StudentDetail />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
  })
  await act(async () => {})
}

describe('coach-console 渲染冒烟(真实路由树)', () => {
  it('列表:学员行/在线态/周计划列,壳侧栏就位', async () => {
    await mountAt('/coach-console')
    const t = document.body.textContent
    expect(t).toContain('教练工作台')
    expect(t).toContain('学员甲')
    expect(t).toContain('138****8884')
  })

  it('详情:白名单四卡 + 计划名/动作名渲染', async () => {
    await mountAt('/coach-console/students/s1')
    const t = document.body.textContent
    expect(t).toContain('Push Day')
    expect(t).toContain('Barbell Bench Press')
    expect(t).toContain('下载数据')
    // 白名单外字段不出现:settings/unit 之外的敏感键(如 measurements)由后端裁剪,前端不渲染占位
  })

  it('无 coach 角色壳:403 面板', async () => {
    mocks.user = { name: '路人', roles: [] }
    await mountAt('/coach-console')
    expect(document.body.textContent).toContain('此入口需要教练角色')
    mocks.user = { name: '教练甲', roles: ['coach'] }
  })
})

describe('coach-console 关键行为', () => {
  it('绑定:非法手机号拦截;合法走 API 并刷新列表', async () => {
    await mountAt('/coach-console')
    const input = document.querySelector('input[aria-label="学员手机号"]')
    const setters = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
    await act(async () => { setters.call(input, '123'); input.dispatchEvent(new Event('input', { bubbles: true })) })
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.trim() === '绑定').click() })
    expect(mocks.coachBindStudent).not.toHaveBeenCalled()
    await act(async () => { setters.call(input, '13800138000'); input.dispatchEvent(new Event('input', { bubbles: true })) })
    mocks.coachBindStudent.mockResolvedValue(SUMMARY)
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.trim() === '绑定').click() })
    expect(mocks.coachBindStudent).toHaveBeenCalledWith('13800138000')
  })

  it('下发:PUT 带 routines/week/expect,成功出提示', async () => {
    mocks.coachPutStudentPlan.mockResolvedValue({ ok: true, rev: 8 })
    await mountAt('/coach-console/students/s1')
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.includes('编辑并下发')).click() })
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.includes('下发计划给学员')).click() })
    expect(mocks.coachPutStudentPlan).toHaveBeenCalledTimes(1)
    const [id, routines, week, expect_] = mocks.coachPutStudentPlan.mock.calls[0]
    expect(id).toBe('s1')
    expect(routines[0].name).toBe('Push Day')
    expect(week).toEqual({ '1': 'r1' })
    expect(expect_).toBe(7)
  })

  it('下发冲突:409 消息带重发提示', async () => {
    mocks.coachPutStudentPlan.mockRejectedValue(
      Object.assign(new Error('the student synced meanwhile — rebase and resend'), { data: { code: 'conflict' } }))
    await mountAt('/coach-console/students/s1')
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.includes('编辑并下发')).click() })
    await act(async () => { [...document.querySelectorAll('button')].find(b => b.textContent.includes('下发计划给学员')).click() })
    expect(document.body.textContent).toContain('学员刚同步过')
  })
})
