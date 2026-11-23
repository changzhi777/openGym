// coach-chat.js — 对话流状态归并纯函数(批次 4;fork 第 3 条硬约束:逻辑必有单测)。
//
// SSE 六事件 → 对话流状态的唯一归并入口(组件只渲染,不藏逻辑)。
// 状态形状:
//   { messages: [{ role: 'user'|'assistant'|'system', text, tools?: [名], proposal?: id }],
//     proposals: [{ id, tool, input }],           // 待决提议(确认卡片数据源)
//     phase: 'idle' | 'streaming' | 'waiting' }   // waiting = 有待决提议
//
// 归并语义(与契约六事件一一对应):
//   user 消息由 applyUserMessage 注入(不经 SSE);token = 助手回复整段(v1 先审后
//   流,回复一次性到达);tool_call = 助手消息加工具标注(只记名,不回显参数);
//   proposal = 待决提议入列;blocked = system 提示(拦截,无正文);done = 定稿。

export function initialState() {
  return { messages: [], proposals: [], phase: 'idle' }
}

// applyUserMessage 用户发出一条消息:入列 + 进 streaming。
export function applyUserMessage(state, text) {
  const t = (text || '').trim()
  if (!t) return state
  return {
    ...state,
    messages: [...state.messages, { role: 'user', text: t }],
    proposals: state.proposals,
    phase: 'streaming',
  }
}

// applyEvent 归并一个 SSE 事件,返回新状态(纯,不改入参)。
export function applyEvent(state, ev) {
  const kind = ev && ev.kind
  const data = ev && ev.data
  switch (kind) {
    case 'token': {
      const text = (data && data.text) || ''
      if (!text) return state
      const msgs = state.messages.slice()
      const last = msgs[msgs.length - 1]
      if (last && last.role === 'assistant' && last.streaming) {
        msgs[msgs.length - 1] = { ...last, text: last.text + text }
      } else {
        msgs.push({ role: 'assistant', text, streaming: true })
      }
      return { ...state, messages: msgs }
    }
    case 'tool_call': {
      const name = (data && data.tool) || ''
      const refs = (data && data.refs) || []
      const msgs = state.messages.slice()
      const last = msgs[msgs.length - 1]
      const at = last && last.role === 'assistant' ? msgs.length - 1 : msgs.length
      const base = at === msgs.length ? { role: 'assistant', text: '' } : msgs[at]
      msgs[at] = {
        ...base,
        tools: [...(base.tools || []), name].filter(Boolean),
        refs: refs.length ? [...new Set([...(base.refs || []), ...refs])] : base.refs,
      }
      return { ...state, messages: msgs }
    }
    case 'proposal': {
      // input 随事件透传(2026-11-18 审查轮:确认卡片必须显示将写入的内容,
      // 盲确认不是确认;缺省 {} 保持旧事件形状兼容)。
      const p = { id: (data && data.proposalId) || '', tool: (data && data.tool) || '', input: (data && data.input) || {} }
      const msgs = state.messages.slice()
      const last = msgs[msgs.length - 1]
      const base = last && last.role === 'assistant' ? last : { role: 'assistant', text: '' }
      if (last && last.role === 'assistant') {
        msgs[msgs.length - 1] = { ...base, proposal: p.id }
      } else {
        msgs.push({ role: 'assistant', text: '', proposal: p.id })
      }
      return { ...state, messages: msgs, proposals: [...state.proposals, p], phase: 'waiting' }
    }
    case 'confirm_result': {
      // 决断已由 confirm 端点单独应答;流内此帧作清列信号(批次 B 起真会发出,
      // 服务端带 proposalId——多标签同会话时另一端据此单删,不误清新提议)。
      const pid = data && data.proposalId
      const rest = pid ? state.proposals.filter(p => p.id !== pid) : []
      const cleared = rest.length === 0
      return { ...state, proposals: rest, phase: state.phase === 'waiting' && cleared ? 'idle' : state.phase }
    }
    case 'blocked': {
      const reason = (data && data.reason) || (typeof data === 'string' ? data : 'sensitive')
      const lines = {
        sensitive: '这条回复没有通过内容检查,已拦截。',
        llm: '这条回复没有通过复核,已拦截。',
        llm_error: '复核服务暂时不可用,这条回复没有发送。',
        upstream: 'AI 服务暂时不可用,稍后再试。',
        memory: '记忆服务暂时不可用,稍后再试。',
      }
      return {
        ...state,
        messages: [...state.messages, { role: 'system', text: lines[reason] || lines.sensitive }],
        phase: 'idle',
      }
    }
    case 'done': {
      // 定稿:助手消息去掉流式标记;有待决提议 → waiting,否则 idle。
      const hasPending = state.proposals.length > 0
      const msgs = state.messages.map(m => (m.streaming ? { ...m, streaming: false } : m))
      return { ...state, messages: msgs, phase: hasPending ? 'waiting' : 'idle' }
    }
    default:
      return state // confirm_ready 等预留事件:不认识就不动
  }
}

