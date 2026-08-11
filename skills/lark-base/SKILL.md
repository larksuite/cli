---
name: lark-base
version: 1.2.5
description: "飞书多维表格（Base）操作：建表、字段、记录、视图、统计、公式/lookup、表单、仪表盘、workflow、角色权限；遇到 Base/多维表格/bitable 或 /base/ 链接时使用。文件导入/导出转 lark-drive，认证/授权转 lark-shared。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli base --help"
---

# base

## 何时使用

使用本 skill：

- 用户明确提到 Base / 多维表格 / bitable，或给出 `/base/` 链接。
- 用户要在 Base 内建表、改表、管理字段、写记录、查记录、配视图。
- 用户要在 Base 内做公式字段、lookup 字段、跨表计算、派生指标、筛选聚合、TopN、统计分析。
- 用户要管理 Base 表单、仪表盘、workflow、高级权限或角色。
- 用户要把旧 Base 聚合式命令或旧写法迁移到当前 `lark-cli base +...` shortcut。

不要使用本 skill：

- 只是认证、初始化配置、切换身份、处理 scope 或权限授权恢复，转 `lark-shared`。
- 把本地文件导入成 Base，或将 Base 导出为本地文件，转 `lark-drive`。
- 泛化数据分析、字段设计、公式讨论，但没有 Base/多维表格上下文。

## 使用边界

- Base 业务操作只使用 `lark-cli base +...` shortcut，不使用旧聚合式 `+table / +field / +record / +view / +history / +workspace`。
- 执行 update 前必须先查当前 shortcut 的 `--help` 或对应 reference。若命令要求完整配置，首次请求必须基于可信的当前配置执行 read-modify-write：只修改用户明确指定的内容，保留其他仍适用的可写配置，并按命令要求的结构提交。若命令支持局部／delta update，按其契约提交最小合法 payload；不得以不完整请求试错补参。
- Base CLI/OpenAPI 当前不支持视图行高、冻结列、列宽等 UI-only 外观设置。遇到这类需求，说明能力边界并停止，不要猜测未文档化参数或改走 raw API。
- **高频：数据分析。** 数据表记录用于查询、分析、解析或比较时，先读取 [Base 数据表查询与分析 SOP](references/lark-base-data-analysis-sop.md)；进入本地分析路径后，使用 `+record-list --format ndjson` 获取分析数据。
- **低频：在线复制。** 复制整个 Base 使用 `+base-copy`，复制 Base 内单张数据表使用 `+table-copy`。
- **更低频：文件导入/导出。** 本地文件与 Base 之间的导入/导出转 `lark-drive`；具体格式、参数、路径限制和仅结构导出规则由 `lark-drive` 负责，导入完成后再回到 Base 命令。
- 认证、初始化、scope、身份切换、权限不足恢复属于 `lark-shared`；Base 文档只保留会影响 Base 路径选择的权限规则。

## 先获取 Base Token 和所需 ID

进入任何需要目标 Base 的 shortcut 前，必须先拿到可用的 `base_token`，以及当前任务需要的 `table_id` / `view_id` / `record_id` / `form_id` / `dashboard_id` / `workflow_id` 等真实 ID；不要把完整 URL、wiki token、workspace token 或孤立 raw token 直接当作 `--base-token`。

- 用户输入 URL 或分享链接：先运行 `lark-cli base +url-resolve --url "<url>" --as user`，用返回的 `base_token` 和相关 ID 继续后续命令。
- Base/Wiki URL 的 `table=` query 参数实际表示当前选中的顶层 block，可能是数据表、仪表盘或 workflow；不要按参数名自行当成 `table_id`。以 `+url-resolve` 返回的 `block_type` 以及 `table_id` / `dashboard_id` / `workflow_id` 为准；`selection_source=url_query` 只说明 URL 当前选中了该 block，不代表它覆盖用户明确点名的目标。若用户点名的 dashboard 与 `block_name` 不一致，先用 `+dashboard-list` 按名称匹配；若只返回中性 `block_id`，按 hint 用 `+base-block-list` 确认类型。
- 用户输入 Base 标题、关键词或不确定名称：先运行 `lark-cli base +title-resolve --title "<keyword>" --as user`；`--title` 传入标题中的短关键词，不超过 30 个字符；过长标题先取最有区分度的短关键词；多候选时先让用户消歧，不要猜。
- 文档嵌入 Base 标签：直接读取 `<bitable>` / `<base_refer>` 的 `token` 作为 `--base-token`，`table-id` 作为 `--table-id`，`view-id` 作为 `--view-id`；孤立 raw token 不走 `+url-resolve`。
- 仍无法定位且用户不是要新建 Base 时，先反问用户要操作哪一个 Base；用户要新建时才用 `+base-create`。

