# lark-doc 画板处理指南

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

## 两个 Skill 的职责边界

| Skill | 核心职责 | 约束 |
|------|------|------|
| `lark-doc` | 文档内容读取/更新、插入空白画板占位、获取 board_token | 不能直接编辑画板内容；`docs +update` 的画板能力仅限插入空白占位 |
| `lark-whiteboard` | 查询/导出画板（+query）；图表内容生成（Mermaid/DSL/SVG 路由、场景选型、渲染验证）；写入画板（+update） | 所有图表内容生成必须委派给此 skill 的 subAgent，主 agent 不得自行生成 |

## 文档与画板协同流程

### 步骤 1：判断场景

| 场景 | 入口 |
|------|------|
| 文档中需要插入新画板 | 继续步骤 2 |
| 已有画板需要更新内容 | 先 `docs +fetch` 获取 `board_token`，跳至步骤 3 |
| 只查看 / 下载已有画板 | 切换至 `lark-whiteboard`，不走本流程 |

### 步骤 2：在文档中创建空白画板

- 创建场景：`docs +create`；编辑场景：`docs +update`
- markdown 中使用 `<whiteboard type="blank"></whiteboard>`（不要转义）
- 多个画板时，在同一段 markdown 中重复多个 whiteboard 标签
- 从响应的 `data.board_tokens` 中读取 token 列表

### 步骤 3：委派 subAgent 生成并写入画板内容

**每个 board_token 启动一个独立 subAgent**，主 agent 不直接执行画板生成管线。

按"subAgent 委派规范"组装输入，并行（或顺序）调用，汇总结果。

> ⚠️ **CRITICAL：主 Agent 绝对禁止**自行编写任何图表代码并直接调用 `whiteboard +update`。必须 100% 委派给 `lark-whiteboard` subAgent，subAgent 会跳至"渲染 & 写入画板"章节执行。

### 步骤 4：完成校验

- 确认每个 token 对应的画板都已填充真实内容
- 不保留空白占位画板；只有空白画板而无内容视为任务未完成

---

## 语义与画板类型映射

| 语义 | 画板类型 |
|------|------|
| 架构/分层/技术方案/模块依赖/调用关系 | 架构图 |
| 流程/审批/部署/业务流转/状态机 | 流程图 |
| 跨角色流程/跨系统交互/端到端链路 | 泳道图 |
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
| 柱状图/条形图/数据对比 | 柱状图 |
| 折线图/趋势图/时序数据 | 折线图 |

---

## subAgent 委派规范

### 输入契约（4 项，缺一不可）

主 agent 为**每个** board_token 单独组装以下 4 项输入：

| # | 字段 | 内容要求 |
|---|------|---------|
| 1 | **画板编号** | 整数（从 1 开始），用于产物目录命名（`./diagrams/board_{n}/`） |
| 2 | **文档背景** | 1–3 句话，说明文档主题、目标读者、场景 |
| 3 | **画板内容** | 图表类型 + 关键元素 + 元素间关系 + 具体文字/数据（越具体越好） |
| 4 | **board_token** | 目标画板的 token 值（主 agent 从 `data.board_tokens` 取得） |

### subAgent Prompt 模板

```
你是一个专注于飞书画板内容生成与写入的 agent，使用最强可用模型执行。

## 任务

为第 {{board_index}} 号画板生成并写入图表内容。

## 文档背景

{{doc_background}}

## 画板内容要求

- 图表类型：{{chart_type}}
- 关键元素：{{key_elements}}
- 元素间关系：{{relationships}}
- 具体文字/数据：{{text_data}}

## 目标画板

board_token：{{board_token}}

## 执行指引

1. 读取 `skills/lark-whiteboard/SKILL.md`，跳至"渲染 & 写入画板"章节，按其完整流程执行
2. 产物目录使用 `./diagrams/board_{{board_index}}/`（固定编号，非时间戳）
3. 生成内容后按 SKILL.md 的"上传飞书画板"章节将内容写入上方 board_token
4. 完成后返回：`{ "board_token": "{{board_token}}", "status": "ok" }`
   若失败：`{ "board_token": "{{board_token}}", "status": "failed", "error": "原因" }`

## 约束

- 只操作画板，不修改文档主体
- 渲染 2 轮后仍有严重问题 → 记录 error，不要无限重试
- 不得在未写入真实内容的情况下返回 ok
```

### 调度策略

| 画板数量 | 调度方式 |
|----------|---------|
| 1 个 | 直接调用单个 subAgent |
| 多个 + 框架支持并行 | 同时发起所有 subAgent（推荐） |
| 多个 + 框架不支持并行 | 按编号顺序串行调用 |

**跨框架适配：**

```
Claude Code   → Task tool（Agent mode）
Copilot Agent → runSubagent
Cursor        → parallel tool calls 或 sequential subagent
其他框架       → 使用平台提供的子任务/并发能力
```

### 主 agent 汇总逻辑

```
所有 subAgent 完成后：
  全部 ok    → 告知用户文档与画板均已完成
  部分 failed → 展示失败的 board_token 和原因，建议用户补充更多细节后重试
  注意：即使全部失败，文档本体（含空白画板占位）已创建，需如实告知用户
```

---

## 关联参考

- 画板查询/创作/修改/渲染写入：[`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)
