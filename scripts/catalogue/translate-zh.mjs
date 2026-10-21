#!/usr/bin/env node
// scripts/catalogue/translate-zh.mjs — 批量补全动作中文说明(fork 专属工具)
//
// 背景:openGym 的 catalogue/i18n/zh.json 只覆盖了动作「名称」;description 全缺,
// instructions 仅 1,324 条老动作有。本脚本对缺失条目调 OpenAI 兼容 API 批量翻译,
// 写回同一 i18n 文件(唯一真相源),之后跑 `node scripts/catalogue/build.mjs`
// 重新生成 frontend/src/instr/zh.js 与 exercise-desc/zh.js。
//
// 用法:
//   TRANSLATE_API_KEY=sk-xxx node scripts/catalogue/translate-zh.mjs            # 全量
//   TRANSLATE_API_KEY=sk-xxx node scripts/catalogue/translate-zh.mjs --limit 40 # 试跑
//
// 环境变量:
//   TRANSLATE_API_KEY   必填(也认 DEEPSEEK_API_KEY / ZHIPU_API_KEY)
//   TRANSLATE_BASE_URL  默认 https://api.deepseek.com/v1
//   TRANSLATE_MODEL     默认 deepseek-chat
//
// 断点续跑:每批完成立即写回 zh.json;重跑自动跳过已有译文的字段。
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const I18N = path.join(ROOT, 'catalogue/i18n/zh.json')
const EXERCISES = path.join(ROOT, 'catalogue/exercises')

const KEY = process.env.TRANSLATE_API_KEY || process.env.DEEPSEEK_API_KEY || process.env.ZHIPU_API_KEY
const BASE = (process.env.TRANSLATE_BASE_URL || 'https://api.deepseek.com/v1').replace(/\/$/, '')
const MODEL = process.env.TRANSLATE_MODEL || 'deepseek-chat'
const BATCH = Number(process.env.TRANSLATE_BATCH || 20)
const limitArg = process.argv.indexOf('--limit')
const LIMIT = limitArg > -1 ? Number(process.argv[limitArg + 1]) : Infinity

if (!KEY) {
  console.error('缺少 API key:请设置 TRANSLATE_API_KEY(或 DEEPSEEK_API_KEY / ZHIPU_API_KEY)')
  process.exit(1)
}

// —— 术语表:与 frontend/docs/i18n-terminology-audit.md 的结论一致 ——
const TERMS = `
必须遵守的术语(健身行业标准中文,不要直译):
- volume → 训练量; reps → 次; sets → 组; rest → 间歇(组间休息)
- superset → 超级组; drop set → 递减组; warm-up set → 热身组; pyramid set → 金字塔组
- deload → 减载; progressive overload → 渐进超负荷; range of motion → 活动范围
- 1RM / e1RM / RIR / RPE / PR 保留原文不译; PR 可写作「纪录」
- core → 核心; glutes → 臀肌; quads → 股四头肌; hamstrings → 腘绳肌; lats → 背阔肌
- 动作名中的 muscle/equipment 用上述口径;「自重」对应 bodyweight
- 语气:教练口吻,简洁,祈使句;与现有译文风格一致(参考 name 字段)
`.trim()

const SYSTEM = `你是健身动作数据的专业中文译者。用户会给你一批 JSON:{"<id>": {"description": "...", "instructions": ["...", ...]}}。
逐条翻译成简体中文,严格输出同结构 JSON:{"<id>": {"description": "...", "instructions": ["...", ...]}}。

规则:
1. 只输出 JSON,不要 markdown 代码块、不要解释。
2. 条数、id、instructions 数组长度必须与输入完全一致。
3. ${TERMS}
4. description 保持 1-2 句的教练口吻概述;instructions 每条一个步骤,保留细节(角度/次数/呼吸)。
5. 如果某条已带中文(极少),原样保留。`

const loadJson = p => JSON.parse(fs.readFileSync(p, 'utf8'))
const writeJson = (p, obj) => {
  // 与仓库现有格式一致:2 空格缩进、键按数字升序 —— 只让新增字段产生 diff
  const ids = Object.keys(obj).sort((a, b) => Number(a) - Number(b))
  const sorted = Object.fromEntries(ids.map(id => [id, obj[id]]))
  fs.writeFileSync(p, JSON.stringify(sorted, null, 2) + '\n')
}

