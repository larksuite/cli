---
name: lark-base
version: 1.2.0
description: "当需要用 lark-cli 操作飞书多维表格（Base）时调用：适用于建表、字段管理、记录读写、视图配置、历史查询，以及角色/表单/仪表盘管理/工作流；也适用于把旧的 +table / +field / +record 写法改成当前命令写法。涉及字段设计、公式字段、查找引用、跨表计算、行级派生指标、数据分析需求时也必须使用本 skill。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli base --help"
---

# base

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)。
> **执行前必做：** 执行任何 `base` 命令前，必须先阅读对应命令的 reference 文档，再调用命令。
> **命名约定：** 仅使用 `lark-cli base +...` 形式的命令。
> **分流规则：** 如果用户要“把本地文件导入成 Base / 多维表格 / bitable”，第一步不是 `base`，而是 `lark-cli drive +import --type bitable`。只有导入完成后，才回到 `lark-cli base +...` 做表内操作。

## Agent 快速执行顺序

1. **先判断任务类型**
   - 本地文件导入成 Base / 多维表格 / bitable → 先切 `lark-cli drive +import --type bitable`
   - 临时统计 / 聚合分析 → `+data-query`
   - 要把结果长期显示在表里 → formula 字段
   - 用户明确要 lookup，或确实更适合 `from/select/where/aggregate` → lookup 字段
   - 明细读取 / 关键词检索 / 导出 → `+record-search / +record-list / +record-get`
2. **先拿结构，再写命令**
   - 至少先拿当前表结构：`+field-list` 或 `+table-get`
   - 跨表场景必须再查**目标表**的结构
3. **formula / lookup 有硬门槛**
   - 先读对应 guide
   - 读完 guide 后，再创建对应字段
4. **写记录前先判断字段可写性**
   - 只写存储字段
   - 系统字段 / formula / lookup 默认只读

## Agent 禁止行为

- 不要把 `+record-list / +record-search` 当聚合分析引擎
- 不要没读 guide 就直接创建 formula / lookup 字段
- 不要凭自然语言猜表名、字段名、公式表达式里的字段引用
- 不要把系统字段、formula 字段、lookup 字段当成 `+record-upsert / +record-batch-create / +record-batch-update` 的写入目标
- 不要把“本地 Excel / CSV 导入成 Base”误判成 `+base-create`、`+table-create` 或 `+record-upsert`；这一步必须先走 `lark-cli drive +import --type bitable`
- 不要在 Base 场景改走 `lark-cli api GET /open-apis/bitable/v1/...`
- 不要因为 wiki 解析结果里的 `obj_type=bitable` 就去找 `bitable.*`；在本 CLI 里应继续使用 `lark-cli base +...`

## Base 基本心智模型

1. **Base 字段分三类**
   - **存储字段**：真实存用户输入的数据，通常适合 `+record-upsert / +record-batch-create / +record-batch-update` 写入，例如文本、数字、日期、单选、多选、人员、关联。**附件字段例外**：对 agent 而言，文件上传必须走 `+record-upload-attachment`。
   - **系统字段**：平台自动维护，只读，典型包括创建时间、最后更新时间、创建人、修改人、自动编号。
   - **计算字段**：通过表达式或跨表规则推导，只读，典型包括 **公式字段（formula）** 和 **查找引用字段（lookup）**。
2. **写记录前先判断字段类别** — 只有存储字段可直接写；公式 / lookup / 创建时间 / 更新时间 / 创建人 / 修改人 / 自动编号都应视为只读输出字段，不能拿来做 `+record-upsert / +record-batch-create / +record-batch-update` 入参。
3. **Base 不只是存表数据，也能内建计算** — 用户提出“统计、比较、排名、文本拼接、日期差、跨表汇总、状态判断”等需求时，不能默认导出数据后手算；要先判断是否应通过 `+data-query` 或公式字段在 Base 内完成。

## 分析路径决策

1. **一次性分析 / 临时查询** → 优先 `+data-query`
   - 适合：分组统计、SUM / AVG / COUNT / MAX / MIN、条件筛选后聚合。
   - 特征：要的是“这次算出来的结果”，不是把结果沉淀成表内字段。
2. **长期复用的派生指标 / 行级计算结果** → 优先公式字段
   - 适合：利润率、是否延期、剩余天数、分档标签、跨表汇总后的派生结果。
   - 特征：要把结果长期显示在 Base 里，跟随记录自动更新。
