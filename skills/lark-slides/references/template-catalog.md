# Slides 模板目录

> **创建 PPT 前必读。** 先根据用户需求匹配 1-2 个最佳模板，再根据每个模板下方的“页型索引”确定需要的页面类型。模板 XML 位于 `assets/templates/*.xml`，属于机器资源；默认通过脚本摘要或裁切，**不要阅读全文模板 XML**，尤其不要默认读取 `light_general.xml` / `dark_general.xml` 这类 54/56 页通用变体库全文。

> **机器优先路径。** 优先运行 [`../scripts/template_tool.py`](../scripts/template_tool.py) 的 `search` 做低成本路由；锁定模板后运行 `summarize` 获取主题和布局摘要；只有需要具体布局骨架时才运行 `extract` 裁切目标页型 XML。`template-index.json` 是脚本缓存/轻量路由索引，不是默认阅读入口。

> **降级路径。** 如果 `template_tool.py` 不可用，使用本文按场景确定候选模板和页型，但不要因为脚本不可用就直接阅读全文模板 XML。

> **对话输出规则。** 当处于需求澄清阶段时，应该把匹配结果整理成 **2-3 个用户可选模板候选**，不要默认把整份目录贴给用户。每个候选至少包含：模板名、适用场景、风格/色调、简短推荐理由。优先展示场景强相关模板；`light_general.xml` / `dark_general.xml` 这类通用模板只作为兜底或补充选项。

## 使用方法

1. 先运行 `python3 skills/lark-slides/scripts/template_tool.py search --query "<主题>" --limit 3`，根据用户描述的 **场景、风格、色调** 做初筛
2. 整理出 **2-3 个**最匹配的用户可选模板候选；优先选场景强相关模板，没有明显场景模板时再用标 ⭐ 的通用模板兜底
3. 用户选定后，再锁定 **1-2 个**最匹配的模板作为实际参考
4. 先看模板下方的 **页型索引**，锁定你真正需要的页型：封面 / 目录 / 分节 / 内容 / 结尾
5. 优先运行 `template_tool.py summarize` 查看 `<theme>` / 页型摘要；只有需要具体布局骨架时，再运行 `template_tool.py extract`
6. 从模板中提取并复用：`<theme>` 配色、页面流、shape 排列布局、装饰元素风格
7. 将用户的实际内容填充到模板的结构框架中，**不要照搬模板的占位文字**
8. 创建前运行 `layout_lint.py --input <file>`；它检查 XML well-formed 和布局风险，不等价于完整 XSD schema 校验

### 脚本快捷命令

```bash
# 先找候选模板
python3 skills/lark-slides/scripts/template_tool.py search --query "工作汇报" --tone light --limit 3

# 看指定页型的紧凑摘要
python3 skills/lark-slides/scripts/template_tool.py summarize --template office--work_report --label 内容

# 只裁切目标页型，避免把整份 XML 拉进上下文
python3 skills/lark-slides/scripts/template_tool.py extract --template office--work_report --label 封面 --out /tmp/work-report-cover.xml
```

如果脚本路径不可用，按这个顺序手动降级：

1. 回到本文对应分类，确认页数、色调、适用场景，选 2-3 个候选
2. 根据“页型索引”确定需要的目标页型
3. 等脚本可用后再用 `summarize` / `extract` 获取详细布局数据；不要整份展开 `assets/templates/*.xml`

## 匹配维度

| 维度 | 说明 |
|------|------|
| 场景 | 用户要做什么？（工作汇报、产品介绍、商业计划书、培训...） |
| 色调 | light（浅色/白底）、dark（深色/黑底）、colorful（多彩活泼） |
| 正式度 | formal（正式商务）、casual（轻松分享）、creative（创意设计） |

---

## office — 办公通用

### ⭐ light_general.xml — 浅色通用变体库

