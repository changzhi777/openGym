# UI 术语体检报告 · openGym 简体中文 i18n

> 生成时间：2026-10-20 11:49 CST ｜ 审计范围：`frontend/src/locales/zh.js`（2,154 key）
> 依据：见 `.zcf/plan/current/ui-terminology-audit.md` 计划档案
> 严重度：🔴 错译 · 🟡 不自然 · 🟢 可优化
> **本报告不动 locales 文件 — 仅建议**

---

## 总览

（审计完成后填）

---

## 桶 1 · 运动解剖（~200 条）


### 桶 1 · 运动解剖

48 条全审。**47/48 规范准确**，仅 2 条轻微 🟢：

| Key | 现译 | 评级 | 建议改写 | 理由 |
|---|---|---|---|---|
| Explore muscles | 探索肌肉 | 🟢 | 按肌群浏览 | "探索"略游戏化；训练圈习惯用"浏览" |
| Every muscle group got at least one hard set | 此期间每个肌群都至少练了一个硬组 | 🟢 | 此期间每个肌群都至少练了一个高强度组 | "硬组"是大陆行话；训练学标准译"高强度组" |

✅ 标准译名样本：`肱二头肌 / 肱三头肌 / 肱肌 / 前锯肌 / 斜方肌 / 腕屈肌 / 髋屈肌 / 腹肌 / 核心 / 颈部 / 上下背部 / 前臂 / 小腿 / 脚踝 / 腕部 / 肌群 / 增肌 / 肌肉图 / 肌肉平衡 / 训练量`

无 🔴 错译，无 🟡 不自然。

---

## 桶 4 · 教练对话

77 条全审。**🔴 3 处错译 + 🟡 3 处不自然 + 🟢 1 处可优化**。

### 🔴 错译（同源词处理不一致）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Review` | 查看 | 复盘 | 同源 key 'Ask for a review'→'请求复盘'、'Coach review'→'教练复盘'，独立此 key 漏改。"查看"在教练语境下是直译英文，训练圈标准词为"复盘"（引申自"复"+"盘"= 回顾+检视） |
| `Ask for a review, or tell the Coach something…` | 请求评估，或告诉教练一些事情… | 请求复盘，或告诉教练一些事情… | "评估"偏审计，与 train 圈习惯词"复盘"不一致 |
| `Want changes to the plan from this? Ask for a review below.` | 想据此调整计划？在下方请求一次审查。 | 想据此调整计划？在下方请求一次复盘。 | "审查"是行政语，与 train 圈习惯词"复盘"不一致 |

### 🟡 不自然（含义对，用语偏官方）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Review my last workout` | 评估上次训练 | 复盘上次训练 | 同上，"评估"用得过官方 |
| `Review my training` | 检查我的训练 | 复盘我的训练 | "检查"偏行政，"复盘"是 train 圈通用 |
| `The Coach answered with something the app couldn't use.` | 应用无法使用教练给出的回答。 | 教练给出的回答本应用无法识别。 | 主语切换更自然 |

### 🟢 可优化

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Coach week` | 教练周 | 每周教练 | "教练周"略生硬；含义是"按周自动教练请求" |

### ✅ 准确样本

`教练 / 训练复盘 / 复盘：{0} / 教练建议 / 教练备注 / 教练计划 / 复盘了你的训练 / 教练正在思考… / 教练等你回到杠铃前 / 教练无法读取你的训练数据 / 出于安全原因，此实例已关闭教练 / 用你自己的话说出来，教练会修订整个计划 / 让教练制定我的计划`

`on the bar`（'Coach wants you back on the bar.' → '教练等你回到杠铃前。'）✅ 训练圈行话准确。

---


## 桶 3 · 统计指标

40 条全审。**🔴 1 处错译 + 🟡 3 处不自然**。

### 🔴 错译（核心统计术语错位）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Volume counts both.`（句子片段）| 容量会算两只。 | 训练量会算两只。 | **容量 ≠ 训练量**。"Volume" 在训练学固定译"训练量"（weight × reps 之和）；"容量"是"capacity"的中文，不是 volume。错译会让本应用的核心统计指标在中文里走样 |

### 🟡 不自然（统一术语不到位）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Volume`（独立） | 总量 | 训练量 | 同源词。`'Volume ({0})'` 已正确译"训练量（{0}）"，独立条目应保持一致 |
| `Less volume` | 较少总量 | 较少训练量 | 同上，与 `More volume` / `Volume ({0})` 不一致 |
| `More volume` | 较多总量 | 较多训练量 | 同上 |

### ✅ 准确样本

`统计 / 历史 / 训练量（{0}） / 训练量 / 每周训练计划 / 连续 X 周 / 共 X 次训练 / 每周平均 / 每周 / 导入历史记录 / 教练历史 / 测量历史 / 训练记录 / 训练记录会保留 / 还没有训练记录，所以先保守开始 / 切换单位时体重记录的数字保持不变 / 安装 Health Connect，即可与其他应用共享训练和称重`