3. **显式要求 Lookup，或确实要按 source/select/where/aggregate 建模** → 用 lookup 字段
   - 默认仍优先考虑 formula。lookup 只在用户明确要求、或更符合固定查找配置时使用。
4. **原始记录读取 / 关键词检索 / 明细导出** → `+record-search / +record-list / +record-get`
   - 不要把 `+record-list / +record-search` 当分析引擎；它们负责取明细，不负责聚合计算。

## 公式 / Lookup 专项规则

1. **涉及 formula / lookup 时，先读 guide，再出命令**
   - formula：[`formula-field-guide.md`](references/formula-field-guide.md)
   - lookup：[`lookup-field-guide.md`](references/lookup-field-guide.md)
2. **guide 先于创建命令**
   - 没读对应 guide 前，不要直接创建 formula / lookup 字段
   - 读完 guide 后，再补齐对应 JSON 并创建字段
   - `type=formula` 必须提供 `expression`
   - `type=lookup` 必须提供 `from / select / where`，必要时补 `aggregate`
3. **公式字段优先于 lookup 字段**
   - 只要用户的诉求是“计算 / 条件判断 / 文本处理 / 日期差 / 跨表聚合 / 跨表筛选后取值”，默认优先尝试 formula。
   - 只有用户明确说要 lookup，或配置天然更适合 lookup 四元组时，再走 lookup。
4. **表名 / 字段名必须精确匹配**
   - 公式、lookup、data-query 中出现的表名 / 字段名，必须来自 `+table-list` / `+table-get` / `+field-list` 的真实返回，禁止凭语义猜测改写。
5. **先拿结构再写表达式**
   - 公式或 lookup 一律先获取相关表结构，再生成表达式 / 配置；不要直接凭用户口述拼字段名。

## Workflow 专项规则

1. **执行任何 workflow 命令前，必须先读对应的命令文档**
   - **创建**: 先读 [workflow-create.md](references/lark-base-workflow-create.md)（它会引导你读 schema）
   - **修改**: 先读 [workflow-update.md](references/lark-base-workflow-update.md) + [workflow-get.md](references/lark-base-workflow-get.md) 获取现有配置
   - **查询/批量操作**: 先读 [workflow-list.md](references/lark-base-workflow-list.md)（注意场景适用性）
   - **获取详情**: 先读 [workflow-get.md](references/lark-base-workflow-get.md)，用于获取完整 workflow 定义（含 steps 结构）
   - **启用**: 先读 [workflow-enable.md](references/lark-base-workflow-enable.md)，启用处于 `disabled` 状态的工作流
   - **禁用**: 先读 [workflow-disable.md](references/lark-base-workflow-disable.md)，禁用处于 `enabled` 状态的工作流
2. **禁止凭自然语言猜测 `type` 值**
   - 必须从 schema 的 StepType 枚举中复制准确的类型名称
   - 如 `AddRecordTrigger` 不是 `CreateTrigger`
3. **创建前确认依赖信息**
   - 通过 `+table-list` / `+field-list` 获取真实的表名、字段名
   - 禁止凭自然语言猜测表名/字段名填入 workflow 配置
4. **workflow_id 与 table_id 区分**
   - `workflow_id` 以 `wkf` 开头，从 `+workflow-list` 或 URL 的 `?table=wkf...` 参数获取
   - `table_id` 以 `tbl` 开头，两者在 URL 的 `?table=` 参数里都会出现，需根据前缀判断
5. **启用/禁用为写入操作**
   - `+workflow-enable` / `+workflow-disable` 执行前必须向用户确认 base-token 和 workflow-id

## Dashboard（仪表盘/数据看板）模块
**当用户提到 "仪表盘、dashboard、数据看板、图表、可视化、block、组件、添加组件、创建图表" 等仪表盘相关的关键词时，必须阅读** [lark-base-dashboard.md](references/lark-base-dashboard.md) 这个指引文档，了解仪表盘模块的命令和能力后再进行后续操作。

## 核心规则

