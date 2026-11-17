// api-chat.test.js — SSE 帧解析纯函数单测(流解析逻辑离线可测;fork 硬约束)。
import { describe, it, expect } from 'vitest'
import { parseSSEFrame } from './api-chat.js'

const frame = (kind, data) => `event: ${kind}\ndata: ${data}\n\n`

describe('parseSSEFrame', () => {
  it('解析完整帧', () => {
    const { events, rest } = parseSSEFrame('', frame('token', JSON.stringify({ kind: 'token', text: '你好' })))
    expect(events).toHaveLength(1)
    expect(events[0].kind).toBe('token')
    expect(events[0].data.text).toBe('你好')
    expect(rest).toBe('')
  })

  it('半帧留存缓冲(分片到达)', () => {
    const full = frame('proposal', JSON.stringify({ proposalId: 'p1' }))
    const half = full.slice(0, full.length - 2) // 掐掉帧尾空行
    const a = parseSSEFrame('', half)
    expect(a.events).toHaveLength(0)
    expect(a.rest).toBe(half)
    // 补上帧尾,完整吐出。
    const b = parseSSEFrame(a.rest, '\n\n')
    expect(b.events).toHaveLength(1)
    expect(b.events[0].data.proposalId).toBe('p1')
  })

  it('多帧连发一次解析', () => {
    const blob = frame('tool_call', '{"tool":"get_week_plan"}') + frame('token', '{"text":"好"}') + frame('done', '{}')
    const { events } = parseSSEFrame('', blob)
    expect(events.map(e => e.kind)).toEqual(['tool_call', 'token', 'done'])
  })

  it('坏 JSON 保留原串不吞帧', () => {
    const { events } = parseSSEFrame('', frame('blocked', 'not-json'))
    expect(events).toHaveLength(1)
    expect(events[0].data).toBe('not-json')
  })

  it('无 event 行按 message 处理', () => {
    const { events } = parseSSEFrame('', 'data: {"x":1}\n\n')
    expect(events[0].kind).toBe('message')
  })

  it('空 data 行不产生事件', () => {
    const { events } = parseSSEFrame('', 'event: ping\n\n')
    expect(events).toHaveLength(0)
  })

  it('CRLF 行终结全帧兼容(SSE 规范三种行终结,批次 C)', () => {
    const crlf = 'event: token\r\ndata: {"kind":"token","text":"答"}\r\n\r\n'
    const a = parseSSEFrame('', crlf)
    expect(a.events).toHaveLength(1)
    expect(a.events[0].kind).toBe('token')
    expect(a.events[0].data.text).toBe('答')
    expect(a.rest).toBe('')
    // 分片切在 \r 与 \n 之间也不丢帧。
    const b = parseSSEFrame('', crlf.slice(0, -3))
    const c = parseSSEFrame(b.rest, crlf.slice(-3))
    expect(c.events).toHaveLength(1)
  })

  it('多行 data 按规范 \\n 拼接(此前后行覆盖前行,批次 C)', () => {
    const { events } = parseSSEFrame('', 'data: {"a":\ndata: 1}\n\n')
    expect(events).toHaveLength(1)
    expect(events[0].data).toEqual({ a: 1 })
  })
})