## 快速路由

| 用户目标 | 优先命令 | 何时读 reference |
|---|---|---|
| 查 Base 本体 | `+base-get` | 用返回确认 Base 名称、owner、权限和可继续操作的 token |
| 创建/复制 Base | `+base-create` / `+base-copy` | 新建时强烈推荐用 `--table-name` + `--fields` 同时配置新 Base 里唯一一个初始数据表的 name 和 schema；写入后报告新 Base 标识和 `permission_grant` |
| Base 文件导入/导出 | 转 `lark-drive` | 文件格式、参数、路径限制和仅结构导出规则由 `lark-drive` 负责；在线复制走 `+base-copy` |
| 查看 Base 内资源目录 | `+base-block-list` | 想先了解一个 Base 里有哪些 table/docx/dashboard/workflow/folder 时优先用它；返回 ID 关系和 fewshot 看 `--help` |
| 管理 Base 内资源目录 | `+base-block-create/move/rename/delete` | 创建或整理 Base 直接管理的 folder/table/docx/dashboard/workflow；资源内容继续用对应命令 |
| 管理数据表 | `+table-list/get/create/update/delete` | 处理 table 的列出、详情、创建、重命名和删除；`+table-create` 必须传 `--fields` 一次性定义表结构，字段 JSON 读 [lark-base-field-json.md](references/lark-base-field-json.md) |
| 复制 Base 内单张数据表 | `+table-copy` / `+table-copy-status` | 在线复制单张数据表；复制范围和异步任务参数查看 `--help` |
| 列/查/删字段 | `+field-list/get/delete/search-options` | 写入前用 list/get 确认字段类型、选项、ID；删除前确认目标字段 |
| 创建/更新字段 | `+field-create` / `+field-update` | 同一表创建多个字段时，默认一次向 `+field-create --json` 传字段对象数组；预计串行运行时间超过 caller/tool timeout 时按时间预算拆分，不按固定条数切块；仅创建一个或多个只含 `name` + `type:text` 的简单字段时按 `+field-create --help` 即可，其他类型或属性必读 [lark-base-field-json.md](references/lark-base-field-json.md)；公式读 [formula-field-guide.md](references/formula-field-guide.md)，lookup 读 [lookup-field-guide.md](references/lookup-field-guide.md)；仍需逐项恢复或命令细节时读 [lark-base-field-create.md](references/lark-base-field-create.md)，更新细节读 [lark-base-field-update.md](references/lark-base-field-update.md) |
| 读取已知记录 | `+record-get` | 已知具体 `record_id` 时可以直接读取记录 |
| 查询或分析数据表记录 | 由 [Base 数据表查询与分析 SOP](references/lark-base-data-analysis-sop.md) 选择 | 数据表记录查询和分析任务先读 SOP |
| 解释、编写或排错 `+data-query` DSL | [data-query guide](references/lark-base-data-query-guide.md) | 用户明确询问 `+data-query` 命令或 DSL 时直接读取；需要完整字段、操作符、限制或响应协议时再读 [DSL SSOT](references/lark-base-data-query.md) |
| 写记录 | `+record-upsert` / `+record-batch-create` / `+record-batch-update` | 必读 [lark-base-record-upsert.md](references/lark-base-record-upsert.md) / [lark-base-record-batch-create.md](references/lark-base-record-batch-create.md) / [lark-base-record-batch-update.md](references/lark-base-record-batch-update.md) 和 [lark-base-cell-value.md](references/lark-base-cell-value.md) |
| 附件字段 | `+record-upload-attachment` / `+record-download-attachment` / `+record-remove-attachment` | 使用附件操作命令上传本地文件系统中的文件，下载/删除按 file token 或字段定位 |
| 删除记录 / 分享记录链接 / 历史 | `+record-delete` / `+record-share-link-create` / `+record-history-list` | 删除前确认 record；分享链接最多 100 条；历史读 [lark-base-record-history-list.md](references/lark-base-record-history-list.md)，只查单条记录，不做整表审计 |
| 管理视图 | `+view-*` | `+view-set-filter` 读 [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md)（filter 条件结构见公共协议 [lark-base-filter-condition.md](references/lark-base-filter-condition.md)）；其余配置先 get 现状，再按返回结构更新 |
| 公式字段 | `+field-create/update --json '{"type":"formula",...}'` | 必读 [formula-field-guide.md](references/formula-field-guide.md)，读后再加隐藏确认 flag `--i-have-read-guide` |
| Lookup 字段 | `+field-create/update --json '{"type":"lookup",...}'` | 必读 [lookup-field-guide.md](references/lookup-field-guide.md)，读后再加隐藏确认 flag `--i-have-read-guide` |
| 表单提交 | `+form-submit` | 先读 [lark-base-form-detail.md](references/lark-base-form-detail.md) 获取题目、filter 和附件所需 `base_token`；提交 JSON 读 [lark-base-form-submit.md](references/lark-base-form-submit.md) |
| 表单题目创建/更新 | `+form-questions-create` / `+form-questions-update` | Base 内表单按 table 管理；先确定并复用真实 `table_id`。读 [lark-base-form-questions-create.md](references/lark-base-form-questions-create.md) / [lark-base-form-questions-update.md](references/lark-base-form-questions-update.md)；题目显隐条件 `visible_rule` 结构见公共协议 [lark-base-filter-condition.md](references/lark-base-filter-condition.md) |
| Base 内表单管理 | `+form-list/get/create/update/delete` / `+form-questions-list/delete` | 缺少或不确定归属时，先用 `+table-list` 或 `+base-block-list` 取得真实 `table_id`；这些命令使用 `--base-token + --table-id` 并在整个工作流中复用同一 `table_id`，删除前确认目标表单 |
| 分享表单详情 | `+form-detail --share-token <share_token>` | 使用表单分享链接里的 `share_token`；提交前读 [lark-base-form-detail.md](references/lark-base-form-detail.md) |
| 仪表盘与组件 | `+dashboard-*` / `+dashboard-block-*` | 提到图表/看板/block 时先读 [lark-base-dashboard.md](references/lark-base-dashboard.md)；组件 `data_config` 读 [dashboard-block-data-config.md](references/dashboard-block-data-config.md)；读取一个或多个图表计算结果用 `+dashboard-block-get-data`；读取完整仪表盘时按 block 类型分流，文本和不支持直接取数的图表按 reference 恢复 |
| Workflow | `+workflow-*` | 创建/更新或理解 steps 时读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) 和 steps JSON SSOT [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)；list/get/enable/disable 只处理 workflow ID 与启停状态 |
| 高级权限与角色 | `+advperm-*` / `+role-*` | 角色操作先读入口 [lark-base-role-guide.md](references/lark-base-role-guide.md)；角色 create/update 或解读完整配置再读权限 JSON SSOT [role-config.md](references/role-config.md)；关闭高级权限会影响自定义角色 |