1. **只使用原子命令** — 使用 `+table-list / +table-get / +field-create / +record-upsert / +record-batch-create / +record-batch-update / +view-set-filter / +record-history-list / +base-get` 这类一命令一动作的写法，不使用旧聚合式 `+table / +field / +record / +view / +history / +workspace`
2. **写记录前先读字段结构** — 先调用 `+field-list` 获取字段结构，再读 [lark-base-shortcut-record-value.md](references/lark-base-shortcut-record-value.md) 确认各字段类型的写入值格式
3. **写字段前先看字段属性规范** — 先读 [lark-base-shortcut-field-properties.md](references/lark-base-shortcut-field-properties.md) 确认 `+field-create/+field-update` 的 JSON 结构
4. **筛选查询按场景执行** — 先读 [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md)、[lark-base-record-list.md](references/lark-base-record-list.md)、[lark-base-record-search.md](references/lark-base-record-search.md)；默认优先 `+record-list`，仅当用户提供明确搜索关键词时使用 `+record-search`；视图筛选读取用 `+view-set-filter` + `+record-list`
5. **对记录进行分析（涉及"最高/最低/总计/平均/排名/比较/数量"等分析意图）** — 先读 [lark-base-data-query.md](references/lark-base-data-query.md)，通过 `+data-query` 进行数据筛选聚合的服务端计算
6. **聚合分析与取数互斥** — 需要分组统计 / SUM / MAX / AVG / COUNT 时，必须使用 `+data-query`（服务端计算），禁止用 `+record-list / +record-search` 拉全量记录再手动计算；反之，`+data-query` 不返回原始记录，取数场景走 `+record-search / +record-list / +record-get`
7. **所有 `+xxx-list` 禁止并发调用** — `+table-list / +field-list / +record-list / +view-list / +record-history-list / +role-list` 只能串行执行
8. **批量上限 500 条/次** — 同一表建议串行写入，并在批次间延迟 0.5–1 秒
9. **统一参数名** — 一律使用 `--base-token`，不使用旧 `--app-token`
10. **遇到“公式 / 查找引用 / 派生指标 / 跨表计算”需求，优先走字段方案判断** — 先判断应建 formula / lookup 字段，还是只做一次性 `+data-query`
11. **公式、lookup、系统字段默认视为只读** — 除 `+field-create / +field-update` 维护字段定义外，不要把这些字段作为记录写入目标
12. **改名和删除按明确意图执行** — `+view-rename` 在目标视图和新名称都明确时可直接执行；`+record-delete / +field-delete / +table-delete` 在用户已经明确要求删除且目标明确时也可直接执行，不需要再补一次确认，并且执行删除命令时要主动补上 `--yes`；只有目标不明确时才继续追问

## 问卷 / 表单提示

- **获取问卷列表**：使用 `+form-list`（先拿 `form-id`）
- **获取单个问卷**：使用 `+form-get`
- **获取表单 / 问卷问题**：使用 `+form-questions-list`
- **删除问卷 / 表单问题**：使用 `+form-questions-delete`
- **创建 / 更新问题**：使用 `+form-questions-create / +form-questions-update`

## 意图 → 命令索引