async function callApi(payload, attempt = 1) {
  const res = await fetch(`${BASE}/chat/completions`, {
    method: 'POST',
    headers: { 'content-type': 'application/json', authorization: `Bearer ${KEY}` },
    body: JSON.stringify({
      model: MODEL,
      temperature: 0.2,
      messages: [
        { role: 'system', content: SYSTEM },
        { role: 'user', content: JSON.stringify(payload) },
      ],
    }),
  })
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    if (attempt <= 3) {
      console.warn(`  API ${res.status},重试 ${attempt}/3…`)
      await new Promise(r => setTimeout(r, 2000 * attempt))
      return callApi(payload, attempt + 1)
    }
    throw new Error(`API ${res.status}: ${body.slice(0, 200)}`)
  }
  const data = await res.json()
  let text = data.choices?.[0]?.message?.content?.trim() || ''
  text = text.replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/, '')
  try {
    return { parsed: JSON.parse(text), usage: data.usage }
  } catch (e) {
    // 模型偶发输出坏 JSON(长文本里未转义/截断):重试整批,3 次仍失败才抛
    if (attempt <= 3) {
      console.warn(`  JSON 坏(第 ${attempt} 次),重试…`)
      await new Promise(r => setTimeout(r, 1500 * attempt))
      return callApi(payload, attempt + 1)
    }
    throw new Error(`JSON 解析失败: ${e.message} — 片段: ${text.slice(0, 160)}`)
  }
}

async function main() {
  const zh = loadJson(I18N)
  const exFiles = fs.readdirSync(EXERCISES).filter(f => f.endsWith('.json'))
  const todo = []
  for (const f of exFiles) {
    const e = loadJson(path.join(EXERCISES, f))
    const cur = zh[e.id] || {}
    const needDesc = e.description && !cur.description
    const needInstr = Array.isArray(e.instructions) && e.instructions.length && !cur.instructions
    if (needDesc || needInstr) todo.push({ id: e.id, e, needDesc, needInstr })
    if (todo.length >= LIMIT) break
  }
  console.log(`待翻译 ${todo.length} 条;端点 ${BASE} 模型 ${MODEL} 批大小 ${BATCH}`)

  let done = 0, tokens = 0
  for (let i = 0; i < todo.length; i += BATCH) {
    const batch = todo.slice(i, i + BATCH)
    const payload = {}
    for (const { id, e, needDesc, needInstr } of batch) {
      payload[id] = {}
      if (needDesc) payload[id].description = e.description
      if (needInstr) payload[id].instructions = e.instructions
    }
    process.stdout.write(`[${i + batch.length}/${todo.length}] 翻译中… `)
    let parsed, usage
    try {
      ({ parsed, usage } = await callApi(payload))
    } catch (e) {
      // 单批失败不拖垮全量:记录后跳过,重跑脚本会自动重试这些条目
      console.log(`❌ 跳过本批(${e.message.slice(0, 80)});重跑脚本会再试`)
      continue
    }
    tokens += (usage?.total_tokens || 0)
    let ok = 0
    for (const { id, e, needDesc, needInstr } of batch) {
      const got = parsed[id]
      if (!got) continue
      zh[id] = zh[id] || {}
      if (needDesc && typeof got.description === 'string' && got.description.trim()) {
        zh[id].description = got.description.trim(); ok++
      }
      if (needInstr && Array.isArray(got.instructions) && got.instructions.length === e.instructions.length) {
        zh[id].instructions = got.instructions; ok++
      }
    }
    writeJson(I18N, zh)  // 断点:每批落盘
    done += batch.length
    console.log(`本批写入 ${ok} 字段,tokens=${usage?.total_tokens || '?'}`)
  }
  console.log(`\n✅ 完成 ${done} 条,total tokens ≈ ${tokens.toLocaleString()}`)
  console.log('下一步: node scripts/catalogue/build.mjs  # 重新生成 frontend/src/{instr,exercise-desc}/zh.js')
}

main().catch(e => { console.error('❌', e.message); process.exit(1) })
