// views/Login.rework.test.jsx — 认证重构后的登录页(手机验证码 / 微信扫码;批F)。
// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

// api.js 的具名导出逐个 mock —— Login 调 smsSend(phone, captchaId, code),
// 不能像旧版那样把所有函数都指到同一个 path 判定的 vi.fn 上。
const mocks = vi.hoisted(() => ({
  smsSend: vi.fn(),
  smsVerify: vi.fn(),
  wechatQrcode: vi.fn(),
  wechatPoll: vi.fn(),
  bindPhone: vi.fn(),
  authCaptcha: vi.fn(),
  user: null,
  config: { invite_only: false },
  toast: vi.fn(),
}))
vi.mock('../lib/api.js', () => ({
  api: vi.fn(),
  smsSend: mocks.smsSend,
  smsVerify: mocks.smsVerify,
  wechatQrcode: mocks.wechatQrcode,
  wechatPoll: mocks.wechatPoll,
  bindPhone: mocks.bindPhone,
  authCaptcha: mocks.authCaptcha,
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
  vi.clearAllMocks()          // 清跨用例的调用记录(mockClear,不清 implementation)
  host = document.createElement('div')
  document.body.appendChild(host)
  mocks.config.invite_only = false
  mocks.authCaptcha.mockResolvedValue({ captchaId: 'cp1', svg: '<svg/>' })
})
afterEach(() => {
  act(() => {})
  host.remove()
})
const renderLogin = async () => {
  await act(async () => { createRoot(host).render(<Login />) })
  await act(async () => {})   // flush 挂载后的 captcha 拉取
}
const btn = label => [...document.querySelectorAll('button')].find(b => b.textContent.trim() === label)
const click = async el => {
  if (!el) return
  await act(async () => { el.dispatchEvent(new MouseEvent('click', { bubbles: true })) })
  await act(async () => {})   // flush 异步 onClick 链
}
const type = async (el, v) => await act(async () => {
  const setter = Object.getOwnPropertyDescriptor(el.constructor.prototype, 'value').set
  setter.call(el, v)
  el.dispatchEvent(new Event('input', { bubbles: true }))
})
const fillPhoneCaptcha = async () => {
  await type(document.querySelector('input[placeholder="请输入手机号"]'), '13800138000')
  await type(document.querySelector('input[placeholder="输入 4 位数字"]'), '1234')
  await click(btn('发送短信验证码'))
}

describe('Login 认证重构(手机/微信)', () => {
  it('双 tab 可切换:手机表单 ↔ 微信扫码表单', async () => {
    mocks.wechatQrcode.mockResolvedValue({ ticket: 't1', image: 'AAAA' })
    await renderLogin()
    expect(document.body.textContent).toContain('图形验证码')
    expect(document.body.textContent).toContain('微信扫码登录')
    await click(btn('微信扫码登录'))
    expect(document.body.textContent).toContain('等待扫码中…')
  })

  it('手机验证码流:发码 → 验证 → setUser 登录', async () => {
    mocks.smsSend.mockResolvedValue({ ok: true })
    mocks.smsVerify.mockResolvedValue({ user: { id: 'u1', name: '健身者8888' }, isNew: true })
    await renderLogin()
    // 图形验证码先出(SVG 数字码,发短信的前置人机闸)
    expect(document.body.textContent).toContain('图形验证码')
    await fillPhoneCaptcha()
    expect(mocks.smsSend).toHaveBeenCalledWith('13800138000', 'cp1', '1234')
    // 发码成功 → 6 格分段验证码解锁(enabled),填满后登录
    const digitBoxes = [...document.querySelectorAll('input[maxlength="1"]')]
    expect(digitBoxes.length).toBe(6)
    for (let i = 0; i < 6; i++) await type(digitBoxes[i], String(i + 1))
    await click(btn('登录 / 注册'))
    expect(mocks.smsVerify).toHaveBeenCalledWith('13800138000', '123456', '')
  })

  it('邀请制实例:邀请码留空被前端拦截', async () => {
    mocks.config.invite_only = true
    mocks.smsSend.mockResolvedValue({ ok: true })
    mocks.smsVerify.mockResolvedValue({ user: { id: 'u2', name: 'n' }, isNew: true })
    await renderLogin()
    await fillPhoneCaptcha()
    const boxes = [...document.querySelectorAll('input[maxlength="1"]')]
    for (const [i, b] of boxes.entries()) await type(b, String(i + 1))
    // 邀请码留空 → 前端闸门拦下:不出 verify 请求,邀请区红字提示
    await click(btn('登录 / 注册'))
    expect(mocks.smsVerify).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('需要邀请码')
    expect(document.body.textContent).toContain('本应用为邀请制')
  })
})