- **54 页 slide 变体**（非完整 PPT，是按需挑选的页面类型库）
- 色调：light | 正式度：formal
- 配色：白底 `#f4f5f6`，蓝色点缀 `#4e6efd`，深灰文字 `#1f2329`
- 页面类型：封面×2, 目录×4, 分节页, 内容×30, 数据卡片×2, 结尾
- 页型索引（建议先读）：封面 `1-2` | 目录 `3-6` | 分节/过渡 `7-8` | 内容/图文/时间线/数据 `9-53` | 结尾 `54`
- **适用**：任何需要浅色风格的场景，从中挑选合适的页面布局

### ⭐ dark_general.xml — 深色通用变体库

- **56 页 slide 变体**（同上，深色版）
- 色调：dark | 正式度：formal
- 配色：深灰底 `#1f2329`，蓝色点缀 `#4e6efd`，白色文字
- 页面类型：封面×2, 目录×4, 分节页, 内容×31, 数据卡片×2, 结尾
- 页型索引（建议先读）：封面 `1-2` | 目录 `3-6` | 分节/过渡 `7-8` | 内容/图文/时间线/数据 `9-54` | 结尾 `55-56`
- **适用**：任何需要深色风格的场景，渐变填充（蓝-青）、透明度叠加

### work_report.xml — 工作汇报

- 13 页 | 色调：light | 正式度：formal
- 配色：图片底图 + 半透明白色卡片（alpha=0.51），紫色 `#7f3bf5`/蓝色 `#3370ff` 点缀
- 结构：封面 → 目录(4 Part) → 数据页 → 内容×9 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `4-12` | 结尾 `13`
- **适用**：工作汇报、月报、项目进展。中英双语标题，4 宫格布局，圆角卡片

### work_summary_report.xml — 工作总结报告