| 意图 | 推荐命令 | 备注 |
|------|---------|------|
| 列表 / 获取数据表 | `lark-cli base +table-list` / `+table-get` | 原子命令 |
| 创建 / 更新 / 删除数据表 | `lark-cli base +table-create` / `+table-update` / `+table-delete` | 一命令一动作 |
| 列表 / 获取字段 | `lark-cli base +field-list` / `+field-get` | 原子命令 |
| 创建 / 更新字段 | `lark-cli base +field-create` / `+field-update` | 使用 `--json` |
| 创建 / 更新公式字段 | `lark-cli base +field-create` / `+field-update` | `type=formula`；先读 formula guide，再创建 / 更新 |
| 创建 / 更新 lookup 字段 | `lark-cli base +field-create` / `+field-update` | `type=lookup`；先读 lookup guide，再创建 / 更新，默认先判断 formula 是否更合适 |
| 关键词搜索记录 | `lark-cli base +record-search` | 透传搜索参数；用于关键词检索，不用于聚合分析 |
| 列表 / 获取记录 | `lark-cli base +record-list` / `+record-get` | 原子命令，如果需要`聚合计算`，`分组统计` 推荐走 `+data-query` |
| 创建 / 更新记录 | `lark-cli base +record-upsert` | `--table-id [--record-id] --json` |
| 批量创建记录 | `lark-cli base +record-batch-create` | 适合大量创建写入（如 CSV/Excel 导入）；先按 record-value 规范整理为 `json.fields + json.rows` |
| 批量更新指定记录 | `lark-cli base +record-batch-update` | `--table-id --json`（`json.record_id_list + json.patch`） |
| 聚合分析 / 比较排序 / 求最值 / 筛选统计 | `lark-cli base +data-query` | 不要用 `+record-list / +record-search` 拉全量数据再手动计算，需使用 `+data-query` 走服务端计算 |
| 配置 / 查询视图 | `lark-cli base +view-*` | `list/get/create/delete/get-*/set-*/rename` |
| 查看记录历史 | `lark-cli base +record-history-list` | 按表和记录查询变更历史 |
| 按视图筛选查询 | `lark-cli base +view-set-filter` + `lark-cli base +record-list` | 组合调用 |
| 把本地文件导入为 Base / 多维表格 | `lark-cli drive +import --type bitable` | 导入阶段属于 `drive`，不是 `base` |
| 创建 / 获取 / 复制 Base | `lark-cli base +base-create` / `+base-get` / `+base-copy` | 原子命令 |
| 列表 / 获取工作流 | `lark-cli base +workflow-list` / `+workflow-get` | 原子命令 |
| 创建 / 更新工作流 | `lark-cli base +workflow-create` / `+workflow-update` | 使用 `--json`，必须阅读 schema |
| 启用 / 停用工作流 | `lark-cli base +workflow-enable` / `+workflow-disable` | 一命令一动作 |
| 启用 / 停用高级权限 | `lark-cli base +advperm-enable` / `+advperm-disable` | 启用后才能使用自定义角色；停用会使已有角色失效 |
| 列表 / 获取角色 | `lark-cli base +role-list / +role-get` | 查看角色摘要或完整配置 |
| 创建 / 更新 / 删除角色 | `lark-cli base +role-create / +role-update / +role-delete` | 管理自定义角色权限 |
| 列表 / 获取表单 | `lark-cli base +form-list` / `+form-get` | 原子命令 |
| 创建 / 更新 / 删除表单 | `lark-cli base +form-create` / `+form-update` / `+form-delete` | 一命令一动作 |
| 列表 / 创建 / 更新 / 删除表单问题 | `lark-cli base +form-questions-list` / `+form-questions-create` / `+form-questions-update` / `+form-questions-delete` | 一命令一动作 |
| 创建/管理仪表盘及图表 | `+dashboard-* / +dashboard-block-*` | **必须先读** [lark-base-dashboard.md](references/lark-base-dashboard.md) |


## 操作注意事项

