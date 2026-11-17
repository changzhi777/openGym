// lib/admin/stepUp.js — 管理后台 step-up(敏感操作二次验证)框架(fork,P1)
//
// 设计 §5.4:全局 ensureStepUp —— 验证成功自动重放原请求一次;403 step-up-required
// 兜底;并发敏感操作单弹窗去重。
// 2026-11-18 审查轮:passkey 仪式已随认证重构(2026-10-29)退役,Go 栈 step-up =
// 短信码(POST /api/admin/step-up {phone, code};phone 必须已绑定当前管理员,
// 码先经 GET /api/auth/captcha + POST /api/auth/sms/send 领取)。弹窗验证器由
// AdminShell 注入(setStepUpVerifier);管理页都渲染在 AdminShell 内,实际路径
// 总有 UI——未注入时明确报错,绝不回落已死的 passkey 流(Go 栈 501)。
import { api } from '../api.js'

let inflight = null   // 单弹窗去重(§5.4)
let verifier = null   // UI 验证器(弹窗),AdminShell 注入;lib 不依赖 React

export const setStepUpVerifier = fn => { verifier = fn }

const defaultVerify = async () => {
  throw new Error('step-up 需要管理后台界面(短信验证码),请在管理台内操作')
}

// ensureStepUp 跑一次验证(并发去重);成功 resolve,取消/失败 reject。
export async function ensureStepUp() {
  inflight = inflight || (verifier ? verifier() : defaultVerify())
  try { await inflight } finally { inflight = null }
}

// 敏感操作统一入口:非 step-up 403 直接透传;是则验证一次并重放一次(不无限循环)。
// Go RequireStepUp → 403 code=passkey-required(码名沿用历史口径,仪式已是短信)。
export async function apiAdmin(path, opts = {}, _retried = false) {
  try {
    return await api(path, opts)
  } catch (e) {
    const needStepUp = e?.status === 403 && (e?.data?.code === 'passkey' || e?.data?.code === 'passkey-required')
    if (!needStepUp || _retried) throw e
    await ensureStepUp()
    return apiAdmin(path, opts, true)
  }
}
