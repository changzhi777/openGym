// login-servers.test.js — 登录页服务器选择逻辑纯函数单测(fork-only)
import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SERVERS, addServer, isCurrent, isDefaultServer, jumpTarget,
  loadServers, normUrl, removeServer, saveServers,
} from './login-servers.js'

describe('normUrl', () => {
  it('补协议并取 origin+去尾斜杠', () => {
    expect(normUrl('10.10.10.7:3131')).toBe('http://10.10.10.7:3131')
    expect(normUrl('https://app.qingmulife.com/')).toBe('https://app.qingmulife.com')
    expect(normUrl('http://a.b.c/x/')).toBe('http://a.b.c/x')
  })
  it('无效输入返回空串', () => {
    expect(normUrl('')).toBe('')
    expect(normUrl('   ')).toBe('')
    expect(normUrl('ftp://x')).toBe('')
    expect(normUrl('http://')).toBe('')
  })
})

describe('loadServers / saveServers', () => {
  it('无存储时回预置三条', () => {
    expect(loadServers(null)).toEqual(DEFAULT_SERVERS)
    expect(loadServers({ getItem: () => null })).toEqual(DEFAULT_SERVERS)
  })
  it('存储损坏回预置', () => {
    expect(loadServers({ getItem: () => '{oops' })).toEqual(DEFAULT_SERVERS)
  })
  it('自定义项拼在预置后且去重', () => {
    const store = { getItem: () => JSON.stringify([
      { url: 'http://x.example:3131' },
      { url: 'http://x.example:3131' },
      { url: DEFAULT_SERVERS[0].url },
      { bad: 1 },
    ]) }
    const v = loadServers(store)
    expect(v).toHaveLength(4)
    expect(v[3]).toEqual({ name: 'http://x.example:3131', url: 'http://x.example:3131' })
  })
  it('saveServers 写入', () => {
    const mem = new Map()
    const store = { getItem: k => mem.get(k) ?? null, setItem: (k, v) => mem.set(k, v) }
    saveServers(store, [{ name: '自定义', url: 'http://y.example' }])
    expect(loadServers(store)).toHaveLength(4)
  })
})

describe('addServer / removeServer', () => {
  it('添加自定义并自动选中', () => {
    const { servers, url } = addServer(DEFAULT_SERVERS, '192.168.1.50:3131')
    expect(url).toBe('http://192.168.1.50:3131')
    expect(servers).toHaveLength(4)
    expect(servers[3]).toEqual({ name: '自定义', url: 'http://192.168.1.50:3131' })
  })
  it('无效地址 url 为 null 且列表不变', () => {
    const { servers, url } = addServer(DEFAULT_SERVERS, 'no host.')
    expect(url).toBeNull()
    expect(servers).toBe(DEFAULT_SERVERS)
  })
  it('已存在则不重复添加', () => {
    const { servers, url } = addServer(DEFAULT_SERVERS, 'http://10.10.10.7:3131/')
    expect(url).toBe('http://10.10.10.7:3131')
    expect(servers).toBe(DEFAULT_SERVERS)
  })
  it('预置不可删,自定义可删', () => {
    expect(removeServer(DEFAULT_SERVERS, DEFAULT_SERVERS[2].url)).toBe(DEFAULT_SERVERS)
    const { servers } = addServer(DEFAULT_SERVERS, 'http://z.example')
    expect(removeServer(servers, 'http://z.example')).toEqual(DEFAULT_SERVERS)
  })
})

describe('jumpTarget', () => {
  it('保当前 pathname+search+hash', () => {
    expect(jumpTarget('http://10.10.10.7:3131', 'https://app.qingmulife.com/'))
      .toBe('http://10.10.10.7:3131/')
    expect(jumpTarget('https://app.qingmulife.com', 'http://10.10.10.7:3131/coach-console/team'))
      .toBe('https://app.qingmulife.com/coach-console/team')
    expect(jumpTarget('http://10.10.10.7:3131', 'http://10.10.10.7:3131/login?x=1#top'))
      .toBe('http://10.10.10.7:3131/login?x=1#top')
  })
  it('无效目标返回空串', () => {
    expect(jumpTarget('http://', 'https://a.example/')).toBe('')
  })
})

describe('isDefaultServer / isCurrent', () => {
  it('预置识别(容忍尾斜杠差异)', () => {
    expect(isDefaultServer('http://10.10.10.7:3131')).toBe(true)
    expect(isDefaultServer('http://10.10.10.7:3131/')).toBe(true)
    expect(isDefaultServer('http://other:1')).toBe(false)
  })
  it('当前 origin 判定', () => {
    expect(isCurrent('http://10.10.10.7:3131/', 'http://10.10.10.7:3131')).toBe(true)
    expect(isCurrent('https://app.qingmulife.com', 'http://10.10.10.7:3131')).toBe(false)
  })
})
