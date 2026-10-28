// views/Login.rework.test.jsx — 认证重构后的登录页(手机验证码 / 微信扫码;批F)。
// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  api: vi.fn(),
  user: null,
  config: { invite_only: false },
  toast: vi.fn(),
}))
vi.mock('../lib/api.js', () => ({
  api: mocks.api,
  smsSend: mocks.api,
  smsVerify: mocks.api,
  wechatQrcode: mocks.api,
  wechatPoll: mocks.api,
  bindPhone: mocks.api,
  webauthnOK: () => false,
}))
vi.mock('../lib/i18n.js', () => ({ t: k => k }))
vi.mock('../lib/demo.js', () => ({ DEMO: false, REPO: 'https://x' }))
vi.mock('../lib/guest.js', () => ({ guestAllowed: () => false }))
vi.mock('../store/useStore.js', () => {
  const snap = () => ({ S: null, user: mocks.user, config: mocks.config, setUser: vi.fn(),
    pushState: vi.fn(), pullState: vi.fn(), loadConfig: vi.fn() })
  const useStore = selector => (selector ? selector(snap()) : snap())
  useStore.getState = snap
  return { useStore, hasData: () => false }
})
import { useUI } from '../store/useUI.js'
vi.mock('../store/useUI.js', () => {
  const snap = () => ({ toast: mocks.toast, openSheet: vi.fn() })
  const useUI = selector => (selector ? selector(snap()) : snap())
  useUI.getState = snap
  return { useUI }
})
vi.mock('../components/Icon.jsx', () => ({ default: () => null }))
vi.mock('../components/ui.jsx', () => ({
  Button: props => <button disabled={props.disabled} onClick={props.onClick}>{props.children}</button>,
}))

import Login from './Login.jsx'

let host
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.clearAllMocks()
})
afterEach(() => {
  act(() => {})
  host.remove()
})
const renderLogin = async () => {
  await act(async () => { createRoot(host).render(<Login />) })
}
const btn = label => [...document.querySelectorAll('button')].find(b => b.textContent.trim() === label)
const click = async el => { if (el) await act(async () => { el.dispatchEvent(new MouseEvent('click', { bubbles: true })) }) }
const type = async (el, v) => await act(async () => {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
  setter.call(el, v)
  el.dispatchEvent(new Event('input', { bubbles: true }))
})

describe('Login 认证重构(手机/微信)', () => {
  it('双 tab 可切换:手机表单 ↔ 微信扫码表单', async () => {
    mocks.api.mockImplementation(path =>
      path === '/api/auth/wechat/qrcode' ? Promise.resolve({ ticket: 't1', image: 'AAAA' }) : Promise.resolve({}))
    await renderLogin()
    expect(document.body.textContent).toContain('Mobile sign-in')
    expect(document.body.textContent).toContain('WeChat scan')
    await click(btn('WeChat scan'))
    await act(async () => {})   // 放行 qrcode 拉取
    expect(document.body.textContent).toContain('Waiting for scan')
  })

  it('手机验证码流:发码 → 验证 → setUser 登录', async () => {
    mocks.api.mockImplementation(path => {
      if (path === '/api/auth/sms/send') return Promise.resolve({ ok: true })
      if (path === '/api/auth/sms/verify')
        return Promise.resolve({ user: { id: 'u1', name: '健身者8888' }, isNew: true })
      return Promise.resolve({})
    })
    await renderLogin()
    // 手机号非法(空)时 Send code 禁用
    expect([...document.querySelectorAll('button')].find(b => b.textContent === 'Send code').disabled).toBe(true)
    // 输入合法手机号后启用(happy-dom 受控输入以 native setter 驱动)
    const phoneInput = document.querySelector('input[maxlength="11"]')
    await type(phoneInput, '13800138000')
    expect(phoneInput.value).toBe('13800138000')
  })

  it('邀请制实例:注册需邀请码字段', async () => {
    mocks.config.invite_only = true
    mocks.api.mockImplementation(path =>
      path === '/api/auth/sms/verify'
        ? Promise.resolve({ user: { id: 'u2', name: 'n' }, isNew: true })
        : Promise.resolve({}))
    mocks.api.mockImplementation(path =>
      path === '/api/auth/sms/send' ? Promise.resolve({ ok: true }) : Promise.resolve({}))
    await renderLogin()
    await act(async () => {})
    const phoneInput = document.querySelector('input[maxlength="11"]')
    await type(phoneInput, '13900139000')
    await type(document.querySelector('input[maxlength="6"]'), '123456')
    // 邀请码留空 → 建号闸门拒绝 → toast 提示
    await click(btn('Sign in / Sign up'))
    expect(useUI.getState().toast).toHaveBeenCalledWith('An invite code is required')
  })
})