## Base 心智模型

- Base 曾用名 Bitable；返回字段、错误或旧文档里的 `bitable` 多为历史兼容，不代表应改走裸 API 或另一套命令。
- `+base-block-list` 是查看一个 Base 内资源目录的新入口：它列出这个 Base 直接管理的 `folder/table/docx/dashboard/workflow`，适合先判断 Base 里有什么，再决定走 table、dashboard、workflow 或 docx 命令。
- `base-block` 只负责资源目录管理，包括创建资源、移动到 folder、重命名和删除；具体资源内容仍走 table/dashboard/workflow 命令。
- 新建 Base 时，强烈推荐一次性执行 `lark-cli base +base-create --name "<base>" --table-name "<table>" --fields '<field-json-array>'`，同时配置新 Base 里唯一一个初始数据表的 name 和 schema；使用 `--fields` 前先读 [lark-base-field-json.md](references/lark-base-field-json.md) 或复用 `+field-create` 的字段 JSON 形状，不要猜字段属性。
- `+base-create` 不传 `--table-name` 和 `--fields` 时，会创建一个默认 schema 的初始数据表。
- `+table-copy` 用于在线复制 Base 内的数据表，`--table-id` 可使用当前 Base 中的表 ID 或表名；复制范围等参数查看 `--help`。
- 表、字段、视图、workflow、dashboard block 的名称和 ID 必须来自真实返回，不要凭用户口述猜。
- `formula` 适合常规计算、条件判断、文本/日期处理和长期派生指标；`lookup` 适合明确的跨表查找、筛选后取值或聚合引用。
- 写入、公式、lookup、workflow、dashboard 前，先读取真实结构：表、字段、视图、关联表和 dashboard block 名称都以命令返回为准。

