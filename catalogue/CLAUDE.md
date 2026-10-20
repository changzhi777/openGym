[根目录](../CLAUDE.md) > **catalogue · 动作目录库**

# catalogue/ — 动作目录（openGym 认识的每一个动作）

> 19,693 个文件的纯数据模块：应用从这个目录构建动作库，**这里合并一个修正，下个版本全员受益**。

## 模块职责

- 6,554 个动作，每个一个 JSON（英文物种源），id 为永久编号
- 19 语言翻译（`i18n/`）与生成的浏览页（`browse/`）
- 授权媒体：每动作一张 webp 静图 + 180px mp4 动画

## 目录结构

| 路径 | 内容 |
|------|------|
| `exercises/<id>.json` | 单动作英文源：name / bodyPart / equipment / target / secondaryMuscles / category / description / instructions（+可选 muscleMap、femaleVariant/variantOf、textSource:"exercisedb"） |
| `i18n/<lang>.json` | 翻译：`{ "<id>": { name, description, instructions } }`，缺失回退英文（欢迎部分翻译） |
| `media/still/<id>.webp` · `media/clip/<id>.mp4` | 图片与动画（180px）。**Gym visual 授权，仅限 openGym 使用**（`media/NOTICE.md`） |
| `browse/` | 生成的分部位总览页（**勿手编**，由 build.mjs 重建） |
| `README.md` | 贡献者指南：字段规范、翻译规范、许可证规则 |

## 硬规则（违者被 PR check 拒绝）

1. **id 永不改、不复用、不删除**——workouts/routines/records 都存 id；重复动作靠改进入目
2. **media/ 不可增删改、不可外传、不可喂 AI 工具**；画面问题走 issue
3. bodyPart/equipment/target/category 只能用 `scripts/catalogue/validate.mjs` 顶部的词表——新词需要应用先支持，开 issue 讨论
4. 文案原创：不抄其他应用/网站/数据集；不贴未审校的机器翻译；无医疗建议；无 em dash；名字小写、器材开头
5. 老的 `textSource: "exercisedb"` 1,324 条（MIT 文本，见 NOTICE.md）仍可改进

## 工具链

```bash
node scripts/catalogue/validate.mjs   # 校验所有条目（Node 20+；PR 上 Catalogue check 自动跑并指出错处）
node scripts/catalogue/build.mjs      # 重生成应用数据文件（frontend/src/lib/exercises-data.js 等）与 browse/
```

其它脚本（在仓库 `scripts/catalogue/`）：`import-drafts.mjs` / `import-legacy.mjs` / `import-names.mjs`（导入）、`check-media.mjs` / `stage-media.mjs`（媒体核查/暂存）、`migration-check.mjs`、`drafts-from-pack.py`。详见 [scripts/CLAUDE.md](../scripts/CLAUDE.md)。

## 与前端的接线

`build.mjs` 产物进 `frontend/src/lib/exercises-data.js`（及动作名/说明翻译数据）；移动端构建用 `stage-media.mjs` 把 180px 媒体暂存进 dist（指向 CDN 数据集）。v1.4.0 前的旧媒体下载目录是根 `media/`（gitignored，仅兼容保留）。

## 测试

无独立测试套件——质量闸门是 `validate.mjs` + GitHub `catalogue.yml` workflow；前端侧有 `catalogue-continuity.test.js` / `catalogue-browse.test.js` 等消费端测试防 id 断裂。

---

变更记录: 2026-10-20 V1.0 初版（init-architect）