// runChatTurn 一个对话回合的完整编排(依赖注入,可离线单测——组件是薄壳,
// 2026-11-18 批次 C 收编:组件不再内联平行实现,测试保护的就是线上路径)。
// 流程:入列用户消息 → sendChat 拿会话 → streamEvents 逐事件归并 →
// 无 done 帧的干净 EOF 归位 phase(代理掐断场景)→ 错误映射(cap 读
// e.data?.code||e.code——api() 的错误形状无顶层 code)→ 返回终态。
// deps:{ sendChat(body), streamEvents(id, onEvent, signal), signal?,
//         onChatId?(id) } — 组件传 api-chat 真实现与中断信号;测试传桩。
export async function runChatTurn(state, userText, deps) {
  let next = applyUserMessage(state, userText)
  let id = null
  try {
    const r = await deps.sendChat({ id: next.chatId || undefined, message: userText })
    id = r && r.id
    if (id && deps.onChatId) deps.onChatId(id)
    let sawDone = false
    await deps.streamEvents(id, ev => {
      if (ev && ev.kind === 'done') sawDone = true
      next = applyEvent(next, ev)
    }, deps.signal)
    if (!sawDone) {
      next = { ...next, phase: next.proposals.length ? 'waiting' : 'idle' }
    }
    return { ...next, chatId: id }
  } catch (e) {
    if (e && e.name === 'AbortError') throw e // 卸载中断:调用方自行忽略
    const capped = e && ((e.data && e.data.code) || e.code) === 'cap'
    const line = capped && e.message ? e.message : 'AI 服务暂时不可用,稍后再试。'
    return {
      ...next,
      chatId: id || next.chatId,
      messages: [...next.messages, { role: 'system', text: line }],
      phase: 'idle',
    }
  }
}

// decideLocally 决断后的本地状态清理(服务端 confirm 由调用方另行请求):
// 移除该提议;清空时回 idle 并落一行回执文案。
export function decideLocally(state, proposalId, decision) {
  // 重复决断(双击/重放)原样返回,不追加第二条回执(2026-11-18 审查轮)。
  if (!state.proposals.some(p => p.id === proposalId)) return state
  const rest = state.proposals.filter(p => p.id !== proposalId)
  const cleared = rest.length === 0
  return {
    ...state,
    proposals: rest,
    messages: cleared
      ? [...state.messages.map(m => (m.streaming ? { ...m, streaming: false } : m)), { role: 'system', text: decision === 'accept' ? '已应用。' : '已拒绝,不写入。' }]
      : state.messages,
    phase: cleared ? 'idle' : 'waiting',
  }
}

// proposalToCard 提议 → 确认卡片数据(kind 决定图标/文案;text 是用户可读主体)。
export function proposalToCard(p) {
  const text = (p && p.input && p.input.text) || ''
  switch (p && p.tool) {
    case 'propose_constraint':
      return { kind: 'constraint', title: '记住一条硬约束', text, confirm: '约束将立即生效于所有计划' }
    case 'propose_memory':
      return { kind: 'memory', title: '记住一条偏好', text, confirm: '写入后可在设置中查看/删除' }
    case 'propose_change':
      return { kind: 'change', title: '修改训练计划', text: p.input && p.input.summary ? p.input.summary : text, confirm: '应用后计划即刻更新' }
    default:
      return { kind: 'unknown', title: '提议', text, confirm: '确认后生效' }
  }
}

// memorySummary 双类记忆 → 展示行(供 Settings「Coach 记住了什么」页与卡片复用)。
export function memorySummary(narrative, constraints) {
  const line = e => ({ id: e.id, kind: e.kind, text: e.text, at: e.at })
  return {
    narrative: (narrative || []).map(line),
    constraints: (constraints || []).map(line),
    total: (narrative || []).length + (constraints || []).length,
  }
}