## 身份与权限降级

- 默认显式使用 `--as user` 操作用户资源；只有用户明确要求应用身份时，才直接用 `--as bot`。
- `+table-copy --wait` 提交成功后会在 stderr 打印完整 `task_id`；若进程被 Ctrl-C 终止，可用该 ID 和原身份执行 `+table-copy-status` 续查，不要重新提交复制。
- user 身份报 scope/授权不足，或错误中包含 `missing_scopes` / `hint`，先转 `lark-shared` 做用户授权恢复，不要直接降级 bot。
- user 身份报资源级无访问且无授权恢复提示时，才可用 `--as bot` 重试一次；bot 仍失败就停止重试并按权限错误处理。
- `91403` 或明确不可访问错误不要循环换身份重试。
- `+base-create` / `+base-copy` 若用 bot 身份执行，关注返回中的 `permission_grant`，并把用户是否可打开新 Base 告知用户。

## 写入前置规则

- 优先用写入返回确认结果；返回信息不足或任务明确要求核验时，再读回。
- 严格区分动作语义：用户要求“新增/创建”时，必须用本轮 create 返回的对象、ID 或数量确认完成，不能把已有资源算作本轮新增；目标已存在时按具体命令或 guide 的同名契约处理，不得自行改写用户语义。复合创建任务对每类资源只做一次必要盘点；只有命令明确返回逐项结果时才优先使用批量创建，并继续配置本轮返回的 ID。
- 写记录前先读字段结构；只写存储字段。系统字段、附件字段、`formula`、`lookup` 不作为普通记录写入目标。
- 附件上传、下载、删除走专用 `+record-*-attachment` 命令。
- 除上述简单 text fast path 外，写字段前先读 [lark-base-field-json.md](references/lark-base-field-json.md)；请求字段类型不在 reference 已支持类型目录中时，说明当前 CLI 不支持并停止，不要猜测未注册的字段 JSON、service 或 schema，也不要用其他字段类型冒充；涉及 `formula` / `lookup` 时必须读 [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)。
- 表名、字段名、视图名、workflow 配置中的名称必须来自真实返回；跨表场景还要读取目标表结构。
- 删除、角色更新、字段更新、表单提交（`+form-submit`）等高风险操作遵循 CLI 的 confirmation gate，必须带 `--yes`；目标不明确时先用 get/list 消歧。
- 真正的 batch 写命令遵守各自文档的单批上限；`+field-create` 数组是顺序单项请求，按 caller timeout 而非固定条数拆分；连续写同一表时串行执行，遇到 `1254291` 按短暂等待后重试处理。
- `select` 字段只支持写入字段中已有的选项；构造 CellValue 前先用 `+field-list` 或 `+field-search-options` 确认目标选项存在。

## 表单与视图细节

