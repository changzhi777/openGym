// team-load.js — 计划分发装载纯函数(fork;团队模式 W10 快照中转的学员侧)。
// 语义真源:muhe-fit android-app lib/TeamLoad.kt;复用判定对齐 sheets.jsx loadStarterPlan
// 的 sameLift 惯例——同名同练习复用现有条目,否则快照原样追加;现有条目永不触碰。

/** 待接收判定:distributed 且未被版本链取代(重分发后旧行挂 supersededBy)。 */
export function isPending(a) {
  return a?.status === 'distributed' && a?.supersededBy == null
}

/** 计划名:分发行 planName,缺失回退快照内 name。 */
export function planNameOf(a) {
  if (a?.planName) return a.planName
  const p = a?.plan
  return p && typeof p === 'object' && typeof p.name === 'string' ? p.name : ''
}

/**
 * 装载快照 routines 进现有 routines:同名同练习复用(sameLift),否则快照原样追加。
 * merged = 装载后的完整 routines;added = 需新写入的条目(复用的不在内)。
 */
export function mergePlanRoutines(current, planRoutines) {
  const merged = [...current]
  const added = []
  for (const r of planRoutines) {
    if (!merged.some(have => sameLift(have, r))) {
      merged.push(r)
      added.push(r)
    }
  }
  return { merged, added }
}

/** 复用判定:同名 + ex 数组等长且逐项动作 id 全同(loadStarterPlan sameLift 语义)。 */
export function sameLift(a, b) {
  if (!a || !b || a.name !== b.name) return false
  const ax = Array.isArray(a.ex) ? a.ex : null
  const bx = Array.isArray(b.ex) ? b.ex : null
  if (!ax && !bx) return true
  if (!ax || !bx || ax.length !== bx.length) return false
  return ax.every((x, i) => x?.id === bx[i]?.id)
}

/** 扫描文本 → 邀请码:QR 内容 {origin}/?team={code} 解析;裸码(无 scheme/空格/问号)直认手输。 */
export function inviteCodeFromScanned(text) {
  if (!text) return null
  const marker = 'team='
  const i = text.indexOf(marker)
  if (i < 0) {
    const t = text.trim()
    return t && !t.includes('://') && !t.includes('?') && !t.includes(' ') ? t : null
  }
  const code = text.slice(i + marker.length).split('&')[0]
  let decoded = code
  try { decoded = decodeURIComponent(code) } catch { /* bad escape → keep raw */ }
  return decoded || null
}
