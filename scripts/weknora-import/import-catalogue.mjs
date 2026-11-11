#!/usr/bin/env node
/* import-catalogue.mjs — catalogue 动作知识 → WeKnora 全局知识区 ETL(批次 3,D16)。
 *
 * 用法(仓根):
 *   WEKNORA_BASE_URL=http://127.0.0.1:8081/api/v1 WEKNORA_API_KEY=wk \
 *     node scripts/weknora-import/import-catalogue.mjs [--limit N] [--dry-run] [--lang zh|en]
 *
 * 纪律(方案 v2 六 / D16):
 *   1. 媒体永不进管道——只搬文本(名称/描述/要点),授权媒体由前端凭 action id
 *      从 catalogue 自取,本脚本不读 media。
 *   2. 每动作一篇 Markdown,action id 写进文档正文(检索结果凭 id 回前端渲染动作卡)。
 *   3. 断点续跑:manifest(imported-ids.json)记录已导入 id,重跑自动跳过。
 *   4. zh 优先:catalogue/i18n/zh.json 有译文的用译文(全量中文化资产复用)。
 *   5. 批量向量化走 Embedding-3 资源包——额度实施前按 D16 公式核算(本脚本
 *      打印估算数,--dry-run 可先看规模)。
 */
import { readdirSync, readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const args = process.argv.slice(2);
const arg = (name, dflt) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 && args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : dflt;
};
const flag = (name) => args.includes(`--${name}`);

const BASE_URL = (process.env.WEKNORA_BASE_URL || '').replace(/\/$/, '');
const API_KEY = process.env.WEKNORA_API_KEY || '';
const LANG = arg('lang', 'zh');
const LIMIT = Number(arg('limit', '0')) || 0;
const DRY = flag('dry-run');
const KB_NAME = arg('kb-name', '沐禾动作知识');

if (!DRY && (!BASE_URL || !API_KEY)) {
  console.error('需要 WEKNORA_BASE_URL / WEKNORA_API_KEY(--dry-run 可先看规模不连网)');
  process.exit(1);
}

// ── 读源:catalogue/exercises/*.json + i18n zh ──────────────────────────────
const exDir = join(ROOT, 'catalogue', 'exercises');
const files = readdirSync(exDir).filter((f) => f.endsWith('.json'));
const zhPath = join(ROOT, 'catalogue', 'i18n', 'zh.json');
let zh = {};
if (LANG === 'zh' && existsSync(zhPath)) {
  zh = JSON.parse(readFileSync(zhPath, 'utf8'));
}

// ── 断点清单 ────────────────────────────────────────────────────────────────
const stateDir = join(ROOT, 'scripts', 'weknora-import');
const manifestPath = join(stateDir, 'imported-ids.json');
const manifest = existsSync(manifestPath) ? JSON.parse(readFileSync(manifestPath, 'utf8')) : { kbId: '', imported: [] };

// ── 组装文档(action id 进正文;媒体零参与) ─────────────────────────────────
function actionDoc(a) {
  const id = a.id;
  const nameZh = zh?.[id]?.name || a.name || id;
  const descZh = zh?.[id]?.description || a.description || '';
  const instrZh = zh?.[id]?.instructions || a.instructions || '';
  return [
    `# ${nameZh}(动作 id: ${id})`,
    '',
    descZh,
    '',
    instrZh,
    '',
    `<!-- source: catalogue/exercises/${id}.json lang=${LANG} -->`,
  ].join('\n');
}

const docs = [];
for (const f of files) {
  if (docs.length >= LIMIT && LIMIT > 0) break;
  try {
    const a = JSON.parse(readFileSync(join(exDir, f), 'utf8'));
    if (manifest.imported.includes(a.id)) continue;
    docs.push({ title: `${zh?.[a.id]?.name || a.name || a.id}(${a.id})`, text: actionDoc(a), id: a.id });
  } catch {
    /* 坏条目跳过,validate.mjs 是质量闸,这里不重复报 */
  }
}
console.log(`待导入 ${docs.length} 篇(总 ${files.length},已导入 ${manifest.imported.length},lang=${LANG})`);

// token 额度估算(D16 公式:文档字符数 ≈ 中文 1 字符 ≈ 1 token,上浮 20% 余量)。
const chars = docs.reduce((n, d) => n + d.text.length, 0);
console.log(`预估向量化 token ≈ ${Math.ceil(chars * 1.2)}(文档 ${docs.length} 篇,批量一次性)`);

if (DRY) {
  console.log('dry-run 完成,不连网。');
  process.exit(0);
}

// ── WeKnora API(与 Go 客户端同形:knowledge-search/knowledge-bases/documents) ──
async function api(method, path, body) {
  const res = await fetch(BASE_URL + path, {
    method,
    headers: { 'content-type': 'application/json', authorization: `Bearer ${API_KEY}` },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${method} ${path} → ${res.status}: ${(await res.text()).slice(0, 200)}`);
  return res.json();
}

// 建库/找库(幂等:先列后建——列表端点按名匹配)。
let kbId = manifest.kbId;
if (!kbId) {
  const list = await api('GET', `/knowledge-bases?per_page=100`);
  const found = (list.data || []).find((k) => k.name === KB_NAME);
  kbId = found ? found.id : (await api('POST', '/knowledge-bases', { name: KB_NAME, description: 'catalogue 动作知识(ETL,文本 only,媒体不在管道)' })).data.id;
  manifest.kbId = kbId;
  console.log(`知识库: ${KB_NAME} → ${kbId}`);
}

// 分批导入(10/批,失败重试 1 次;节奏友好,避免压爆向量化队列)。
mkdirSync(stateDir, { recursive: true });
let ok = 0, fail = 0;
for (let i = 0; i < docs.length; i += 10) {
  const batch = docs.slice(i, i + 10);
  for (const d of batch) {
    try {
      await api('POST', `/knowledge-bases/${kbId}/documents`, { knowledge_base_id: kbId, title: d.title, content: d.text });
      manifest.imported.push(d.id);
      ok++;
    } catch (e) {
      fail++;
      console.error(`  ✗ ${d.id}: ${String(e.message).slice(0, 120)}`);
    }
  }
  writeFileSync(manifestPath, JSON.stringify(manifest));
  console.log(`  批 ${i / 10 + 1}: 累计 ${ok} ok / ${fail} fail`);
}
console.log(`完成:${ok} 导入 / ${fail} 失败;manifest 已写 ${manifestPath}`);