- 16 页 | 色调：dark | 正式度：formal
- 配色：深灰底 `#2b2f36`，深蓝 `#3a6cea`/靛蓝 `#0e5efd` 点缀
- 结构：封面 → 目录 → [分节页 → 内容]×4 → 结尾（4 个清晰分节）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 6, 9, 12` | 内容 `4-5, 7-8, 10-11, 13-15` | 结尾 `16`
- **适用**：年度/季度工作总结，分节清晰的正式汇报

### work_summary.xml — 工作总结

- 20 页 | 色调：light | 正式度：formal
- 配色：暖黄 `#fff6dc` + 浅蓝 `#ddeff7` 交替，天蓝 `#1393cf` 点缀，金色 `#f7c15f` 强调
- 结构：封面 → 目录 → 内容×7 → 数据×2 → 内容×8 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-9, 12-19` | 结尾 `20`
- **适用**：轻松温暖风格的工作总结。shape 类型最丰富（饼图、三角、弧形等）

### quarterly_review.xml — 季度复盘

- 17 页 | 色调：dark | 正式度：formal
- 配色：海军蓝图片底图，钢蓝 `#435671` 文字，半透明白色叠加（alpha=0.6）
- 结构：封面 → 目录 → 内容×2 → 数据×2 → 内容×10 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-4, 7-16` | 结尾 `17`
- **适用**：季度复盘、OKR 回顾，数据驱动型汇报

### dept_annual_report.xml — 部门年度汇报

- 17 页 | 色调：dark | 正式度：formal
- 配色：黑色底图 + 极光渐变（紫/青/蓝），青色 `#64e8d6` → 蓝色 `#73bbff` 渐变点缀
- 结构：封面 → 目录 → 内容×4 → 数据 → 内容×9 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-6, 8-16` | 结尾 `17`
- **适用**：年度部门汇报，视觉冲击力强，多彩渐变风

### project_kickoff.xml — 项目启动宣讲

- 18 页 | 色调：dark | 正式度：formal
- 配色：蓝色渐变底 `linear-gradient(#4b5cf5, #233afb)`，紫色 `#ad82f7` 强调
- 结构：封面 → 目录 → 内容×15 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-17` | 结尾 `18`
- **适用**：项目启动、产品发布、技术宣讲。大胆蓝紫渐变，卡片式布局

---

## product — 产品

### product_analysis.xml — 产品分析报告

- 17 页 | 色调：light | 正式度：formal
- 配色：柔蓝渐变底图 `#deebff → #ffffff`，蓝色 `#2e4ef6 → #2182f4` 点缀，浅蓝卡片 `#e2eaf9`
- 结构：封面 → 目录 → 内容 → 数据 → 内容×4 → 数据 → 内容×5 → SWOT → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3, 5-8, 10-16` | 结尾 `17`
- **适用**：产品分析、竞品分析。包含 SWOT 分析页，数据图表页

### product_intro.xml — 产品介绍

- 18 页 | 色调：colorful | 正式度：formal
- 配色：多彩渐变（每个分节不同色系：青蓝、橙红、紫、蓝靛）
- 结构：封面 → 目录 → [分节页 → 内容]×4 → 结尾（含对比页、时间线页）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 7, 11, 15` | 内容 `4-6, 8-10, 12-14, 16-17` | 结尾 `18`
- **适用**：产品发布、产品介绍。视觉最丰富，每节独立配色

### market_analysis.xml — 市场分析报告

- 11 页 | 色调：dark | 正式度：formal
- 配色：黑底，蓝紫渐变 `#0d47db → #7d00fa` 点缀
- 结构：封面 → 目录 → 分节 → 数据 → 内容×2 → 分节 → 内容×3 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 7` | 内容 `5-6, 8-10` | 结尾 `11`
- **适用**：市场分析、行业报告。精简紧凑（11 页），蓝紫科技感

### business_case_analysis.xml — 商业案例分析

- 18 页 | 色调：dark | 正式度：formal
- 配色：深灰图片底图，金属银渐变 `#9c9c9e → #c0c5d3` 点缀
- 结构：封面 → 内容 → [分节页 → 内容]×4 → 结尾（4 个清晰分节）
- 页型索引（建议先读）：封面 `1` | 目录 `无` | 分节 `3, 7, 11, 15` | 内容 `2, 4-6, 8-10, 12-14, 16-17` | 结尾 `18`
- **适用**：商业案例、行业案例拆解。金属质感，分节明确

### product_promotion.xml — 产品推广方案

- 13 页 | 色调：dark | 正式度：formal
- 配色：深灰 `#2b2f36` + 白色交替，极简黑白
- 结构：封面 → 目录 → 内容×6 → 分节 → 内容×3 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `9` | 内容 `3-8, 10-12` | 结尾 `13`
- **适用**：推广方案、营销策略。极简风，黑白对比

### product_promotion_2.xml — 产品推广方案二

- 13 页 | 色调：light | 正式度：formal
- 配色：白底，深灰 `#2b2f36` 强调，图片裁切与几何色块组合
- 结构：封面 → 目录 → 内容×6 → 分节 → 内容×3 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `9` | 内容 `3-8, 10-12` | 结尾 `13`
- **适用**：产品推广、数字营销方案。比 `product_promotion.xml` 更偏浅色商务和图文展示

---

## marketing — 市场营销

### business_plan.xml — 商业计划书

- 18 页 | 色调：light | 正式度：formal
- 配色：白底，蓝青渐变封面 `#0b3be5 → #25d8ff`，鲜蓝 `#194cff` 点缀，含甜甜圈图表
- 结构：封面 → 目录 → 内容×12 → 时间线 → 内容×2 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-14, 16-17` | 结尾 `18`
- **适用**：商业计划书、创业路演。含时间线页、图表页

### roadshow_business_plan.xml — 企业路演商业计划书

- 20 页 | 色调：light | 正式度：formal
- 配色：白底，绿色 `#20b14e` 点缀（增长/商业感）
- 结构：封面 → 目录 → [分节页 → 内容]×5 → 结尾（含时间线、流程图）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 6, 9, 12, 15` | 内容 `4-5, 7-8, 10-11, 13-14, 16-19` | 结尾 `20`
- **适用**：融资路演、企业路演。绿色增长主题，含 chevron 流程图形

### brand_communication.xml — 品牌传播方案

- 17 页 | 色调：dark | 正式度：formal
- 配色：黑色图片底图，青绿渐变 `#59c2ff → #a1ffc8` 点缀，半透明白色卡片
- 结构：封面 → 目录 → 内容×8 → 数据 → 内容×5 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-10, 12-16` | 结尾 `17`
- **适用**：品牌传播、公关方案。青绿科技感 + 暗色背景

### marketing_strategy.xml — 市场营销策略

- 16 页 | 色调：dark | 正式度：creative
- 配色：纯黑底 `#0b0b0b`，红橙霓虹渐变 `#ff0000 → #fa6627`，紫蓝 `#bc50ff → #2f2bfd`
- 结构：封面 → 目录 → 内容×13 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-15` | 结尾 `16`
- **适用**：营销策略、品牌推广。最具视觉冲击力，霓虹风格

### marketing_plan.xml — 营销策划方案

- 15 页 | 色调：dark | 正式度：formal
- 配色：近黑底 `#18181a`，亮黄 `#ffdb1d` 强调，灰色 `#93969c` 文字
- 结构：封面 → 目录 → 内容×10 → 数据 → 内容 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-12, 14` | 结尾 `15`
- **适用**：营销策划、推广计划。黄+黑高对比，醒目

### product_whitepaper.xml — 产品白皮书

- 21 页 | 色调：colorful | 正式度：formal
- 配色：图片底图，暖橙粉渐变 `#ffaeac → #ffe6c8`，珊瑚 `#ff7d34 → #ff3ebd`
- 结构：封面 → 目录 → 内容×18 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-20` | 结尾 `21`
- **适用**：产品白皮书、技术白皮书、深度报告。页数最多（21 页），暖色调

### brand_operations_plan.xml — 品牌运营计划

- 18 页 | 色调：light | 正式度：formal
- 配色：浅灰底 `#eff2f6`，中蓝 `#3c6fe5` 点缀
- 结构：封面 → 目录 → [分节页 → 内容]×5 → 结尾（5 个分节，结构最清晰）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 6, 9, 12, 15` | 内容 `4-5, 7-8, 10-11, 13-14, 16-17` | 结尾 `18`
- **适用**：品牌运营、年度运营计划。蓝灰企业风，分节最规范

### brand_logo_design.xml — 品牌标志设计

- 16 页 | 色调：light | 正式度：formal
- 配色：近白底 `#fbfbfb`，亮蓝 `#1c66f6` 点缀，浅蓝卡片 `#c6e7ff`
- 结构：封面 → 目录 → 内容×7 → 数据 → 内容×4 → 数据 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-9, 11-14` | 结尾 `16`
- **适用**：品牌设计、VI 设计提案。清爽蓝白，专业设计感

---

## operations — 运营

### marketing_plan.xml — 营销策划方案

- 15 页 | 色调：dark | 正式度：formal
- 配色：近黑底 `#18181a`，亮黄 `#ffdb1d` 强调，灰色 `#93969c` 文字
- 结构：封面 → 目录 → 内容×10 → 数据 → 内容 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-12, 14` | 结尾 `15`
- **适用**：运营策划、营销活动方案。黄+黑高对比，醒目

### product_promotion.xml — 产品推广方案

- 13 页 | 色调：light | 正式度：formal
- 配色：白底，深灰 `#2b2f36` 强调，图片裁切与几何色块组合
- 结构：封面 → 目录 → 内容×6 → 分节 → 内容×3 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `9` | 内容 `3-8, 10-12` | 结尾 `13`
- **适用**：产品推广、运营增长方案。适合图文展示和分阶段推广计划

### brand_operations_plan.xml — 品牌运营计划

- 18 页 | 色调：light | 正式度：formal
- 配色：浅灰底 `#eff2f6`，中蓝 `#3c6fe5` 点缀
- 结构：封面 → 目录 → [分节页 → 内容]×5 → 结尾（5 个分节，结构最清晰）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 6, 9, 12, 15` | 内容 `4-5, 7-8, 10-11, 13-14, 16-17` | 结尾 `18`
- **适用**：品牌运营、年度运营计划。蓝灰企业风，分节规范

### brand_logo_design.xml — 品牌标志设计

- 16 页 | 色调：light | 正式度：formal
- 配色：近白底 `#fbfbfb`，亮蓝 `#1c66f6` 点缀，浅蓝卡片 `#c6e7ff`
- 结构：封面 → 目录 → 内容×7 → 数据 → 内容×4 → 数据 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-9, 11-14` | 结尾 `16`
- **适用**：品牌设计、运营视觉提案。清爽蓝白，专业设计感

---

## hr — 人力资源

### employee_training.xml — 员工培训

- 14 页 | 色调：dark | 正式度：formal
- 配色：近黑底 `rgba(13,20,32)`，图片分节页交替，单色简洁
- 结构：封面 → 目录 → [分节页(01-05) → 内容]×5 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 5, 7, 9, 11` | 内容 `4, 6, 8, 10, 12-13` | 结尾 `14`
- **适用**：员工培训、规章制度宣讲。暗色严肃风，5 个章节分节页

### employee_training_workshop.xml — 员工培训训练

- 19 页 | 色调：colorful | 正式度：casual
- 配色：暖米底 `rgba(247,245,240)` + 深灰 `rgba(60,60,60)` 交替，5 种彩色点缀（钢蓝、青绿、橄榄金、鲑鱼粉）
- 结构：封面 → 目录 → [深色标题卡 → 浅色内容]×7 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 5, 7, 9, 11, 13, 15` | 内容 `4, 6, 8, 10, 12, 14, 16-18` | 结尾 `19`
- **适用**：培训工作坊、技能培训。温暖多彩，轻松氛围

### onboarding.xml — 新人入职培训

- 15 页 | 色调：dark | 正式度：formal
- 配色：深炭灰底 `rgba(43,47,54)`，亮紫 `rgba(75,63,221)` 点缀，浅灰卡片 `rgba(243,244,246)`
- 结构：封面 → 目录 → [分节页(01-05) → 内容]×5 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 5, 7, 9, 11` | 内容 `4, 6, 8, 10, 12-14` | 结尾 `15`
- **适用**：新人入职、公司介绍。紫色 CTA 按钮，包含公司概况/愿景/价值观/流程/制度等章节

---

## administration — 行政

### all_hands_meeting.xml — 全员大会

- 18 页 | 色调：light | 正式度：formal
- 配色：蓝紫渐变图片底图，毛玻璃白色卡片（alpha=0.7），薰衣草 `rgba(155,177,255)` 点缀
- 结构：封面 → 目录 → [分节页 → 内容]×4 → 结尾（含图表页）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 7, 11, 15` | 内容 `4-6, 8-10, 12-14, 16-17` | 结尾 `18`
- **适用**：全员大会、季度会议、公司大会。毛玻璃风格，蓝紫渐变，含 chart 图表

### company_intro.xml — 企业介绍

- 16 页 | 色调：light | 正式度：formal
- 配色：白底极简，金黄 `rgba(255,196,25)` 点缀，深灰 `rgba(31,35,41)` 文字
- 结构：封面 → 目录 → 内容×12 → 数据(chart) → 结尾（含图表页）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-14` | 结尾 `16`
- **适用**：企业介绍、公司宣传。极简白底，中英双语，含行业前景图表

### corporate_culture.xml — 企业文化宣传

- 17 页 | 色调：dark | 正式度：formal
- 配色：深炭灰底 `rgba(31,35,41)`，橙红 `rgba(255,104,68)` 强调，浅灰 `rgba(246,246,246)` 内容区
- 结构：封面 → 前言 → 目录 → [分节页 → 内容]×4 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `3` | 分节 `4, 7, 10, 13` | 内容 `2, 5-6, 8-9, 11-12, 14-16` | 结尾 `17`
- **适用**：企业文化、公司愿景宣传。含前言页、组织架构页，橙红活力色

### annual_gala.xml — 年度盛典

- 16 页 | 色调：dark | 正式度：creative
- 配色：近黑底 `rgba(16,15,21)`，紫蓝青多色渐变 `rgba(210,132,232) → rgba(69,96,245)` 霓虹风
- 结构：封面 → 目录 → [分节页 → 内容]×6 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 5, 7, 9, 11, 13` | 内容 `4, 6, 8, 10, 12, 14-15` | 结尾 `16`
- **适用**：年会、庆典、颁奖典礼。多彩霓虹渐变，节日庆典感，含 LOGO 占位

---

## personal — 个人

### experience_sharing.xml — 经验分享

- 20 页 | 色调：light | 正式度：casual
- 配色：图片底图，亮蓝 `rgba(52,113,252)` 点缀，毛玻璃白色卡片（alpha=0.7）
- 结构：封面 → 目录 → 内容×17 → 结尾（含 table 表格页、前后对比布局）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-19` | 结尾 `20`
- **适用**：经验分享、项目复盘分享。含步骤流程页、数据指标展示、表格

### teaching_sharing.xml — 分享教学

- 20 页 | 色调：colorful | 正式度：casual
- 配色：插画风图片底图，蓝紫 `rgba(128,145,239)` + 暖橙 `rgba(242,170,104)` 双色调
- 结构：封面 → 目录 → 内容×17 → 结尾（含 Demo 案例、痛点分析、推荐书单）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-19` | 结尾 `20`
- **适用**：教学分享、技术分享、读书会。温暖蓝橙配色，含 chevron 流程图

### personal_resume.xml — 个人简历

- 16 页 | 色调：dark | 正式度：formal
- 配色：深炭灰图片底图，暖橙 `rgba(253,151,51)` + 霓虹绿 `rgba(76,241,29)` 渐变点缀
- 结构：封面(含联系方式) → 目录 → [分节页 → 内容]×4 → 结尾
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `3, 6, 9, 12` | 内容 `4-5, 7-8, 10-11, 13-15` | 结尾 `16`
- **适用**：个人简历、求职展示。橙绿双色点缀，含个人信息字段

### promotion_report.xml — 职位晋升汇报

- 15 页 | 色调：dark | 正式度：formal
- 配色：纯黑底 `rgba(3,3,1)`，薰衣草渐变 `rgba(216,216,255) → rgba(171,194,255)` 点缀，白色文字
- 结构：封面 → 目录 → 内容×12 → 结尾（中英双语标题）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-14` | 结尾 `15`
- **适用**：晋升汇报、述职报告。暗色稳重 + 薰衣草柔和点缀

### promotion_defense.xml — 晋级答辩

- 11 页 | 色调：light | 正式度：formal
- 配色：白底，亮紫 `rgba(74,58,255)` 点缀
- 结构：封面 → 目录 → 内容×8 → 结尾（紧凑无分节页）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-10` | 结尾 `11`
- **适用**：晋级答辩、技术答辩。最精简（11 页），紫色点缀，中英双语

### self_intro.xml — 个人介绍

- 11 页 | 色调：colorful | 正式度：casual
- 配色：暖白底 `rgba(248,247,246)`，5 种彩色（红 `#d83931`、青 `#17b69c`、橙 `#ec914e`、黄 `#fad355`、蓝 `#3347ff`）
- 结构：封面 → 内容×9 → 结尾（含表格、甜甜圈图表、饼图）
- 页型索引（建议先读）：封面 `1` | 目录 `无` | 分节 `无` | 内容 `2-10` | 结尾 `11`
- **适用**：个人介绍、设计师作品集。shape 类型最丰富，图表元素多

---

## misc — 其他

### book_sharing.xml — 读书分享

- 16 页 | 色调：light | 正式度：casual
- 配色：白底 + 深橄榄绿侧边栏 `rgba(43,54,44)`，棕褐 `rgba(128,66,8)` 点缀
- 结构：封面 → 内容×14 → 结尾（中文传统编号：壹贰叁肆）
- 页型索引（建议先读）：封面 `1` | 目录 `无` | 分节 `无` | 内容 `2-15` | 结尾 `16`
- **适用**：读书分享、文学赏析。文艺复古风，装饰性诗句分隔，中国传统美学

### club_event_plan.xml — 社团活动策划

- 15 页 | 色调：dark | 正式度：casual
- 配色：纯黑底，红色 `rgba(216,57,49)` + 金色 `rgba(219,168,0)` 双色强调
- 结构：封面 → 目录 → 内容×12 → 结尾（含预算页、应急方案页）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-14` | 结尾 `15`
- **适用**：活动策划、社团方案。黑红金配色，含预算/应急等实用页面

### student_career_plan.xml — 学生职业规划

- 15 页 | 色调：colorful | 正式度：casual
- 配色：深靛蓝底 `rgba(21,20,87)`，霓虹多彩（品红、青、黄、粉、热粉）
- 结构：封面 → 目录 → 内容×12 → 结尾（含 SWOT 分析、短/中/长期目标时间线）
- 页型索引（建议先读）：封面 `1` | 目录 `2` | 分节 `无` | 内容 `3-14` | 结尾 `15`
- **适用**：职业规划、学生汇报。最鲜艳多彩，霓虹风格，含 SWOT 和时间线

---

## 快速匹配索引

### 按场景

| 场景关键词 | 推荐模板（优先级从高到低） |
|-----------|------------------------|
| 工作汇报/月报/周报 | `office--work_report` > `office--work_summary` > `office--light_general` |
| 工作总结/年度总结 | `office--work_summary_report` > `office--work_summary` |
| 季度复盘/OKR | `office--quarterly_review` > `office--work_summary_report` |
| 部门年度汇报 | `office--dept_annual_report` > `office--work_summary_report` |
| 项目启动/技术宣讲 | `office--project_kickoff` |
| 产品介绍/产品发布 | `product--product_intro` > `product--product_analysis` |
| 产品分析/竞品分析 | `product--product_analysis` |
| 市场分析/行业报告 | `product--market_analysis` |
| 商业案例 | `product--business_case_analysis` |
| 商业计划书/创业 | `marketing--business_plan` > `marketing--roadshow_business_plan` |
| 融资路演/企业路演 | `marketing--roadshow_business_plan` |
| 品牌传播/公关 | `marketing--brand_communication` |
| 营销策略/推广 | `marketing--marketing_strategy` > `marketing--marketing_plan` |
| 产品白皮书 | `marketing--product_whitepaper` |
| 品牌运营/年度计划 | `marketing--brand_operations_plan` |
| 品牌设计/VI | `marketing--brand_logo_design` |
| 员工培训/制度宣讲 | `hr--employee_training` > `hr--employee_training_workshop` |
| 新人入职 | `hr--onboarding` |
| 全员大会/公司大会 | `administration--all_hands_meeting` |
| 企业介绍/公司宣传 | `administration--company_intro` |
| 企业文化 | `administration--corporate_culture` |
| 年会/庆典/颁奖 | `administration--annual_gala` |
| 经验分享/项目分享 | `personal--experience_sharing` |
| 教学分享/技术分享 | `personal--teaching_sharing` |
| 个人简历/求职 | `personal--personal_resume` |
| 晋升汇报/述职 | `personal--promotion_report` > `personal--promotion_defense` |
| 个人介绍 | `personal--self_intro` |
| 读书分享 | `misc--book_sharing` |
| 活动策划 | `misc--club_event_plan` |
| 职业规划 | `misc--student_career_plan` |

### 按色调

| 色调 | 模板 |
|------|------|
| **light** | `office--light_general` ⭐, `office--work_report`, `office--work_summary`, `product--product_analysis`, `marketing--business_plan`, `marketing--roadshow_business_plan`, `marketing--brand_operations_plan`, `marketing--brand_logo_design`, `administration--all_hands_meeting`, `administration--company_intro`, `personal--experience_sharing`, `personal--promotion_defense`, `misc--book_sharing` |
| **dark** | `office--dark_general` ⭐, `office--work_summary_report`, `office--quarterly_review`, `office--dept_annual_report`, `office--project_kickoff`, `product--market_analysis`, `product--business_case_analysis`, `product--product_promotion`, `marketing--brand_communication`, `marketing--marketing_strategy`, `marketing--marketing_plan`, `hr--employee_training`, `hr--onboarding`, `administration--corporate_culture`, `administration--annual_gala`, `personal--personal_resume`, `personal--promotion_report`, `misc--club_event_plan` |
| **colorful** | `product--product_intro`, `marketing--product_whitepaper`, `hr--employee_training_workshop`, `personal--self_intro`, `personal--teaching_sharing`, `misc--student_career_plan` |
