// lib/admin/stepUp.js — 管理后台 step-up(二次短信验证)框架(fork,认证重构后)
//
// 设计 §5.4:全局 ensureStepUp —— 验证成功自动重放原请求一次;403 step-up-required 兜底;
// 并发敏感操作单弹窗去重。apiAdmin() 包装:拦截该 403 → 弹一次验证 → 原样重放。
// 验证 = 管理员绑定手机号 + 短信码(POST /api/admin/step-up;Go 侧 SmsStore 核验开窗)。
// UI 由 AdminShell 注入 setStepUpVerifier(弹 StepUpSheet);未注入时敏感操作直接报错,
// 不再回落 passkey(passkey 已随认证重构退役)。
import { api } from '../api.js'

let inflight = null   // 单弹窗去重(§5.4)
let verifier = null   // UI 验证器(弹窗),AdminShell 注入;lib 不依赖 React

export const setStepUpVerifier = fn => { verifier = fn }

// 敏感操作统一入口:非 step-up 403 直接透传;是则验证一次并重放一次(不无限循环)。
export async function apiAdmin(path, opts = {}, _retried = false) {
  try {
    return await api(path, opts)
  } catch (e) {
    const needStepUp = e?.status === 403 && (e?.data?.code === 'step-up-required' || e?.data?.code === 'passkey-required')
    if (!needStepUp || _retried) throw e
    if (!verifier) throw new Error('敏感操作需要二次验证，但验证弹窗未就绪')
    inflight = inflight || verifier()
    try { await inflight } finally { inflight = null }
    return apiAdmin(path, opts, true)
  }
}
