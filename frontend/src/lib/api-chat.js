// api-chat.js — AI-Native 对话面七端点封装(批次 4,fork-only 文件,上游 api.js 零改动)。
//
// JSON 端点复用 lib/api.js 的 api()(同源 cookie/超时/错误形状全套既有语义);
// SSE 流端点用原生 fetch——api() 强制 JSON 解析,不适配 text/event-stream。
// 纯函数 parseSSEFrame 单独导出:流解析逻辑可离线单测(fork 第 3 条硬约束)。
import { api, appBase } from './api.js'

// parseSSEFrame 把一段流文本并进缓冲,吐出完整事件帧。SSE 帧形:
// `event: token\ndata: {"kind":"token","text":"…"}\n\n`(帧间空行分隔)。
// data 解析失败保留原串(不吞事件),由调用方按 kind 分派。
export function parseSSEFrame(buffer, incoming) {
  let buf = (buffer || '') + (incoming || '')
  const events = []
  for (;;) {
    const end = buf.indexOf('\n\n')
    if (end < 0) break
    const frame = buf.slice(0, end)
    buf = buf.slice(end + 2)
    let kind = 'message'
    let data = null
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) kind = line.slice(6).trim()
      else if (line.startsWith('data:')) data = line.slice(5).trim()
    }
    if (data) {
      try { data = JSON.parse(data) } catch { /* keep string */ }
      events.push({ kind, data })
    }
  }
  return { events, rest: buf }
}

// POST /api/coach/chat — 发起/续聊,回 { id, state }。
export function sendChat(body) {
  return api('/api/coach/chat', { method: 'POST', body: JSON.stringify(body) })
}

// POST /api/coach/chat/{id}/confirm — 提议决断(Agent 唯一写入口)。
export function confirmProposal(chatId, proposalId, decision) {
  return api(`/api/coach/chat/${chatId}/confirm`, { method: 'POST', body: JSON.stringify({ proposalId, decision }) })
}

// GET /api/coach/memory — 双类记忆(叙事 n*/约束 c*)。
export function fetchCoachMemory() {
  return api('/api/coach/memory')
}

export function updateMemoryEntry(id, text) {
  return api(`/api/coach/memory/${id}`, { method: 'PUT', body: JSON.stringify({ text }) })
}

export function deleteMemoryEntry(id) {
  return api(`/api/coach/memory/${id}`, { method: 'DELETE' })
}

// GET /api/coach/usage — 三池日/月聚合 + 限额。
export function fetchCoachUsage() {
  return api('/api/coach/usage')
}

// streamChatEvents 订阅一个回合的事件流(onEvent 逐事件回调;done 帧后自然结束;
// 404/503 等抛错,形状与 api() 一致)。signal 可取消(组件卸载)。
export async function streamChatEvents(chatId, onEvent, signal) {
  const res = await fetch(appBase().replace(/\/$/, '') + `/api/coach/chat/${chatId}/events`, {
    signal,
    headers: { accept: 'text/event-stream' },
  })
  if (!res.ok) {
    let body = {}
    try { body = await res.json() } catch { /* 非 JSON(如代理页) */ }
    const e = new Error(body.error || 'HTTP ' + res.status)
    e.status = res.status
    e.code = body.code
    throw e
  }
  const reader = res.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    const { events, rest } = parseSSEFrame(buf, dec.decode(value, { stream: true }))
    buf = rest
    for (const ev of events) {
      onEvent(ev)
      if (ev.kind === 'done') return
    }
  }
}
