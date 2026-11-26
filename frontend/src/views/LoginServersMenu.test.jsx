// LoginServersMenu.test.jsx — 服务器选择菜单冒烟(linkedom + createRoot,CoachMemory 模式):
// 展开列表 / 当前标记已在且禁用 / 点其他服务器走 location.replace 并记录 last / 自定义添加与删除。
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { parseHTML } from 'linkedom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const replace = vi.fn()
const store = new Map()
const fakeStorage = {
  getItem: k => store.get(k) ?? null,
  setItem: (k, v) => store.set(k, v),
}
const fakeLocation = { origin: 'http://10.10.10.7:3131', href: 'http://10.10.10.7:3131/', replace }

beforeEach(() => {
  store.clear()
  replace.mockClear()
  vi.stubGlobal('location', fakeLocation)
  vi.stubGlobal('localStorage', fakeStorage)
})
afterEach(() => {
  delete globalThis.document
  delete globalThis.window
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

import LoginServersMenu from './LoginServersMenu.jsx'

async function renderMenu() {
  const parsed = parseHTML('<!doctype html><html><body><div id="root"></div></body></html>')
  globalThis.window = parsed.window
  globalThis.document = parsed.window.document
  const container = parsed.window.document.getElementById('root')
  const root = createRoot(container)
  await act(async () => root.render(<LoginServersMenu />))
  return {
    container,
    win: parsed.window,
    text: () => container.textContent,
    buttons: () => [...container.querySelectorAll('button')],
    input: () => container.querySelector('input'),
    click: async el => act(async () => el.click()),
    cleanup: () => act(async () => root.unmount()),
  }
}

describe('LoginServersMenu', () => {
  it('渲染当前服务器行(默认 183 局域网=当前 origin)', async () => {
    const h = await renderMenu()
    expect(h.text()).toContain('API 服务器')
    expect(h.text()).toContain('183 测试版（局域网）')
    expect(h.text()).toContain('切换')
    await h.cleanup()
  })

  it('展开后列出三预置,当前标记(已在)且禁用', async () => {
    const h = await renderMenu()
    await h.click(h.buttons().find(b => b.textContent.includes('切换')))
    expect(h.text()).toContain('生产正式版')
    const here = h.buttons().find(b => b.textContent.includes('已在'))
    expect(here).toBeTruthy()
    expect(here.disabled).toBe(true)
    await h.cleanup()
  })

  it('点其他服务器走 location.replace 且记录 last', async () => {
    const h = await renderMenu()
    await h.click(h.buttons().find(b => b.textContent.includes('切换')))
    await h.click(h.buttons().find(b => b.textContent.includes('生产正式版')))
    expect(replace).toHaveBeenCalledWith('https://app.qingmulife.com/')
    expect(store.get('muhe_last_login_server')).toBe('https://app.qingmulife.com')
    await h.cleanup()
  })

  it('自定义添加:无效地址报错且不跳转(有效添加的纯函数行为由 login-servers.test 覆盖)', async () => {
    const h = await renderMenu()
    await h.click(h.buttons().find(b => b.textContent.includes('切换')))
    const input = h.input()
    // React 受控 input:必须走原生 value setter + input 事件(linkedom 环境已知约束)
    const setVal = v => {
      const setter = Object.getOwnPropertyDescriptor(h.win.HTMLInputElement.prototype, 'value').set
      setter.call(input, v)
      input.dispatchEvent(new h.win.Event('input', { bubbles: true }))
    }
    setVal('no host.')
    await act(async () => {})          // flush 受控 input 的 setState 重渲染
    await h.click(h.buttons().find(b => b.textContent === '添加'))
    expect(h.text()).toContain('地址无效')
    expect(replace).not.toHaveBeenCalled()
    await h.cleanup()
  })

  it('自定义项可删除(预置无删除钮)', async () => {
    store.set('muhe_login_servers', JSON.stringify([{ name: '自定义', url: 'http://z.example' }]))
    const h = await renderMenu()
    await h.click(h.buttons().find(b => b.textContent.includes('切换')))
    const del = h.buttons().find(b => (b.getAttribute('aria-label') || '').includes('z.example'))
    expect(del).toBeTruthy()
    await h.click(del)
    expect(h.buttons().filter(b => (b.getAttribute('aria-label') || '').includes('z.example'))).toHaveLength(0)
    await h.cleanup()
  })
})
