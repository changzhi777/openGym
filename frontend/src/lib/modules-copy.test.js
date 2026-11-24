import { describe, expect, it } from 'vitest'
import { MODULES, GROUPS, PRESETS } from './modules.js'
import { moduleCopy, groupCopy, presetCopy } from './modules-copy.js'
import { t } from './i18n.js'

describe('modules-copy', () => {
  it('feeds t() exactly the English modules.js keeps, and shows what modules.js keeps (the search matches on it)', () => {
    const m = moduleCopy(), g = groupCopy(), p = presetCopy()
    // moduleCopy 必须把 modules.js 的同一串原文交给 t()(check-source-strings 才看得见);
    // 渲染值经过 i18n-core 的品牌单点替换(openGym→Muhe Fit),与 raw 对比时同样走 t()。
    for (const x of MODULES) expect(m[x.id]).toEqual([t(x.title), t(x.desc)])
    for (const x of GROUPS) expect(g[x.id]).toBe(t(x.title))
    for (const x of PRESETS) expect(p[x.id]).toEqual([t(x.title), t(x.desc)])
    expect(Object.keys(m)).toHaveLength(MODULES.length)
  })
})
