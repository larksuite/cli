# 文档样式指南

创建或编辑文档时，必须遵循本指南，使用结构化 block 提升可读性和视觉层次。

## 一、核心原则

1. **结构优于文字**：能用结构化 block 表达的信息，不用纯文本段落
2. **Front-load 结论**：文档以 `<callout>` 开头概括核心结论；每章节首段点明要旨
3. **视觉节奏**：连续纯文本不超过 3 段；不同主题章节间用 `<hr/>` 分隔
4. **最少惊讶**：同类信息使用同类元素，全篇风格统一
5. **重要信息画板化**：核心流程、架构、对比、风险、路线图、指标趋势等重要信息优先使用画板表达

## 二、元素选择指南

涉及图表需求时，先判定简单/复杂：简单图启动 SubAgent 直接插入 `<whiteboard type="svg">完整 SVG</whiteboard>`，不读取 **lark-whiteboard**；复杂图才使用空白画板 + **lark-whiteboard** SubAgent。

| 场景 | 推荐方案                                                          |
|-|---------------------------------------------------------------|
| 核心结论 / 摘要 / 注意事项 | `<callout>` + emoji + 背景色                                     |
| 重要方案对比 / 优劣势 / Before vs After | `<grid>` 2 列分栏；简单 SVG SubAgent；复杂矩阵用 lark-whiteboard SubAgent |
| 简短低风险对比 | `<grid>` 2 列分栏                                                |
| 3+ 属性的结构化数据 / 指标表 | `<table>` + 表头背景色                                             |
| 任务清单 / 检查项 | `<checkbox>`                                                  |
| 代码片段 | `<pre lang="x" caption="说明">`                                 |
| 引用 / 公式 | `<blockquote>` / `<latex>`                                    |
| 操作入口 / 跳转链接 | `<button>` / `<a type="url-preview">`                         |
| 简单流程图 / 小型状态机 / 小型时间线 | 简单 SVG SubAgent                                               |
| 简单自定义图形 / 小型 SVG 示意图 | 简单 SVG SubAgent                                               |
| 复杂架构图 / 数据图 / 思维导图 / 组织架构 | 空白画板 + lark-whiteboard SubAgent                               |

### 画板意图识别

撰写或审查每个段落/章节时，**必须判断该内容是否适合用图表达**。满足以下任一特征时，应使用画板而非纯文本；如果该内容承载章节核心结论、关键决策或主要论据，即使结构较简单也优先画板化：

| 内容特征 | 信号词 / 模式 | 推荐画板类型 |
|-|-|-|
| 多步骤的操作流程或决策路径 | "先…然后…最后"、"步骤 1/2/3"、"如果…则…否则" | 流程图 / 泳道图 |
| 系统或模块间的依赖与交互 | "调用"、"依赖"、"上游/下游"、"请求→响应" | 架构图 |
| 上下级或从属关系 | "汇报给"、"下属"、"隶属"、"团队结构" | 组织架构图 |
| 时间线或阶段演进 | "Q1/Q2"、"里程碑"、"阶段一→阶段二"、日期序列 | 时间线 / 里程碑 |
| 因果分析或问题归因 | "根因"、"原因"、"导致"、"影响因素" | 鱼骨图 |
| 两个及以上方案/对象的多维度对比 | "vs"、"方案 A/B"、"优劣"、"对比" | 对比图 |
| 层级递进或优先级排序 | "基础→进阶→高级"、"L1/L2/L3"、"核心→外围" | 金字塔图 |
| 数值趋势或周期变化 | 带数字的时间序列、"增长/下降"、百分比变化 | 折线图 / 柱状图 |
| 漏斗或转化率 | "转化率"、"漏斗"、"从…到…留存" | 漏斗图 |
| 发散或归纳的思维结构 | "要点"、"维度"、"分支"、多层嵌套列表 | 思维导图 |
| 循环或飞轮效应 | "正循环"、"飞轮"、"闭环"、"A 驱动 B 驱动 C" | 飞轮图 |
| 占比分布 | "占比"、"份额"、"分布"、百分比加总 ≈100% | 饼图 / 树状图 |

**判断规则：**
- 重要信息能图示就图示；不要为了省步骤把关键流程、架构、对比、风险链路写成纯文本
- 简单图由 SubAgent 直接插入 `<whiteboard type="svg">完整 SVG</whiteboard>`，不读取 **lark-whiteboard**
- 复杂图或已有画板更新才先插入 `<whiteboard type="blank"></whiteboard>`，再启动 SubAgent 使用 **lark-whiteboard** skill 写入内容
- 低重要度、局部辅助信息才用 `<table>` / `<grid>` / `<callout>` 承载

### 画板语法与插入

> **提醒：** `docs +update` 不能编辑已有画板内容；下面的语法都是**新增**画板块。修改已有画板需启动 SubAgent 读取 [`lark-whiteboard`](../../../lark-whiteboard/SKILL.md)。

#### 简单 SVG 画板（SubAgent 插入）

1. 主 Agent 启动 SubAgent，传入 doc token、插入位置、图表目标和源内容
2. SubAgent 使用 `<whiteboard type="svg">完整自包含 SVG</whiteboard>` 通过 `docs +create --api-version v2` / `docs +update --api-version v2` 插入
3. SVG 必须包含 `<svg>` 根节点和 `viewBox`，不要引用外部图片、脚本或远程资源

#### 复杂画板（空白画板 + lark-whiteboard SubAgent）

1. 用 `<whiteboard type="blank"></whiteboard>` 通过 `docs +create --api-version v2` / `docs +update --api-version v2` 插入空白画板
2. 从 v2 响应 `data.document.new_blocks` 中提取画板 `block_token`
3. 必须启动 SubAgent，把 `block_token`、图表目标、推荐画板类型和源内容交给它
4. SubAgent 读取 [`lark-whiteboard`](../../../lark-whiteboard/SKILL.md) skill 并写入该画板；主 Agent 不直接调用画板渲染流程

更完整的协同流程见 [`lark-doc-whiteboard.md`](../lark-doc-whiteboard.md)。

## 三、颜色语义

全篇保持语义一致，同一语义必须使用同一颜色：

| 语义 | emoji 前缀 | callout 背景色 | 文字色 |
|-|-|-|-|
| 信息、说明 | ℹ️ "说明：" | `light-blue` | `blue` |
| 成功、推荐 | ✅ "推荐：" | `light-green` | `green` |
| 警告 / 错误 / 风险 | ⚠️❌ | `light-red` | `red` |
| 注意、待确认 | ❗"注意：" | `light-yellow` | `yellow` |
| 中性、辅助 | — | `light-gray` | — |

- 表头统一 `background-color="light-gray"`
- 关键指标用 `<span text-color="green/red">` 突出，**必须同时用 ↑↓ 或 +/- 标注方向**（色觉无障碍）

## 四、排版规范

- 标题层级 ≤ 4 层，段落单段 ≤ 5 行，列表嵌套 ≤ 2 层，Grid ≤ 3 列
- 文档开头用 `<callout>` front-load 结论；

## 五、丰富度自检

生成内容后必须自检，**未达标时主动优化**：

| 指标 | 达标标准 |
|-|-|
| 富 block 密度 | ≥ 40%（非纯文本 block 数 ÷ 总 block 数） |
| 元素多样性 | ≥ 3 种不同 block 类型 |
| 连续纯文本 | ≤ 3 段连续 `<p>` |
| 章节丰富度 | 每 h1/h2 ≥ 1 个非纯文本 block |
| 开头 callout | 必须 |
| 视觉节奏 | 不同主题章节间有 `<hr/>` |