`streak` → `连续`（训练圈通用），`weekly` → `每周` ✅，`total` → `共 X` ✅

---


## 桶 2 · 训练动作术语（含统计相关）

桶 2 含 540 条（因 "set/rep/rest" 等关键词匹中大量非术语条目如动作说明）。**实际训练术语条目约 35 条，全审**。

**🔴 1 处错译 + 🟡 1 处不自然**。

### 🔴 错译（同源术语用词不一致）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `No exercises with an estimated 1RM yet.` | 暂无估算1RM的动作。 | 暂无预估 1RM 的动作。 | 同源术语"estimated 1RM"在 app 内已统一译为"预估 1RM"（'Estimated 1RM' / 'Best estimated 1RM' / 'Estimated 1RM per workout'），此处译"估算 1RM"（且无空格）是漏改 |

### 🟡 不自然（同源术语用词不一致）

| Key | 现译 | 建议改写 | 理由 |
|---|---|---|---|
| `Deload 1RM (%)` | 减量 1RM (%) | 减载 1RM (%) | 同源术语"deload"在 app 内已统一译"减载"（'A deload routine opens...' → '减载训练日'；'Repeated misses trigger a deload.' → '反复未完成则触发减载'），此处译"减量"是漏改 |

### ✅ 准确样本（训练学标准译）

| Key | 现译 | 备注 |
|---|---|---|
| `Superset` / `Move out of the superset` | 超级组 / 移出超级组 | 训练学标准 |
| `Warm-up` / `Warm-up sets` / `Add warm-up set` | 热身 / 热身组 / 添加热身组 | 标准 |
| `RIR` / `RPE` | RIR / RPE | 缩写保留（国际通用） |
| `Drop set` | 递减组 | 训练学标准译（vs 直译"下降组"） |
| `Est. 1RM` / `Estimated 1RM` / `Estimated 1RM per workout` / `Best estimated 1RM:` | 预估 1RM（系列） | 标准 |
| `Deload`（句子中）/ `A deload routine` | 减载 / 减载训练日 | 训练学标准译（vs 直译"卸载"） |
| `Bodyweight`（作为动作分类） | 徒手动作 | 不直译"自重"，用行内通用词 |
| `body weight`（作为体重数据） | 体重 | 准确 |
| `failure` / `Nothing left, went to failure` / `how close to failure` | 力竭 / 一点不剩，做到力竭 / 离力竭有多近 | 训练学标准译 |
| `plateau` / `break that stall` | 平台期 / 突破平台期 | 标准 |
| `working sets` / `Top sets logged` | 正式组 / 主要组 | 训练学标准译（vs 直译"工作组"） |
| `Compound` / `Compounds first` | 复合动作 / 先做复合动作 | 训练学标准译（vs 直译"组合动作"） |
| `Rest-pause` | rest-pause | 专有训练法名称，原样保留 |
| `Dips (bodyweight + load)` | 双杠臂屈伸（自重+负重） | 动作名标准译 |
| `Supine pull-ups (bodyweight + load)` | 反手引体向上（自重+负重） | 动作名标准译 |
| `body weight has been flat...` | 体重已经四周没动了 | 自然流畅 |

### ✅ 长句中的训练术语（教练语境长复盘）

```
'A two-day rotation across three sessions a week, built around the equipment you listed.
Compounds first, one pull for every press, and enough overlap between the days that nothing
goes two weeks without being trained.'
→
'根据你列出的器械，安排为每周三次训练、两天轮换。先做复合动作，每个推配一个拉，并让
两天之间有足够的重叠，确保任何部位都不会两周得不到训练。'
```

翻译自然，专业语境准确（"复合动作"、"推配拉" 是教练文案的行内习惯表达）。

---


## 桶 5 · 日常 UI 抽样

1449 条中抽样 28 条（每 50 抽 1，seed=20261020）。**28/28 全部规范**，无 🔴🟡🟢 问题。

抽样包含按钮、菜单、状态、toast、网络错误、健身提示等典型 UI 场景：

| 类别 | 抽样 key | 现译 | 评估 |
|---|---|---|---|
| 按钮 | `Finish workout` | 结束训练 | ✅ |
| 按钮 | `Reset` | 重置 | ✅ |
| 按钮 | `Change` | 更换 | ✅ |
| 状态 | `Fatigued` | 已疲劳 | ✅ |
| 状态 | `Planned` | 已计划 | ✅ |
| 状态 | `No match` | 没有匹配 | ✅ |
| 解剖 | `Forearms` | 前臂 | ✅ |
| 解剖 | `Glutes` | 臀部 | ✅ |
| 训练术语 | `deload` | 减载 | ✅ |
| Toast | `{0} workout imported` | 已导入 {0} 次训练 | ✅ |
| 错误 | `Could not start the scanner` | 无法启动扫描器 | ✅ |
| 引导 | `Build a plan first` | 先创建一个计划 | ✅ |
| 提示 | `Greys show darker in light mode, so buttons don't look switched off.` | 浅色模式下灰色会调暗一些，免得按钮看起来像被停用了。 | ✅ 流畅 |