- **Base token 口径统一**：统一使用 `--base-token`
- **`+xxx-list` 调用纪律**：`+table-list / +field-list / +record-list / +view-list / +record-history-list / +role-list / +dashboard-list / +dashboard-block-list / +workflow-list` 禁止并发调用；批量执行时只能串行
- **`+record-list` 分页与 limit**：`--limit` 最大 `200`。先拉首批并检查 `has_more`；仅当 `has_more=true` 且用户明确需要更多数据（如“全部导出/全量明细/继续下一页”）时再按 `offset` 递增翻页，禁止单次传超过 `200`
- **记录读取字段筛选**：`+record-list` 支持重复传参 `--field-id` 做字段筛选
- **`+record-list / +record-search` 选择规则**：优先使用 `+record-list`；仅当用户给出明确搜索关键词时，才使用 `+record-search`
- **`+record-search` 使用规则**：仅通过 `--json` 传搜索请求体；`keyword/search_fields/offset/limit` 等字段合法性由 API 侧按 schema 校验
- **`+record-batch-create` 适用边界**：优先用于大量创建写入（如 CSV/Excel 导入）。当输入是长表格或长文本时，先按 [lark-base-shortcut-record-value.md](references/lark-base-shortcut-record-value.md) 做字段映射和类型规范化，再组装 `fields + rows` 调用命令写入
- **字段可写性先判断**：存储字段才可写；公式 / lookup / 系统字段默认只读，写记录时应跳过
- **公式能力要主动想到**：用户说“算一下”“生成标签”“判断是否异常”“跨表汇总”“按日期差预警”时，要先判断是否应该建公式字段，而不是只返回手工分析方案
- **lookup 不是默认首选**：lookup 只在用户明确要求或确实更适合固定查找模型时使用；常规计算、跨表聚合和条件判断优先 formula
- **附件字段**：如果用户要“上传附件 / 给记录加文件”，只能走 `+record-upload-attachment` 这条链路（读字段 → 读记录 → 上传素材 → 回写记录）
- **人员字段 / 用户字段**：调试时注意 `user_id_type` 与执行身份（user / bot）差异
- **history 使用方式**：`+record-history-list` 按 `table-id + record-id` 查询记录历史，不支持整表历史扫描
- **workspace 状态**：已接入 `+base-create / +base-get / +base-copy`
- **`+base-create / +base-copy` 结果返回规范**：创建或复制成功后，回复中必须主动返回新 Base 的标识信息。若返回结果里带可访问链接（如 `base.url`），要一并返回
- **`+base-create / +base-copy` 友好性规则**：`--folder-token`、`--time-zone`、复制时的 `--name` 都是可选项。用户没有特别要求时，不要为了这些可选参数额外打断；能直接创建/复制就直接执行
- **`+base-create / +base-copy` 权限处理（bot 创建）**：若 Base 由应用身份（bot）创建，创建或复制成功后默认继续使用 bot 身份为当前可用 user（指当前 CLI 中 auth 模块已登录且可用的用户身份）添加 `full_access`（管理员）权限，并在回复中明确授权结果（成功 / 无可用 user / 授权失败及原因）。若授权未完成，要继续给出后续引导（稍后重试授权或继续用 bot）；owner 转移必须单独确认，禁止擅自执行
- **advperm 使用方式**：`+advperm-enable` 启用高级权限后才能管理角色（`+role-*`）；`+advperm-disable` 是高风险操作，停用后已有自定义角色全部失效；操作用户必须为 Base 管理员；先读 [lark-base-advperm-enable.md](references/lark-base-advperm-enable.md) / [lark-base-advperm-disable.md](references/lark-base-advperm-disable.md)
- **role 使用方式**：`+role-create` 仅支持 `custom_role`；`+role-update` 采用 Delta Merge（`role_name` 和 `role_type` 必须始终提供）；`+role-delete` 不可逆且仅支持自定义角色；角色配置支持 `base_rule_map`（Base 级复制/下载）、`table_rule_map`（表级权限含记录/字段粒度）、`dashboard_rule_map`（仪表盘权限）、`docx_rule_map`（文档权限）；写角色前先读 [role-config.md](references/role-config.md)
- **表单 form-id**：通过 `+form-list` 获取；`+form-create` 返回的 `id` 即 `form-id`，可用于 `+form-questions-*` 操作
- **workflow 使用方式**：在创建或更新 workflow 前，必须仔细阅读 [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md) 了解各触发器和节点组件的结构；同时 `+workflow-list` 返回的不是完整树状结构，若需读取完整结构请使用 `+workflow-get`。
- **data-query 使用方式**：使用 `+data-query` 前必须先阅读 [lark-base-data-query.md](references/lark-base-data-query.md) 了解 DSL 结构、支持的字段类型、聚合函数和限制条件；DSL 中的 `field_name` 必须与表字段名精确匹配，构造前先用 `+field-list` 获取真实字段名
- **公式 / lookup 使用方式**：构造表达式或 where 条件前，至少先拿当前表结构；跨表时要查找目标表的结构，不允许凭自然语言猜字段名
- **视图重命名确认规则**：用户已经明确“把哪个视图改成什么名字”时，`+view-rename` 直接执行即可，不需要再补一句确认
- **删除确认规则（记录 / 字段 / 表）**：如果用户已经明确说要删除，并且目标也明确，`+record-delete / +field-delete / +table-delete` 可直接执行，不需要再补一次确认；执行时直接带 `--yes` 通过 CLI 的高风险写入校验。只有目标仍有歧义时，再先用 `+record-get / +field-get / +table-get` 或 list 命令确认

## Wiki 链接特殊处理（特别关键！）

知识库链接（`/wiki/TOKEN`）背后可能是云文档、电子表格、多维表格等不同类型的文档。**不能直接假设 URL 中的 token 就是 file_token**，必须先查询实际类型和真实 token。

### 处理流程

1. **使用 `wiki.spaces.get_node` 查询节点信息**
   ```bash
   lark-cli wiki spaces get_node --params '{"token":"&lt;wiki_token&gt;"}'
   ```

2. **从返回结果中提取关键信息**
   - `node.obj_type`：文档类型（docx/doc/sheet/bitable/slides/file/mindnote）
   - `node.obj_token`：**真实的文档 token**（用于后续操作）
   - `node.title`：文档标题

