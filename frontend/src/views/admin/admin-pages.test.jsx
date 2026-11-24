// @vitest-environment happy-dom
// views/admin/admin-pages.test.jsx — 全子页渲染冒烟:每页在真实路由树下挂载不崩。
// 缘起(2026-10-30 三连修):Invites.jsx 缺 import / UserDetail created 崩这类雷,
// 页面不上生产没人打开就不暴露——冒烟让它们在 CI 就炸。
// 挂载方式:MemoryRouter + 完整 /admin 路由树(同 App.jsx 结构),api 按 path 给
// 最小真实形状;不 mock stepUp.js/sheets.jsx,保持真实装配链。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
// happy-dom 无 EventSource;Overview 的 live SSE 用最小 stub(happy 域下收不到事件,
// 只需构造/监听/close 不崩——生产浏览器有原生实现,不受此影响)。
globalThis.EventSource = class EventSource {
  addEventListener() {}
  removeEventListener() {}
  close() {}
}

const mocks = vi.hoisted(() => ({ api: vi.fn(), apiAdmin: vi.fn(), authCaptcha: vi.fn() }))

vi.mock('../../lib/api.js', () => ({
  api: mocks.api,
  apiAdmin: mocks.apiAdmin,
  authCaptcha: mocks.authCaptcha,
  smsSend: vi.fn(),
  passkeyAssertion: vi.fn(),
}))
vi.mock('../../lib/admin/i18n.js', () => ({ adminT: k => k, adminWhen: () => '刚刚' }))
vi.mock('../../lib/mobile.js', () => ({ MOBILE: false }))
vi.mock('../../store/useStore.js', () => {
  const snap = () => ({ user: { admin: true, name: 'cz' }, S: null, config: {},
    setUser: vi.fn(), pushState: vi.fn(), pullState: vi.fn(), loadConfig: vi.fn() })
  const useStore = sel => (sel ? sel(snap()) : snap())
  useStore.getState = snap
  return { useStore, hasData: () => false }
})
import { useUI } from '../../store/useUI.js'
vi.mock('../../store/useUI.js', () => {
  const snap = () => ({ toast: vi.fn(), openSheet: vi.fn() })
  const useUI = sel => (sel ? sel(snap()) : snap())
  useUI.getState = snap
  return { useUI }
})
vi.mock('../../components/Icon.jsx', () => ({ default: () => null }))

import AdminShell from './AdminShell.jsx'
import Overview from './Overview.jsx'
import Users from './Users.jsx'
import UserDetail from './UserDetail.jsx'
import Audit from './Audit.jsx'
import Roles from './Roles.jsx'
import Invites from './Invites.jsx'
import Placeholder from './Placeholder.jsx'
import AdminCoach from '../AdminCoach.jsx'

const CZ = { id: 'u1', name: 'cz', created: '2026-10-29T07:16:36Z', disabled: false, admin: false }
const apiShapes = () => {
  mocks.api.mockImplementation(path => {
    if (path === '/api/admin/session') return Promise.resolve({ role: 'super_admin' })
    if (path.startsWith('/api/admin/overview')) return Promise.resolve({ users: 1, workouts: 0 })
    if (path.startsWith('/api/admin/users')) return Promise.resolve({
      users: [{ ...CZ, email: null, role: null, workouts: 0, lastWorkout: null, lastSync: null, hasPush: false, hasPhone: true, hasWx: false }],
      total: 1, today_new: 1, disabled_count: 0, page: 1, per_page: 25, invite_only: false, now: Date.now(),
    })
    if (path.startsWith('/api/admin/user?')) return Promise.resolve({ user: CZ, workouts: [], bodyweight: [] })
    if (path.startsWith('/api/admin/audit')) return Promise.resolve({
      events: [{ id: 1, admin: 'cz', action: 'admin.step_up', targetType: null, targetId: null, detail: { via: 'sms' }, at: Date.now(), ip: '1.2.3.4' }],
      total: 1,
    })
    if (path === '/api/admin/roles') return Promise.resolve({ roles: [] })
    if (path === '/api/admin/invites') return Promise.resolve({ invites: [], invite_only: false })
    return Promise.resolve({})
  })
  mocks.authCaptcha.mockResolvedValue({ captchaId: 'cp1', svg: '<svg/>' })
}

const PAGES = [
  ['/admin', Overview],
  ['/admin/users', Users],
  ['/admin/users/u1', UserDetail],
  ['/admin/audit', Audit],
  ['/admin/roles', Roles],
  ['/admin/invites', Invites],
  ['/admin/revenue', Placeholder],
  ['/admin/coach', AdminCoach],
]

let host
let rootRef
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  apiShapes()
  vi.clearAllMocks()
  apiShapes()   // clearAllMocks 清掉 once/implementation 后重挂分发
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
          <Route path="/admin" element={<AdminShell />}>
            {PAGES.map(([sub, El]) => (
              sub === '/admin'
                ? <Route key="idx" index element={<El />} />
                : <Route key={sub} path={sub.replace('/admin/', '')} element={<El />} />
            ))}
          </Route>
        </Routes>
      </MemoryRouter>,
    )
  })
  await act(async () => {})   // flush 数据加载
}

describe('admin 全子页渲染冒烟(真实路由树)', () => {
  for (const [route] of PAGES) {
    it(`${route} 挂载不崩,侧栏与内容区就位`, async () => {
      await mountAt(route)
      const t = document.body.textContent
      expect(t).toContain('管理后台')      // AdminShell 侧栏渲染 = 壳活着
      expect(t.length).toBeGreaterThan(60) // 内容区不是空白
    })
  }

  it('冒烟样例:用户列表出徽章、审计出事件、邀请码出工具条', async () => {
    await mountAt('/admin/users')
    expect(document.body.textContent).toContain('手机')            // 身份徽章链路
    await mountAt('/admin/audit')
    expect(document.body.textContent).toContain('admin.step_up')   // 审计事件渲染
    await mountAt('/admin/invites')
    expect(document.body.textContent).toContain('创建邀请码')       // 工具条(缺 import 会整页崩)
  })
})
