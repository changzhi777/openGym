// gen-onerm.mjs — 用 frontend/src/lib/onerm.js 的**真函数**生成 Go 侧 golden 夹具。
//
// 用法(仓根):node scripts/golden/gen-onerm.mjs
// 产物:backend-go/internal/agent/testdata/onerm-golden.json
//
// 纪律:夹具期望值必须来自 JS 真函数(算法单源,R3),不许手抄/手算。
// 覆盖:8 公式 × 10 重量 × 14 次数 × 4 RIR 全组合(6720 案)+ accuracy 组合(104 案),
// 含全部边界(0/负重量、0/负/超限次数、RIR null/0/2/4、cap ±1)。
import { estimate1RM, calculate1RMAccuracy } from '../../frontend/src/lib/onerm.js';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const FORMULAS = ['epley', 'brzycki', 'lombardi', 'oconner', 'mayhew', 'wathan', 'lander', 'weighted'];
const WEIGHTS  = [0, -100, 20, 40, 62.5, 80, 100, 101.25, 142.5, 200];
const REPS     = [-3, 0, 1, 2, 3, 5, 8, 10, 11, 12, 13, 15, 16, 30];
const RIRS     = [null, 0, 2, 4];

const cases = [];
for (const w of WEIGHTS) {
  for (const r of REPS) {
    for (const f of FORMULAS) {
      for (const rir of RIRS) {
        cases.push({ w, r, f, rir, expect: estimate1RM(w, r, f, rir) });
      }
    }
  }
}

const accuracy = [];
for (const r of [-1, 0, 1, 2, 3, 5, 8, 10, 12, 14, 15, 16, 30]) {
  for (const rir of [null, 0, 1, 3]) {
    for (const f of ['weighted', 'epley']) {
      accuracy.push({ r, rir, f, expect: calculate1RMAccuracy(r, rir, f) });
    }
  }
}

// selfcheck:每条公式以真名各算一针(可分辨值,r=6 时八值互不撞),防"两端同名
// fallback 互相掩盖"——若 gen 或 Go 侧拼错公式名,estimate1RM 会静默回退 epley,
// 组合 golden 可能假绿,本段的每名独立锚点会红。
const selfcheck = FORMULAS.map((f) => ({
  f,
  w: 100,
  r: 6,
  rir: null,
  expect: estimate1RM(100, 6, f, null),
}));

const out = {
  _comment: 'golden 夹具:期望值由 frontend/src/lib/onerm.js 真函数生成(gen-onerm.mjs),Go 侧 onerm_golden_test.go 比对,禁止手编',
  generatedBy: 'scripts/golden/gen-onerm.mjs',
  cases,
  accuracy,
  selfcheck,
};

const dest = join(ROOT, 'backend-go', 'internal', 'agent', 'testdata');
mkdirSync(dest, { recursive: true });
const destFile = join(dest, 'onerm-golden.json');
writeFileSync(destFile, JSON.stringify(out));
console.log(`wrote ${destFile}: ${cases.length} estimate cases + ${accuracy.length} accuracy cases`);
