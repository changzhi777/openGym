// @vitest-environment happy-dom
// views/admin/Roles.test.jsx — 角色页行为(批1):403 分支/成员表/授予流(设计 §2.5/§3.10/§3.12)。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({ api: vi.fn(), apiAdmin: vi.fn(), role: 'super_admin' }))
vi.mock('../../lib/api.js', () => ({ api: mocks.api }))
vi.mock('../../lib/admin/stepUp.js', () => ({ apiAdmin: mocks.apiAdmin }))
vi.mock('../../lib/admin/i18n.js', () => ({ adminT: k => k, adminWhen: () => '刚刚' }))
vi.mock('../../lib/mobile.js', () => ({ MOBILE: false }))
vi.mock('react-router-dom', () => ({
  Link: props => <a href={props.to}>{props.children}</a>,
  NavLink: props => <a href={props.to}>{props.children}</a>,
  useOutletContext: () => ({ role: mocks.role, readonly: mocks.role === 'support' }),
}))

import Roles from './Roles.jsx'

const ROLES = {
  roles: [
    { user_id: 'u1', name: '常智', role: 'super_admin', granted_by: '', granted_by_name: '', granted_at: 1 },
    { user_id: 'u2', name: '张伟', role: 'ops', granted_by: 'u1', granted_by_name: '常智', granted_at: 2 },
    { user_id: 'u3', name: '李娜', role: 'support', granted_by: 'u1', granted_by_name: '常智', granted_at: 3 },
  ],
}

let host
let rootRef
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  mocks.role = 'super_admin'
  vi.clearAllMocks()
})
afterEach(() => {
  if (rootRef) act(() => rootRef.unmount())
  rootRef = null
  host.remove()
})

const renderRoles = async () => {
  await act(async () => { rootRef = createRoot(host); rootRef.render(<Roles />) })
}
const btn = label => [...document.querySelectorAll('button')].find(b => b.textContent === label)
const click = async el => { if (el) await act(async () => { el.dispatchEvent(new MouseEvent('click', { bubbles: true })) }) }

describe('Roles 权限闸门', () => {
  it('support 打开页面:403 面板,不拉角色数据', async () => {
    mocks.role = 'support'
    await renderRoles()
    expect(document.body.textContent).toContain('此页面仅超级管理员可见')
    expect(document.body.textContent).toContain('客服(只读)')
    expect(mocks.api).not.toHaveBeenCalled()
  })

  it('super_admin:统计头 + 成员表 + 超管行操作列为 —', async () => {
    mocks.api.mockResolvedValue(ROLES)
    await renderRoles()
    expect(document.body.textContent).toContain('成员 3 · 超级管理员 1 · 运营 1 · 客服(只读) 1')
    expect(document.body.textContent).toContain('张伟')
    expect(document.body.textContent).toContain('超级管理员只能通过服务器配置调整')
    expect(document.body.textContent).toContain('变更角色为客服(只读)')
  })
})

describe('Roles 授予流', () => {
  it('弹窗:选用户选角色 → apiAdmin grant', async () => {
    mocks.api.mockImplementation(path => {
      if (path === '/api/admin/roles') return Promise.resolve(ROLES)
      if (path === '/api/admin/users')
        return Promise.resolve({ users: [{ id: 'u9', name: '王强', email: '' }, { id: 'u2', name: '张伟', email: '' }] })
      return Promise.resolve({})
    })
    mocks.apiAdmin.mockResolvedValue({ ok: true })
    await renderRoles()
    await act(async () => {}) // 放行 roles load
    await click(btn('授予管理员'))
    expect(document.body.textContent).toContain('选择用户')
    await click([...document.querySelectorAll('.adm-picklist button')].find(b => b.textContent.includes('王强')))
    await click(document.querySelector('.adm-rolepick'))
    await click(btn('授予角色'))
    expect(mocks.apiAdmin).toHaveBeenCalledTimes(1)
    expect(mocks.apiAdmin.mock.calls[0][0]).toBe('/api/admin/roles/grant')
    expect(JSON.parse(mocks.apiAdmin.mock.calls[0][1].body)).toEqual({ id: 'u9', role: 'ops' })
  })
})