- Base 内表单 list/get/create/update/delete 和题目管理都属于具体数据表：第一个管理命令前必须已有归属明确的真实 `table_id`；缺失或归属不明确时才用 `+table-list` 或 `+base-block-list` 定位，已有真实 ID 时直接复用。后续管理命令始终传同一 `base_token + table_id`。
- 表单问题由数据表字段承载，question `id` 就是 `field_id`。创建问题前先 `+form-questions-list`；除非用户明确要求同名的独立问题，否则标题已存在时优先用 `+form-questions-update` 修改必填状态、标题或描述，不要先创建同名问题再删除旧问题。
- `+form-questions-delete` 用于删除非主字段问题；主字段问题使用 `+form-questions-update` 修改。
- `+form-submit` 是高风险写操作，必须带 `--yes` 确认；调用前必须先跑 `+form-detail`，读取 `questions[].type`、`required`、`filter` 和附件场景需要的 `base_token`；不要填写被 filter 隐藏的问题。
- `+form-questions-update` 是题目配置全量覆盖，不是 patch；未传字段会回落默认值，传空字符串 / `null` / 空数组会直接写入空或清空。更新前先 `+form-questions-list` 读取当前题目，把要保留的 `title` / `description` / `required` / `option_display_mode` / `visible_rule` 等字段带回请求。
- 表单附件不要写进 `fields`，放在 `--json.attachments`；提交附件时必须同时传表单所属 Base 的 `--base-token`。
- `+view-set-filter` 是唯一保留的 view reference；sort/group/card/timebar/visible-fields 这类配置先用对应 get 命令读现状，保留未修改字段，只替换用户要求变更的配置。
- 视图适合持久化、共享和 UI 复用；一次性筛选/排序可先用 `+record-list` / `+record-search` 的 filter/sort 验证结果，再按需要沉淀为持久视图。

## Dashboard / Workflow / Role

- Dashboard 的复杂点是 block 的 `data_config`，不是 list/get/create/delete 命令参数。创建或更新 block 前先读 [dashboard-block-data-config.md](references/dashboard-block-data-config.md)，组件必须串行创建；`+dashboard-arrange` 是服务端智能布局，仅在用户明确要求重排/美化、或对本次会话从零新建的仪表盘做收尾整理时执行。`+dashboard-block-get-data` 读取图表最终计算结果，不返回 block 名称、类型、布局或 `data_config`；需要元数据先用 `+dashboard-block-get`。用户要求“全部/完整”仪表盘内容时不得跳过 text 或不支持直接取数的 block，按 [lark-base-dashboard.md](references/lark-base-dashboard.md) 的完整读取分支恢复。
- Dashboard shortcut 不支持指定组件的 `x/y/w/h`、精确位置或尺寸，不能把 `+dashboard-arrange` 静默当作等价实现。用户只要求一般性重排/美化时可执行一次智能重排；用户要求精确结果时先说明限制并询问是否接受自适应布局，接受后才执行。不要探测 raw `lark-cli api`、源码或未公开布局参数。
- 创建接口成功返回即表示写入成功；只有结果不确定时才额外执行一次 `+dashboard-get` 或 `+dashboard-block-list`。不要仅为确认创建而逐组件调用 `+dashboard-block-get-data`。
- 用户要读取多个组件的计算结果时，先完整列出组件（`+dashboard-block-list --page-size 100`；若 `has_more=true`，继续把返回的 `page_token` 传给 `--page-token`，直到 `has_more=false`），再按 [lark-base-dashboard-block-get-data.md](references/lark-base-dashboard-block-get-data.md) 在一个 shell 工具调用内串行读取；不要把每个 block 拆成独立模型轮次。
- Workflow 的复杂点是 `steps` 结构。创建、更新或解释完整 workflow 时读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) 和 steps JSON SSOT [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)；enable/disable/list 只需确认 workflow ID、当前启停状态和用户意图。
- Role 的复杂点是权限 JSON。角色操作先读入口 [lark-base-role-guide.md](references/lark-base-role-guide.md)；`+role-create` 只支持自定义角色；`+role-update` 是 delta merge；角色 create/update 或解读完整配置时读权限 JSON SSOT [role-config.md](references/role-config.md)。`+role-delete` 只适用于自定义角色，系统角色不可删除；删除角色和关闭高级权限前必须确认目标和影响。

## 常见恢复