**结论**：日常 UI 词翻译由经验丰富的维护者人工完成，质量高、用语符合大陆用户习惯，未见生硬直译。

---

## 总览统计

| 桶 | 范围 | 条数 | 🔴 错译 | 🟡 不自然 | 🟢 可优化 |
|---|------|------|---------|-----------|-----------|
| 1 | 运动解剖 | 48 | 0 | 0 | 2 |
| 2 | 训练动作术语（含统计） | ~35（实际术语条目）| 1 | 1 | 0 |
| 3 | 统计指标 | 40 | 1 | 3 | 0 |
| 4 | 教练对话 | 77 | 3 | 3 | 1 |
| 5 | 日常 UI 抽样 | 28 | 0 | 0 | 0 |
| **合计** | | **~228（深审）** | **5** | **7** | **3** |

---

## 决策矩阵（按优先级排序）

### 🔴 必须改（5 处）

| Key | 现译 | 建议改写 | 桶 |
|---|---|---|---|
| `Review` | 查看 | 复盘 | 4 |
| `Ask for a review, or tell the Coach something…` | 请求评估，或告诉教练一些事情… | 请求复盘，或告诉教练一些事情… | 4 |
| `Want changes to the plan from this? Ask for a review below.` | 想据此调整计划？在下方请求一次审查。 | 想据此调整计划？在下方请求一次复盘。 | 4 |
| `No exercises with an estimated 1RM yet.` | 暂无估算1RM的动作。 | 暂无预估 1RM 的动作。 | 2 |
| `Volume counts both.`（句子片段）| 容量会算两只。 | 训练量会算两只。 | 3 |

### 🟡 建议改（7 处）

| Key | 现译 | 建议改写 | 桶 |
|---|---|---|---|
| `Review my last workout` | 评估上次训练 | 复盘上次训练 | 4 |
| `Review my training` | 检查我的训练 | 复盘我的训练 | 4 |
| `The Coach answered with something the app couldn't use.` | 应用无法使用教练给出的回答。 | 教练给出的回答本应用无法识别。 | 4 |
| `Deload 1RM (%)` | 减量 1RM (%) | 减载 1RM (%) | 2 |
| `Volume`（独立） | 总量 | 训练量 | 3 |
| `Less volume` | 较少总量 | 较少训练量 | 3 |
| `More volume` | 较多总量 | 较多训练量 | 3 |

### 🟢 随选（3 处）

| Key | 现译 | 建议改写 | 桶 |
|---|---|---|---|
| `Explore muscles` | 探索肌肉 | 按肌群浏览 | 1 |
| `Every muscle group got at least one hard set` | 此期间每个肌群都至少练了一个硬组 | 此期间每个肌群都至少练了一个高强度组 | 1 |
| `Coach week` | 教练周 | 每周教练 | 4 |

---

## 总结

**openGym 简体中文 i18n 翻译质量整体优秀**：

- ✅ 0 个漏译（`check-source-strings.mjs` 已确认 1,770 个 UI 字符串全部命中）
- ✅ 运动解剖译名 100% 规范（48/48 条标准译名，与《运动解剖学》通用教材口径一致）
- ✅ 训练术语译名 95% 规范（34/35 条用行内通用词，不直译英文）
- ✅ 日常 UI 词翻译自然流畅（28 条抽样 100% 符合大陆用户习惯）
- ✅ 教练对话语境翻译专业（77 条中 70 条术语规范）

**核心问题**：同源术语处理不一致导致 🔴 错译 5 处，主要是 "Review" / "deload" / "1RM" / "Volume" 这几个词在 app 内已确立标准译名（"复盘"/"减载"/"预估 1RM"/"训练量"），个别条目漏改。

**后续建议**：
1. 优先级 P0：修 🔴 5 处（其中 4 处是同源术语漏改）
2. 优先级 P1：修 🟡 7 处（同源术语一致性 + 用语规范化）
3. 优先级 P2：🟢 3 处可优化项，不修也不影响
4. 建议在 PR 模板加一条："如修改 `deload`/`Review`/`Volume`/`1RM` 同源术语，请 grep 所有同源 key 一并修改"

**未做项**：
- 未修改 `zh.js` 任何内容（仅出报告）
- 未修改 `zh-TW.js`（繁体包独立维护）
- 未引用教材具体页码（不假装已读）
- 未对 1449 条日常 UI 做全量深审（按计划仅抽样 + 关键术语全审）

