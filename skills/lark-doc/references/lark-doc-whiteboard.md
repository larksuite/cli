# lark-doc 画板处理指南

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

## 两个 Skill 的职责边界

| Skill | 核心职责 | 约束 |
|------|------|------|
| `lark-doc` | 识别画板机会、判断简单/复杂、调度 SubAgent、插入简单 SVG 画板或复杂空白画板 | 主 Agent 不直接创作画板内容；简单图不需要读取 `lark-whiteboard` |
| `lark-whiteboard` | 查询/导出已有画板；复杂图表生成（Mermaid/DSL/SVG 路由、场景选型、渲染验证）；写入已有/空白画板 | 仅复杂图或已有画板更新时由独立 SubAgent 读取 |

## 画板优先规则

写文档时，重要信息优先画板化。遇到核心流程、系统架构、方案对比、风险链路、里程碑、指标趋势、因果归因、组织关系、能力分层等内容，不要只用段落或表格承载；除非内容只是一次性补充说明，否则应规划为画板。

同一篇文档可以有多个画板。优先多个聚焦画板，而不是把所有信息塞进一张大图。

## 文档与画板协同流程

### 步骤 1：识别画板机会

| 场景 | 入口 |
|------|------|
| 文档中需要插入简单新画板 | 走步骤 2A |
| 文档中需要插入复杂新画板 | 走步骤 2B |
| 已有画板需要更新内容 | 先 `docs +fetch --api-version v2` 获取 `board_token`，跳至步骤 3B |
| 只查看 / 下载已有画板 | 切换至 `lark-whiteboard`，不走本流程 |

简单图判定：节点少、静态、布局可控、适合一个完整自包含 SVG 表达，例如小型流程、2-3 方对比、小型状态机、简单时间线或小型示意图。

复杂图判定：节点多、跨泳道/跨系统、需要自动布局或精细排版、包含数据图表、组织架构、复杂架构、复杂依赖、已有画板更新，或需要 `lark-whiteboard` 的渲染验证。

### 步骤 2A：简单图 — SubAgent 直接插入 SVG 画板

主 Agent 启动 SubAgent，让它用 `docs +create --api-version v2` / `docs +update --api-version v2` 插入：

```xml
<whiteboard type="svg"><svg ...>...</svg></whiteboard>
```

简单图 SubAgent 的最小上下文：
- doc token、插入位置（标题 / block_id / command）
- 图表目标、受众、源段落或数据
- 要求读取 `lark-doc-xml.md`；不需要读取 `lark-whiteboard`
- SVG 必须完整自包含：包含 `<svg>` 根节点和 `viewBox`，不引用外部图片、脚本、远程资源

### 步骤 2B：复杂图 — 先创建空白画板

- 主 Agent 使用 `docs +create --api-version v2` / `docs +update --api-version v2` 插入 `<whiteboard type="blank"></whiteboard>`。
- 从 v2 响应的 `data.document.new_blocks[]` 中读取 `block_type == "whiteboard"` 的 `block_token` 作为 board_token。

### 步骤 3B：复杂图或已有画板 — 启动 lark-whiteboard SubAgent

复杂图和已有画板更新必须启动 SubAgent。主 Agent 只传最小上下文，不直接执行 `lark-whiteboard` 的渲染和写入流程。

复杂图 SubAgent 的最小上下文：
- board_token
- 图表目标、推荐画板类型、受众
- 与图表直接相关的源段落或数据
- 要求读取 [`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)，按其完整流程写入该 board_token

多个画板互不依赖时，可并行启动多个 SubAgent；每个 SubAgent 只负责一个画板或一个 SVG 插入点，不要互相复用上下文。

### 步骤 4：完成校验

- 简单 SVG：确认插入的是 `<whiteboard type="svg">`，且内容是完整 `<svg ...>...</svg>`
- 复杂画板：确认每个 token 对应的画板都已填充真实内容
- 不保留空白占位画板；复杂路径只有空白画板而无内容视为任务未完成

---

## 语义与画板类型映射

下表用于帮助主 Agent 判断简单/复杂路径，并给 SubAgent 指定推荐画板类型。

| 语义 | 画板类型 |
|------|------|
| 小型流程/状态机/简单时间线/小型对比/小型示意图 | SVG 画板（简单路径） |
| 架构/分层/技术方案/模块依赖/调用关系 | 架构图（复杂路径） |
| 流程/审批/部署/业务流转/状态机 | 流程图（按复杂度分流） |
| 跨角色流程/跨系统交互/端到端链路 | 泳道图（复杂路径） |
| 组织/层级/汇报关系 | 组织架构图 |
| 时间线/里程碑/版本规划 | 里程碑图 |
| 因果/复盘/根因分析 | 鱼骨图 |
| 方案对比/技术选型/功能矩阵 | 对比图 |
| 循环/飞轮/闭环/增长链路 | 飞轮图 |
| 层级占比/能力模型/需求层次 | 金字塔图 |
| 矩形树图/层级面积占比 | 树状图 |
| 转化漏斗/销售漏斗 | 漏斗图 |
| 分类梳理/知识体系/思维导图/时序图/类图 | Mermaid |
| 数据分布/占比/饼图 | Mermaid |
| 简单自定义图形/小型 SVG 示意图 | SVG 画板（简单路径） |
| 柱状图/条形图/数据对比 | 柱状图 |
| 折线图/趋势图/时序数据 | 折线图 |

---

## 关联参考

- 画板查询/创作/修改/渲染写入：[`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)