| 错误 / 现象 | 恢复动作 |
|---|---|
| `param baseToken is invalid` / `base_token invalid` | 检查是否把 wiki token、workspace token 或完整 URL 当成了 `--base-token`；按入口规则重新获取真实 `base_token` |
| `not found` 且输入来自 Wiki 链接 | 优先检查是否把 wiki token 当成 base token，不要立刻改走裸 API |
| `1254045` 字段名不存在 | 重新 `+field-list`，使用真实字段名或字段 ID；注意空格、大小写和跨表字段 |
| `1254015` 字段值类型不匹配 | 先 `+field-list`，再按 [lark-base-cell-value.md](references/lark-base-cell-value.md) 构造 CellValue |
| `Invalid discriminator value`（字段写入缺 `type`） | 按完整提交规则读取当前字段，只改目标内容后提交；不要只补 `type` 重试 |
| filter 报 `value of type array` / `Only string values` | 用 record/view 的 tuple `--filter-json`（非 `+data-query` 对象型），value 按字段 type 选标量或数组；见 [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md) |
| 日期 / 人员 / 超链接字段报格式错误 | 日期用 `YYYY-MM-DD HH:mm`；人员用 `[{ "id": "ou_xxx" }]`；超链接用 URL 或 markdown link 字符串 |
| formula / lookup 创建失败 | 先读 [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)，再按 guide 重建请求 |
| `ignored_fields` / `READONLY` | 移除只读字段，只写存储字段 |
| `1254104` | 批量超过 200，分批调用 |
| `1254291` | 并发写冲突，串行写入并在批次间短暂等待 |

## 保留 Reference

- [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md)：所有数据表记录查询和分析的统一入口；依次选择 jq、Python 或 Cloud
- [Python 标准库](references/lark-base-data-analysis-python-stdlib.md) / [pandas](references/lark-base-data-analysis-pandas.md)：统一数据分析 SOP 选定 Python 实现后按需读取的同场景示例
- [lark-base-data-analysis-cloud.md](references/lark-base-data-analysis-cloud.md)：统一 SOP 判定 jq 与 Python 路径均不适用时的云端查询 SOP
- [lark-base-data-query-guide.md](references/lark-base-data-query-guide.md) / [lark-base-data-query.md](references/lark-base-data-query.md)：Cloud SOP 选定 `+data-query` 后或用户直接询问该命令/DSL 时读取 fewshot，完整 DSL 细节再读 SSOT；其 `filters` 使用独立对象 DSL
- [lark-base-cell-value.md](references/lark-base-cell-value.md)：记录 CellValue 构造
- [lark-base-field-json.md](references/lark-base-field-json.md)：字段 JSON 构造
- [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)：公式与 lookup 字段
- [lark-base-field-create.md](references/lark-base-field-create.md) / [lark-base-field-update.md](references/lark-base-field-update.md)：字段创建/更新命令级补充
- [lark-base-record-upsert.md](references/lark-base-record-upsert.md) / [lark-base-record-batch-create.md](references/lark-base-record-batch-create.md) / [lark-base-record-batch-update.md](references/lark-base-record-batch-update.md) / [lark-base-record-history-list.md](references/lark-base-record-history-list.md)：记录写入 JSON 与历史返回解释
- [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md)：视图筛选 JSON
- [lark-base-filter-condition.md](references/lark-base-filter-condition.md)：视图 filter、记录 `--filter-json`、表单 `visible_rule` 的 tuple 条件结构公共协议 SSOT
- [lark-base-form-detail.md](references/lark-base-form-detail.md) / [lark-base-form-submit.md](references/lark-base-form-submit.md) / [lark-base-form-questions-create.md](references/lark-base-form-questions-create.md) / [lark-base-form-questions-update.md](references/lark-base-form-questions-update.md)：表单详情、提交和复杂 JSON
- [lark-base-dashboard.md](references/lark-base-dashboard.md) / [dashboard-block-data-config.md](references/dashboard-block-data-config.md) / [lark-base-dashboard-block-get-data.md](references/lark-base-dashboard-block-get-data.md)：仪表盘、组件配置与图表结果协议
- [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) / [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)：workflow 入口与 steps JSON SSOT
- [lark-base-role-guide.md](references/lark-base-role-guide.md) / [role-config.md](references/role-config.md)：角色入口与权限 JSON SSOT