3. **根据 `obj_type` 选择后续命令**

   | obj_type | 说明 | 后续命令 |
         |----------|------|-----------|
   | `docx` | 新版云文档 | `drive file.comments.*`、`docx.*` |
   | `doc` | 旧版云文档 | `drive file.comments.*` |
   | `sheet` | 电子表格 | `sheets.*` |
   | `bitable` | 多维表格 | `lark-cli base +...`（优先）；如果 shortcut 不覆盖，再用 `lark-cli base <resource> <method>`；**不要**改走 `lark-cli api /open-apis/bitable/v1/...` |
   | `slides` | 幻灯片 | `drive.*` |
   | `file` | 文件 | `drive.*` |
   | `mindnote` | 思维导图 | `drive.*` |

4. **把 wiki 解析出的 `obj_token` 当成 Base token 使用**
   - 当 `obj_type=bitable` 时，`node.obj_token` 就是后续 `base` 命令应使用的真实 token。
   - 也就是说：如果原始输入是 `/wiki/...` 链接，不要把 `wiki_token` 直接塞给 `--base-token`。

5. **如果已经报了 token 错，再回退检查 wiki**
   - 如果命令返回 `param baseToken is invalid`、`base_token invalid`、`not found`，并且用户最初给的是 `/wiki/...` 链接或 `wiki_token`，优先怀疑“把 wiki token 当成了 base token”。
   - 这时不要改走 `bitable/v1` API；应立即重新执行 `lark-cli wiki spaces get_node`，确认 `obj_type=bitable` 后，改用 `node.obj_token` 重新执行 `lark-cli base +...`。

### 查询示例

```bash
# 查询 wiki 节点
lark-cli wiki spaces get_node --params '{"token":"Pgrr***************UnRb"}'
```

返回结果示例：
```json
{
  "node": {
    "obj_type": "docx",
    "obj_token": "UAJ***************E9nic",
    "title": "ai friendly 测试 - 1 副本",
    "node_type": "origin",
    "space_id": "6946843325487906839"
  }
}
```

## Base 链接解析规则
| 链接类型 | 格式 | 处理方式 |
|---------|------|---------|
| 直接 Base 链接 | `/base/{token}` | 直接提取作为 `--base-token` |
| Wiki 知识库链接 | `/wiki/{token}` | 先调用 `wiki.spaces.get_node`，取 `node.obj_token` |
### URL 参数提取
```
https://{domain}/base/{base-token}?table={table-id}&view={view-id}
```
- `/base/{token}` → `--base-token`
- `?table={id}` → `--table-id`
- `?view={id}` → `--view-id`
### 禁止事项
- **禁止**将完整 URL 直接作为 `--base-token` 参数传入
- **禁止**将 wiki_token 直接作为 `--base-token`

## 常见错误速查

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 1254064 | 日期格式错误 | 用毫秒时间戳，非字符串 / 秒级 |
| 1254068 | 超链接格式错误 | 用 `{text, link}` 对象 |
| 1254066 | 人员字段错误 | 用 `[{id:"ou_xxx"}]`，并确认 `user_id_type` |
| 1254045 | 字段名不存在 | 检查字段名（含空格、大小写） |
| 1254015 | 字段值类型不匹配 | 先 `+field-list`，再按类型构造 |
| `param baseToken is invalid` / `base_token invalid` | 把 wiki token、workspace token 或其他 token 当成了 `base_token` | 如果输入来自 `/wiki/...`，先用 `lark-cli wiki spaces get_node` 取真实 `obj_token`；当 `obj_type=bitable` 时，用 `node.obj_token` 作为 `--base-token` 重试，不要改走 `bitable/v1` |
| formula / lookup 创建失败 | 指南未读或结构不合法 | 先读 `formula-field-guide.md` / `lookup-field-guide.md`，再按 guide 重建请求 |
| 系统字段 / 公式字段写入失败 | 只读字段被当成可写字段 | 改为写存储字段，计算结果交给 formula / lookup / 系统字段自动产出 |
| 1254104 | 批量超 500 条 | 分批调用 |
| 1254291 | 并发写冲突 | 串行写入 + 批次间延迟 |

## 参考文档

