// lib/admin/stepUp 的契约(2026-11-18 批次 A 短信化后):403 passkey-required
// 只触发一次验证,验证成功才重放原请求;验证本体 = AdminShell 注入的弹窗
// (图形码→发码→验码,内部打 /api/admin/step-up)——lib 只管去重与重放闸门。
// 无 UI 注入时明确报错,绝不回落已死的 passkey 流。
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../api.js', () => ({
  api: vi.fn(),
  passkeyAssertion: vi.fn(), // 已退役;仅保持 mock 形状防误引
}))

import { api, passkeyAssertion } from '../api.js'
import { apiAdmin, setStepUpVerifier, ensureStepUp } from './stepUp.js'

const forbidden = (code) => Object.assign(new Error('forbidden'), { status: 403, data: { code } })

describe('apiAdmin step-up', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setStepUpVerifier(null) // 清模块态:验证器不跨用例泄漏
  })

  it('a step-up-required 403 verifies once and replays the request', async () => {
    const verifier = vi.fn().mockResolvedValue()
    setStepUpVerifier(verifier)
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockResolvedValueOnce({ deleted: true })       // replay
    const out = await apiAdmin('/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
    expect(out).toEqual({ deleted: true })
    expect(verifier).toHaveBeenCalledTimes(1)
    expect(passkeyAssertion).not.toHaveBeenCalled()   // passkey 流已死,不得回落
    expect(api).toHaveBeenCalledTimes(2)              // original + replay
    expect(api).toHaveBeenNthCalledWith(2, '/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
  })

  it('a failed verification does not replay, and the error is the one that stopped it', async () => {
    const stop = new Error('验证码错误')
    setStepUpVerifier(vi.fn().mockRejectedValue(stop))
    api.mockRejectedValueOnce(forbidden('passkey-required'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toBe(stop)
    expect(api).toHaveBeenCalledTimes(1)              // original only; no replay
  })

  it('403s that are not about the step-up pass straight through, with no prompt', async () => {
    const verifier = vi.fn().mockResolvedValue()
    setStepUpVerifier(verifier)
    api.mockRejectedValueOnce(forbidden('role'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(verifier).not.toHaveBeenCalled()
    expect(api).toHaveBeenCalledTimes(1)
  })

  it('a second 403 after the replay is not a new round of prompt-and-retry', async () => {
    const verifier = vi.fn().mockResolvedValue()
    setStepUpVerifier(verifier)
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockRejectedValueOnce(forbidden('passkey-required'))  // replay 也 403
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(verifier).toHaveBeenCalledTimes(1)
    expect(api).toHaveBeenCalledTimes(2)
  })

  it('no verifier UI injected → clear error, no retry loop (批次 A:不回落死掉的 passkey 流)', async () => {
    api.mockRejectedValueOnce(forbidden('passkey-required'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' }))
      .rejects.toThrow(/管理后台界面/)
    expect(api).toHaveBeenCalledTimes(1)
    expect(passkeyAssertion).not.toHaveBeenCalled()
  })

  it('ensureStepUp dedupes concurrent prompts (并发敏感操作单弹窗)', async () => {
    const verifier = vi.fn().mockResolvedValue()
    setStepUpVerifier(verifier)
    const [a, b] = await Promise.all([ensureStepUp(), ensureStepUp()])
    expect(a).toBe(b)
    expect(verifier).toHaveBeenCalledTimes(1)
  })
})
