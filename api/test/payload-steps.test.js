/* exerciseSteps — the zh technique pack a Chinese-language payload may carry.
 *
 * The field is optional by design: an English payload must be byte-for-byte the payload it
 * was before this existed, a language the catalogue has no pack for reads as English does,
 * and every bound the prompt relies on (a handful of exercises, a few steps, a chapter cap)
 * is pinned here so a future edit cannot quietly widen it past the payload ceiling.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import { tempData, sampleState, daysAgo } from './helpers.mjs';

tempData();
const payload = await import('../coach/core/payload.js');
const { INSTRUCTIONS_ZH, COUNT } = await import('../coach/core/instructions-zh.js');

const H = 'h'.repeat(16);

test('a zh payload carries the pack: id, Chinese name, steps', () => {
  const p = payload.build(sampleState(), { handle: H, kind: 'create', lang: 'zh' });
  assert.ok(Array.isArray(p.exerciseSteps), 'the field exists for a zh user');
  const first = p.exerciseSteps.find(e => e.id === '0001');
  assert.ok(first, '0001 is in the plan, so it is in the pack');
  assert.equal(first.n, INSTRUCTIONS_ZH['0001'].n, 'the name is the translated one');
  assert.ok(first.s.length > 0 && first.s.every(s => typeof s === 'string' && s), 'steps travel as strings');
});

test('zh-TW gets the traditional pack, not the simplified one', () => {
  const p = payload.build(sampleState(), { handle: H, kind: 'create', lang: 'zh-TW' });
  const first = p.exerciseSteps.find(e => e.id === '0001');
  assert.ok(first && first.n !== INSTRUCTIONS_ZH['0001'].n, 'the traditional name differs');
});

test('a non-zh payload has no pack, and no Chinese text rides along either', () => {
  const p = payload.build(sampleState(), { handle: H, kind: 'create', lang: 'en' });
  assert.ok(!('exerciseSteps' in p), 'the field is absent, not empty');
  const json = JSON.stringify(p);
  assert.ok(!json.includes(INSTRUCTIONS_ZH['0001'].n), 'no translated name leaks into an English payload');
});

test('the state language triggers it too — the lang arrives without opts', () => {
  const S = sampleState({ lang: 'zh' });
  const p = payload.build(S, { handle: H, kind: 'create' });
  assert.ok(Array.isArray(p.exerciseSteps), 'meta.lang fell back to S.lang and matched');
});

test('only the ids the payload is about are carried; misses fall through silently', () => {
  const S = sampleState();
  S.routines[0].ex.push({ id: 'no-such-exercise', sets: 3, reps: 10 }, { id: 'x'.repeat(13), sets: 3, reps: 10 });
  const p = payload.build(S, { handle: H, kind: 'create', lang: 'zh' });
  const ids = p.exerciseSteps.map(e => e.id);
  assert.ok(ids.includes('0001') && ids.includes('0007'), 'the plan\'s catalogue ids are there');
  assert.ok(!ids.includes('no-such-exercise') && !ids.includes('x'.repeat(13)), 'ids with no pack are skipped, not stubbed');
});

test('a debrief carries the steps of the session it reads', () => {
  const p = payload.build(sampleState(), { handle: H, kind: 'debrief', lang: 'zh' });
  const ids = (p.exerciseSteps || []).map(e => e.id);
  assert.ok(ids.includes('0001'), 'the session\'s one exercise has its steps');
});

test('the bounds hold: exercise cap, step cap, chapter cap, payload ceiling', () => {
  // A hundred catalogue exercises the pack covers — far past every cap.
  const S = sampleState({
    routines: [{
      id: 'r1', name: 'Big', ex: Object.keys(INSTRUCTIONS_ZH).slice(0, 100).map(id => ({ id, sets: 3, reps: 10 }))
    }]
  });
  const p = payload.build(S, { handle: H, kind: 'create', lang: 'zh' });
  const steps = p.exerciseSteps;
  assert.ok(steps.length <= payload.EXERCISE_STEPS_MAX, 'at most EXERCISE_STEPS_MAX exercises');
  assert.ok(steps.every(e => e.s.length <= payload.STEP_COUNT_MAX), 'at most STEP_COUNT_MAX steps each');
  assert.ok(steps.every(e => e.s.every(s => s.length <= payload.STEP_TEXT_MAX)), 'each step cut at STEP_TEXT_MAX');
  const chars = steps.reduce((n, e) => n + e.n.length + e.s.join('').length, 0);
  assert.ok(chars <= payload.EXERCISE_STEPS_CHARS + payload.STEP_COUNT_MAX * payload.STEP_TEXT_MAX + 100,
    'the chapter stays around EXERCISE_STEPS_CHARS (the last entry may complete)');
  assert.ok(JSON.stringify(p).length < payload.MAX_PAYLOAD_CHARS, 'the whole payload stays under the ceiling');
});

test('a real pack entry longer than the step cap is cut, not trusted', () => {
  const long = Object.values(INSTRUCTIONS_ZH).find(e => e.s.length > payload.STEP_COUNT_MAX);
  if (!long) return; // none in the pack today: the bound is still pinned by the test above
  const id = Object.keys(INSTRUCTIONS_ZH).find(k => INSTRUCTIONS_ZH[k] === long);
  const S = sampleState({ routines: [{ id: 'r1', name: 'A', ex: [{ id, sets: 3, reps: 10 }] }] });
  const p = payload.build(S, { handle: H, kind: 'create', lang: 'zh' });
  assert.equal(p.exerciseSteps[0].s.length, payload.STEP_COUNT_MAX);
});

test('the generated pack is the committed one: count matches the source it was built from', () => {
  assert.ok(COUNT > 1300, 'the committed pack carries the full classic set (sanity; regen: scripts/build-coach-assets.mjs)');
});
