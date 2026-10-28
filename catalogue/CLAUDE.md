[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **catalogue · 动作目录库**

# catalogue/ — 动作目录（沐禾健身认识的每一个动作）

> 19,693 个文件的纯数据模块：应用从这个目录构建动作库，**这里合并一个修正，下个版本全员受益**。fork 现状：🆕 **中文说明全量中文化**——`i18n/zh.json` 已从"只有名称"补齐到 description + instructions 共 5,632 条（2026-10-22，DeepSeek 批量 + 术语表对齐，台账见 FORK.md §5）。

## 模块职责

- 6,554 个动作，每个一个 JSON（英文物种源），id 为永久编号
- 19 语言翻译（`i18n/`）与生成的浏览页（`browse/`）；**zh 已是覆盖最全的语言之一**
- 授权媒体：每动作一张 webp 静图 + 180px mp4 动画

## 目录结构

| 路径 | 内容 |
|------|------|
| `exercises/<id>.json` | 单动作英文源：name / bodyPart / equipment / target / secondaryMuscles / category / description / instructions（+可选 muscleMap、femaleVariant/variantOf、textSource:"exercisedb"） |
| `i18n/<lang>.json` | 翻译：`{ "<id>": { name, description, instructions } }`，缺失回退英文。🆕 `zh.json`：名称全覆盖 + 5,632 条 description/instructions 中文（批量翻译唯一真相源，重跑 `translate-zh.mjs` 自动跳过已有字段） |
| `media/still/<id>.webp` · `media/clip/<id>.mp4` | 图片与动画（180px）。**Gym visual 授权，仅限本应用使用**（`media/NOTICE.md`；fork 版权边界见 FORK.md §3——非商业可原样保留，**商用须先取得自有授权**，禁止喂 AI） |
| `browse/` | 生成的分部位总览页（**勿手编**，由 build.mjs 重建） |
| `README.md` | 贡献者指南：字段规范、翻译规范、许可证规则 |

## 硬规则（违者被 PR check 拒绝）

1. **id 永不改、不复用、不删除**——workouts/routines/records 都存 id；重复动作靠改进入目
2. **media/ 不可增删改、不可外传、不可喂 AI 工具**；画面问题走 issue
3. bodyPart/equipment/target/category 只能用 `scripts/catalogue/validate.mjs` 顶部的词表——新词需要应用先支持，开 issue 讨论
4. 文案原创：不抄其他应用/网站/数据集；**不贴未审校的机器翻译**（fork 的 zh 全量翻译经术语表对齐审计口径，属已审校批次，台账在 FORK.md）；无医疗建议；无 em dash；名字小写、器材开头
5. 老的 `textSource: "exercisedb"` 1,324 条（MIT 文本）仍可改进

## 工具链

```bash
node scripts/catalogue/validate.mjs    # 校验所有条目（PR 上 Catalogue check 自动跑）
node scripts/catalogue/build.mjs       # 重生成应用数据文件与 browse/（zh 改动后必跑）
# fork：批量补中文（断点续跑；写回 zh.json 后必须再 build）
TRANSLATE_API_KEY=sk-xxx node scripts/catalogue/translate-zh.mjs [--limit 40]
```

其它脚本见 [scripts/CLAUDE.md](../scripts/CLAUDE.md)。

## 与前端的接线

`build.mjs` 产物进 `frontend/src/lib/exercises-data.js` 及翻译数据（🆕 `frontend/src/instr/zh.js`、`frontend/src/exercise-desc/zh.js`）；AI 教练中文包 `api/coach/core/instructions-zh.js` 已随中文化同步扩充。移动端构建用 `stage-media.mjs` 暂存 180px 媒体进 dist。

## 测试

无独立测试套件——质量闸门是 `validate.mjs` + GitHub `catalogue.yml` workflow；前端侧有 `catalogue-continuity.test.js` / `catalogue-browse.test.js` 等消费端测试防 id 断裂。

---

变更记录: 2026-10-27 V1.1 增量（init-architect）:zh 说明全量中文化 5,632 条、translate-zh 工具链、fork 版权边界注记；2026-10-20 V1.0 初版
