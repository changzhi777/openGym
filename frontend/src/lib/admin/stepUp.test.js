// lib/admin/stepUp 的契约:403 step-up-required 只触发一次验证(注入的 verifier 弹窗),
// 验证成功才重放原请求;验证失败透传验证器错误;重放后再 403 不再弹第二轮。
// 认证重构后验证本体 = 管理员绑定手机号 + 短信码(StepUpSheet → POST /api/admin/step-up);
// verifier 不经 api(),故重放才是第二次 api 调用。
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../api.js', () => ({
  api: vi.fn(),
  passkeyAssertion: vi.fn(),   // 已退役;保留 mock 以防误引
}))

import { api } from '../api.js'
import { apiAdmin, setStepUpVerifier } from './stepUp.js'

const forbidden = (code) => Object.assign(new Error('forbidden'), { status: 403, data: { code } })

describe('apiAdmin step-up', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.mockReset()   // 清 once 队列(clearAllMocks 只清调用记录,残留队列会跨用例错位)
    setStepUpVerifier(null)
  })

  it('a step-up-required 403 verifies once (injected verifier) and replays the request', async () => {
    const verify = vi.fn().mockResolvedValue()
    setStepUpVerifier(verify)
    api
      .mockRejectedValueOnce(forbidden('step-up-required'))
      .mockResolvedValueOnce({ deleted: true })       // replay
    const out = await apiAdmin('/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
    expect(out).toEqual({ deleted: true })
    expect(verify).toHaveBeenCalledTimes(1)
    expect(api).toHaveBeenCalledTimes(2)
    expect(api).toHaveBeenNthCalledWith(2, '/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
  })

  it('a failed verification surfaces the verifier error, no replay', async () => {
    setStepUpVerifier(() => Promise.reject(new Error('已取消二次验证')))
    api.mockRejectedValueOnce(forbidden('step-up-required'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toThrow('已取消二次验证')
    expect(api).toHaveBeenCalledTimes(1)   // original only; no replay
  })

  it('403s that are not about step-up pass straight through, with no prompt', async () => {
    const verify = vi.fn()
    setStepUpVerifier(verify)
    api.mockRejectedValueOnce(forbidden('role'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(verify).not.toHaveBeenCalled()
    expect(api).toHaveBeenCalledTimes(1)
  })

  it('a second 403 after the replay is not a new round of prompt-and-retry', async () => {
    const verify = vi.fn().mockResolvedValue()
    setStepUpVerifier(verify)
    api
      .mockRejectedValueOnce(forbidden('step-up-required'))
      .mockRejectedValueOnce(forbidden('step-up-required'))   // replay fails again
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(verify).toHaveBeenCalledTimes(1)
    expect(api).toHaveBeenCalledTimes(2)
  })

  it('legacy passkey-required code still triggers the same flow (backend compat)', async () => {
    const verify = vi.fn().mockResolvedValue()
    setStepUpVerifier(verify)
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockResolvedValueOnce({ deleted: true })
    const out = await apiAdmin('/api/admin/user/delete', { method: 'POST' })
    expect(out).toEqual({ deleted: true })
    expect(verify).toHaveBeenCalledTimes(1)
  })

  it('no verifier injected → clear error, no crash', async () => {
    api.mockRejectedValueOnce(forbidden('step-up-required'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' }))
      .rejects.toThrow('敏感操作需要二次验证')
  })
})
