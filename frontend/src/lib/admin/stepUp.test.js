// lib/admin/stepUp 的契约:403 passkey-required 只触发一次验证,验证成功才重放原请求。
// 验证本体是把 assertion 交给后端(/api/admin/step-up)——只签名不提交的话,后端无从验签。
import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('../api.js', () => ({
  api: vi.fn(),
  passkeyAssertion: vi.fn()
}))

import { api, passkeyAssertion } from '../api.js'
import { apiAdmin } from './stepUp.js'

const forbidden = (code) => Object.assign(new Error('forbidden'), { status: 403, data: { code } })

describe('apiAdmin step-up', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('a passkey-required 403 verifies once and replays the request', async () => {
    const credential = { id: 'cred-1', response: {} }
    passkeyAssertion.mockResolvedValue({ cid: 'cid-1', credential })
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockResolvedValueOnce({ ok: true })            // POST /api/admin/step-up
      .mockResolvedValueOnce({ deleted: true })       // replay
    const out = await apiAdmin('/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
    expect(out).toEqual({ deleted: true })
    expect(passkeyAssertion).toHaveBeenCalledTimes(1)
    expect(api).toHaveBeenNthCalledWith(2, '/api/admin/step-up', {
      method: 'POST', body: JSON.stringify({ cid: 'cid-1', credential })
    })
    expect(api).toHaveBeenNthCalledWith(3, '/api/admin/user/delete', { method: 'POST', body: '{"id":"u1"}' })
  })

  it('a failed verification does not replay, and the error is the one that stopped it', async () => {
    passkeyAssertion.mockResolvedValue({ cid: 'cid-2', credential: {} })
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockRejectedValueOnce(forbidden('passkey'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(api).toHaveBeenCalledTimes(2)   // original + step-up; no replay
  })

  it('403s that are not about the passkey pass straight through, with no prompt', async () => {
    api.mockRejectedValueOnce(forbidden('role'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(passkeyAssertion).not.toHaveBeenCalled()
    expect(api).toHaveBeenCalledTimes(1)
  })

  it('a second 403 after the replay is not a new round of prompt-and-retry', async () => {
    passkeyAssertion.mockResolvedValue({ cid: 'cid-3', credential: {} })
    api
      .mockRejectedValueOnce(forbidden('passkey-required'))
      .mockResolvedValueOnce({ ok: true })
      .mockRejectedValueOnce(forbidden('passkey-required'))
    await expect(apiAdmin('/api/admin/user/delete', { method: 'POST' })).rejects.toMatchObject({ status: 403 })
    expect(passkeyAssertion).toHaveBeenCalledTimes(1)
    expect(api).toHaveBeenCalledTimes(3)
  })
})