- [lark-base-shortcut-field-properties.md](references/lark-base-shortcut-field-properties.md) — `+field-create/+field-update` 调用前必看，各类型 field JSON 规范
- [role-config.md](references/role-config.md) — 角色权限配置详解
- [lark-base-shortcut-record-value.md](references/lark-base-shortcut-record-value.md) — record 写入（`+record-upsert / +record-batch-create / +record-batch-update`）调用前必看，各类型 record JSON 规范
- [lark-base-record-batch-create.md](references/lark-base-record-batch-create.md) — `+record-batch-create` 用法与 `--json` 结构
- [lark-base-record-batch-update.md](references/lark-base-record-batch-update.md) — `+record-batch-update` 用法与 `--json` 结构
- [formula-field-guide.md](references/formula-field-guide.md) — formula 字段写法、函数约束、CurrentValue 规则、跨表计算模式（强烈推荐）
- [lookup-field-guide.md](references/lookup-field-guide.md) — lookup 字段配置规则、where/aggregate 约束、与 formula 的取舍
- [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md) — 视图筛选配置
- [lark-base-record-list.md](references/lark-base-record-list.md) — 记录列表读取与分页
- [lark-base-record-search.md](references/lark-base-record-search.md) — 关键词搜索记录
- [lark-base-advperm-enable.md](references/lark-base-advperm-enable.md) — `+advperm-enable` 启用高级权限
- [lark-base-advperm-disable.md](references/lark-base-advperm-disable.md) — `+advperm-disable` 停用高级权限
- [lark-base-role-list.md](references/lark-base-role-list.md) — `+role-list` 列出角色
- [lark-base-role-get.md](references/lark-base-role-get.md) — `+role-get` 获取角色详情
- [lark-base-role-create.md](references/lark-base-role-create.md) — `+role-create` 创建角色
- [lark-base-role-update.md](references/lark-base-role-update.md) — `+role-update` 更新角色
- [lark-base-role-delete.md](references/lark-base-role-delete.md) — `+role-delete` 删除角色
- [lark-base-dashboard.md](references/lark-base-dashboard.md) — dashboard 模块工作流指引
- [dashboard-block-data-config.md](references/dashboard-block-data-config.md) — Block data_config 结构、图表类型、filter 规则
- [lark-base-workflow.md](references/lark-base-workflow.md) — workflow 命令索引
- [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md) — `+workflow-create/+workflow-update` JSON body 数据结构详解，包含触发器及各类节点的配置规则（强烈推荐）
- [lark-base-data-query.md](references/lark-base-data-query.md) — `+data-query` 聚合分析（DSL 结构、支持字段类型、聚合函数）
- [examples.md](references/examples.md) — 完整操作示例（建表、筛选、更新）

## 命令分组

> **执行前必做：** 从下表定位到命令后，务必先阅读对应命令的 reference 文档，再调用命令。

| 命令分组 | 说明 |
|----------|------|
| [`table commands`](references/lark-base-table.md) | `+table-list / +table-get / +table-create / +table-update / +table-delete` |
| [`field commands`](references/lark-base-field.md) | `+field-list / +field-get / +field-create / +field-update / +field-delete / +field-search-options` |
| [`record commands`](references/lark-base-record.md) | `+record-search / +record-list / +record-get / +record-upsert / +record-batch-create / +record-batch-update / +record-upload-attachment / +record-delete` |
| [`view commands`](references/lark-base-view.md) | `+view-list / +view-get / +view-create / +view-delete / +view-get-* / +view-set-* / +view-rename` |
| [`data-query commands`](references/lark-base-data-query.md) | `+data-query` |
| [`history commands`](references/lark-base-history.md) | `+record-history-list` |
| [`base / workspace commands`](references/lark-base-workspace.md) | `+base-create / +base-get / +base-copy` |
| [`advperm commands`](references/lark-base-advperm-enable.md) | `+advperm-enable / +advperm-disable` |
| [`role commands`](references/lark-base-role-list.md) | `+role-list / +role-get / +role-create / +role-update / +role-delete` |
| [`form commands`](references/lark-base-form-create.md) | `+form-list / +form-get / +form-create / +form-update / +form-delete` |
| [`form questions commands`](references/lark-base-form-questions-create.md) | `+form-questions-list / +form-questions-create / +form-questions-update / +form-questions-delete` |
| [`workflow commands`](references/lark-base-workflow.md) | `+workflow-list / +workflow-get / +workflow-create / +workflow-update / +workflow-enable / +workflow-disable` |
| [`dashboard / dashboard-block commands`](references/lark-base-dashboard.md) | `+dashboard-list / +dashboard-get / +dashboard-create / +dashboard-update / +dashboard-delete / +dashboard-block-list / +dashboard-block-get / +dashboard-block-create / +dashboard-block-update / +dashboard-block-delete` |
