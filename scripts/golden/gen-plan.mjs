#!/usr/bin/env node
/* gen-plan.mjs — 用 MCP get_week_plan 的**真 handler**生成 Go 侧 golden 夹具(批次 6 后段)。
 *
 * 用法(仓根):node scripts/golden/gen-plan.mjs
 * 产物:backend-go/internal/agent/testdata/plan-golden.json
 *
 * 锚定层级:连组装层一起锚定——gen 写 state JSON 到临时目录,经 OPENGYM_DATA/
 * OPENGYM_UID env + ESM query 分键独立加载 mcp/src/tools.js,调真 handler 记录
 * 全输出。8 个场景覆盖 planned_by 六分支 + queue 容错边界:
 *   weekday / coach(活跃+own ride-along) / coach 已完成进位 / pin open /
 *   pin done(读作 override) / rest_override / plain override / queue 容错。
 * 时钟钉死 FAKE_NOW(2026-11-11 周三,Date monkey-patch 在 import tools.js 前)。
 */
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, utimesSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const FAKE_NOW = new Date('2026-11-11T12:00:00')
const RealDate = Date
class FakeDate extends RealDate {
  constructor(...a) { a.length ? super(...a) : super(FAKE_NOW.getTime()) }
  static now() { return FAKE_NOW.getTime() }
}
globalThis.Date = FakeDate

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const R1 = 'r-push', R2 = 'r-pull', R3 = 'r-gone'

const baseState = {
  unit: 'kg',
  routines: [
    { id: R1, name: '推力日' },
    { id: R2, name: '拉力日' },
    { id: R3, name: '已删动作' },
  ],
  week: { 3: [R1], 5: [R2] }, // JS getDay():0=周日…6=周六 → 周三=3,周五=5
  dayPlan: {},
  workouts: [],
}

const MONDAY = new Date('2026-11-09T12:00:00').getTime() // 本周一(apply 点)

const SCENARIOS = [
  { name: 'weekday_only', state: { ...baseState } },
  {
    name: 'coach_active',
    state: { ...baseState, queue: { ids: [R1, R2], since: MONDAY, startsOn: '2026-11-09', label: 'US W1' } },
  },
  {
    name: 'coach_first_done',
    state: {
      ...baseState,
      queue: { ids: [R1, R2], since: MONDAY, startsOn: '2026-11-09', label: 'US W1' },
      workouts: [{ id: 'w1', d: '2026-11-11', start: MONDAY + 3600e3, name: '推力日', routineIds: [R1] }],
    },
  },
  {
    name: 'pin_open',
    state: {
      ...baseState,
      queue: { ids: [R1, R2], since: MONDAY, startsOn: '2026-11-09', label: 'US W1' },
      dayPlan: { '2026-11-12': R2 }, // R2 钉到周四
    },
  },
  {
    name: 'pin_done_reads_override',
    state: {
      ...baseState,
      queue: { ids: [R1, R2], since: MONDAY, startsOn: '2026-11-09', label: 'US W1' },
      workouts: [{ id: 'w1', d: '2026-11-11', start: MONDAY + 3600e3, name: '推力日', routineIds: [R1] }],
      dayPlan: { '2026-11-11': R1 }, // 已完成的 session 被钉在今天 → pin 满足,读作 override
    },
  },
  { name: 'rest_override', state: { ...baseState, dayPlan: { '2026-11-11': 'rest' } } },
  { name: 'plain_override', state: { ...baseState, dayPlan: { '2026-11-11': R2 } } },
  {
    name: 'queue_tolerant',
    state: {
      ...baseState,
      queue: { ids: [R1, R1, R3, R2] }, // 重复 id 去重;R3 不存在丢弃;无 startsOn → since 当天
      workouts: [],
    },
  },
]

// 单 DATA_DIR/单 uid:state.js 是模块级单例(DATA_DIR 首次 import 固定),但它有
// mtime 重读兜底——每场景覆写同一 state 文件并用 utimes 强制 mtime 递增,
// getState() 即自动重读(mtimeMs 毫秒分辨率,快写会撞值,必须显式递增)。
const tmp = mkdtempSync(join(tmpdir(), 'muhe-plan-gen-'))
const dataDir = join(tmp, 'data')
mkdirSync(dataDir, { recursive: true })
process.env.OPENGYM_DATA = dataDir
process.env.OPENGYM_UID = 'u'
const stateFile = join(dataDir, 'state-u.json')
const scenarios = []
try {
  for (let i = 0; i < SCENARIOS.length; i++) {
    const sc = SCENARIOS[i]
    writeFileSync(stateFile, JSON.stringify(sc.state))
    const at = new Date(FAKE_NOW.getTime() + i * 60_000) // mtime 逐场景 +1min,保重读
    utimesSync(stateFile, at, at)
    const mod = await import('../../mcp/src/tools.js')
    const out = mod.getWeekPlan.handler()
    scenarios.push({ name: sc.name, state: sc.state, output: out })
  }
} finally {
  rmSync(tmp, { recursive: true, force: true })
}

const dest = join(ROOT, 'backend-go', 'internal', 'agent', 'testdata')
mkdirSync(dest, { recursive: true })
const destFile = join(dest, 'plan-golden.json')
writeFileSync(destFile, JSON.stringify({
  _comment: 'golden 夹具:期望值由 MCP get_week_plan 真 handler 生成(gen-plan.mjs),时钟钉死 2026-11-11 周三;禁止手编',
  fakeToday: '2026-11-11',
  scenarios,
}))
console.log(`wrote ${destFile}: ${scenarios.length} scenarios (${scenarios.map(s => s.name).join(', ')})`)
