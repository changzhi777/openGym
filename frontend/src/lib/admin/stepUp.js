// lib/admin/stepUp.js — 管理后台 step-up(二次 passkey 验证)框架(fork,P1)
//
// 设计 §5.4:全局 ensureStepUp —— 验证成功自动重放原请求一次;403 step-up-required 兜底;
// 并发敏感操作单弹窗去重。后端现实:Node(P1)不强制;Go 后端将返回 403 {code:'passkey-required'}。
// apiAdmin() 包装:拦截该 403 → 弹一次验证 → 原样重放 —— 后端就绪即无缝生效,不改调用方。
import { api, passkeyAssertion } from '../api.js'

let inflight = null   // 单弹窗去重(§5.4)
let verifier = null   // UI 验证器(弹窗),AdminShell 注入;lib 不依赖 React

export const setStepUpVerifier = fn => { verifier = fn }

// 无 UI 时退化:取挑战 → 系统认证器签名 → 把 assertion 交给后端开窗(Go /api/admin/step-up)。
// 认证器只签名不下发;不 POST 的话后端无从验签,敏感操作会再次 403。
const defaultVerify = async () => {
  const { cid, credential } = await passkeyAssertion()
  await api('/api/admin/step-up', { method: 'POST', body: JSON.stringify({ cid, credential }) })
}

// 敏感操作统一入口:非 step-up 403 直接透传;是则验证一次并重放一次(不无限循环)。
export async function apiAdmin(path, opts = {}, _retried = false) {
  try {
    return await api(path, opts)
  } catch (e) {
    const needStepUp = e?.status === 403 && (e?.data?.code === 'passkey' || e?.data?.code === 'passkey-required')
    if (!needStepUp || _retried) throw e
    inflight = inflight || (verifier ? verifier() : defaultVerify())
    try { await inflight } finally { inflight = null }
    return apiAdmin(path, opts, true)
  }
}
