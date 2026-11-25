import { describe, expect, it } from 'vitest'
import { inviteCodeFromScanned, isPending, mergePlanRoutines, planNameOf, sameLift } from './team-load.js'

// 装载/判定纯函数保护网:语义对拍 muhe-fit TeamLoad.kt(同名同练习复用/verbatim 追加/
// 版本链 pending/QR 码解析)。
const r = (name, ids) => ({ name, ex: ids.map(id => ({ id })) })
describe('isPending', () => {
  it('distributed 且未被版本链取代才算待接收', () => {
    expect(isPending({ status: 'distributed', supersededBy: null })).toBe(true)
    expect(isPending({ status: 'distributed' })).toBe(true)
    expect(isPending({ status: 'distributed', supersededBy: 'x' })).toBe(false)
    expect(isPending({ status: 'loaded', supersededBy: null })).toBe(false)
    expect(isPending({ status: 'declined', supersededBy: null })).toBe(false)
    expect(isPending(null)).toBe(false)
    expect(isPending({})).toBe(false)
  })
})

describe('planNameOf', () => {
  it('planName 优先,缺失回退快照内 name,双缺空串', () => {
    expect(planNameOf({ planName: 'PPL', plan: { name: 'X' } })).toBe('PPL')
    expect(planNameOf({ plan: { name: '推腿拉' } })).toBe('推腿拉')
    expect(planNameOf({ plan: {} })).toBe('')
    expect(planNameOf({})).toBe('')
  })
})

describe('sameLift', () => {
  it('同名同动作序列复用', () => {
    expect(sameLift(r('推', ['a', 'b']), r('推', ['a', 'b']))).toBe(true)
  })
  it('同名不同动作数/不同 id 不复用', () => {
    expect(sameLift(r('推', ['a']), r('推', ['a', 'b']))).toBe(false)
    expect(sameLift(r('推', ['a']), r('推', ['c']))).toBe(false)
    expect(sameLift(r('推', ['a']), r('拉', ['a']))).toBe(false)
  })
  it('双无 ex 视为同(空计划),单边 ex 不复用', () => {
    expect(sameLift({ name: '休' }, { name: '休' })).toBe(true)
    expect(sameLift({ name: '休' }, r('休', ['a']))).toBe(false)
  })
})

describe('mergePlanRoutines', () => {
  const push = r('推', ['bench', 'ohp'])
  const legs = r('腿', ['squat'])
  it('快照新条目 verbatim 追加,复用条目不进 added', () => {
    const cur = [push]
    const { merged, added } = mergePlanRoutines(cur, [r('推', ['bench', 'ohp']), legs])
    expect(added).toEqual([legs])
    expect(merged).toEqual([push, legs])
    // 现有条目永不触碰:merged[0] 与 cur[0] 同引用
    expect(merged[0]).toBe(cur[0])
  })
  it('空档全量追加,顺序保持快照序', () => {
    const { merged, added } = mergePlanRoutines([], [push, legs])
    expect(merged).toEqual([push, legs])
    expect(added).toHaveLength(2)
  })
})

describe('inviteCodeFromScanned', () => {
  it('QR 链接抽 team= 参数并 URL 解码', () => {
    expect(inviteCodeFromScanned('http://192.168.1.5:3110/?team=ABCD-EF23')).toBe('ABCD-EF23')
    expect(inviteCodeFromScanned('https://x.example/?a=1&team=MELZ%2D6RZK')).toBe('MELZ-6RZK')
    expect(inviteCodeFromScanned('https://x.example/?team=ABCD-EF23&utm=x')).toBe('ABCD-EF23')
  })
  it('裸码手输直认,带 scheme/问号/空格的杂串拒绝', () => {
    expect(inviteCodeFromScanned('abcd-ef23')).toBe('abcd-ef23')
    expect(inviteCodeFromScanned(' ABCD-EF23 ')).toBe('ABCD-EF23')
    expect(inviteCodeFromScanned('http://x/?link=zz')).toBe(null)
    expect(inviteCodeFromScanned('look at this')).toBe(null)
    expect(inviteCodeFromScanned('')).toBe(null)
    expect(inviteCodeFromScanned(null)).toBe(null)
  })
})
