---
name: lark-base
version: 1.2.2
description: "飞书多维表格（Base）操作：建表、字段、记录、视图、统计、公式/lookup、表单、仪表盘、workflow、角色权限；遇到 Base/多维表格/bitable 或 /base/ 链接时使用。文件导入转 lark-drive，认证/授权转 lark-shared。"
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
- 把本地 Excel / CSV / `.base` 导入成 Base，转 `lark-drive +import --type bitable`。
- 泛化数据分析、字段设计、公式讨论，但没有 Base/多维表格上下文。

## 使用边界

- Base 业务操作只使用 `lark-cli base +...` shortcut，不使用旧聚合式 `+table / +field / +record / +view / +history / +workspace`。
- 本轮 Base 不依赖 `lark-cli schema`。SKILL 只保留路由、风险和复杂 JSON/DSL；简单命令由命令自身的参数、tips 和错误恢复承接。
- 用户要把 Excel / CSV / `.base` 导入成 Base 时，先转 `lark-cli drive +import --type bitable`，导入完成后再回到 Base 命令。
- 用户只给 Base 名称或关键词时，先用 `lark-cli drive +search --query <keyword> --doc-types bitable` 定位资源。
- Base 命令必须先有 `base_token` 或可解析出的 Base URL。没有 token 时：用户要新建就用 `+base-create`；用户给标题/关键词就搜 `lark-cli drive +search --query "<base title>" --doc-types bitable --only-title --as user`；仍无法定位时，反问用户具体是哪一个 Base。
- 认证、初始化、scope、身份切换、权限不足恢复属于 `lark-shared`；Base 文档只保留会影响 Base 路径选择的权限规则。

## 快速路由

| 用户目标 | 优先命令 | 何时读 reference |
|---|---|---|
| 查 Base 本体 | `+base-get` | 用返回确认 Base 名称、owner、权限和可继续操作的 token |
| 创建/复制 Base | `+base-create` / `+base-copy` | 写入后报告新 Base 标识；注意返回中的 `permission_grant` |
| 查看 Base 内资源目录 | `+base-block-list` | 想先了解一个 Base 里有哪些 table/docx/dashboard/workflow/folder 时优先用它；返回 ID 关系和 fewshot 看 `--help` |
| 管理 Base 内资源目录 | `+base-block-create/move/rename/delete` | 创建或整理 Base 直接管理的 folder/table/docx/dashboard/workflow；资源内容继续用对应命令 |
| 管理数据表 | `+table-list/get/create/update/delete` | 处理 table 的列出、详情、创建、重命名和删除 |
| 列/查/删字段 | `+field-list/get/delete/search-options` | 写入前用 list/get 确认字段类型、选项、ID；删除前确认目标字段 |
| 创建/更新字段 | `+field-create` / `+field-update` | 必读 [lark-base-field-json.md](references/lark-base-field-json.md)；公式读 [formula-field-guide.md](references/formula-field-guide.md)；lookup 读 [lookup-field-guide.md](references/lookup-field-guide.md)；命令细节读 [lark-base-field-create.md](references/lark-base-field-create.md) / [lark-base-field-update.md](references/lark-base-field-update.md) |
| 读记录明细 | `+record-get` / `+record-list` / `+record-search` | 涉及筛选、排序、Top/Bottom N、聚合、多表关联、全局结论时读 [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md) |
| 写记录 | `+record-upsert` / `+record-batch-create` / `+record-batch-update` | 必读 [lark-base-record-upsert.md](references/lark-base-record-upsert.md) / [lark-base-record-batch-create.md](references/lark-base-record-batch-create.md) / [lark-base-record-batch-update.md](references/lark-base-record-batch-update.md) 和 [lark-base-cell-value.md](references/lark-base-cell-value.md) |
| 附件字段 | `+record-upload-attachment` / `+record-download-attachment` / `+record-remove-attachment` | 附件不要伪造成普通 CellValue；上传走本地文件，下载/删除按 file token 或字段定位 |
| 删除记录 / 分享记录链接 / 历史 | `+record-delete` / `+record-share-link-create` / `+record-history-list` | 删除前确认 record；分享链接最多 100 条；历史读 [lark-base-record-history-list.md](references/lark-base-record-history-list.md)，只查单条记录，不做整表审计 |
| 管理视图 | `+view-*` | `+view-set-filter` 读 [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md)；其余配置先 get 现状，再按返回结构更新 |
| 一次性聚合统计 | `+data-query` | 必读 [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md) 和入口 [lark-base-data-query-guide.md](references/lark-base-data-query-guide.md)；完整 DSL 再读 [lark-base-data-query.md](references/lark-base-data-query.md) |
| 公式字段 | `+field-create/update --json '{"type":"formula",...}'` | 必读 [formula-field-guide.md](references/formula-field-guide.md)，读后再加隐藏确认 flag `--i-have-read-guide` |
| Lookup 字段 | `+field-create/update --json '{"type":"lookup",...}'` | 必读 [lookup-field-guide.md](references/lookup-field-guide.md)，读后再加隐藏确认 flag `--i-have-read-guide` |
| 表单提交 | `+form-submit` | 先读 [lark-base-form-detail.md](references/lark-base-form-detail.md) 获取题目、filter 和附件所需 `base_token`；提交 JSON 读 [lark-base-form-submit.md](references/lark-base-form-submit.md) |
| 表单题目创建/更新 | `+form-questions-create` / `+form-questions-update` | 读 [lark-base-form-questions-create.md](references/lark-base-form-questions-create.md) / [lark-base-form-questions-update.md](references/lark-base-form-questions-update.md) |
| 其他表单管理 | `+form-list/get/detail/create/update/delete` / `+form-questions-list/delete` | `+form-detail` 读 [lark-base-form-detail.md](references/lark-base-form-detail.md)；删除前确认目标表单 |
| 仪表盘与组件 | `+dashboard-*` / `+dashboard-block-*` | 提到图表/看板/block 时先读 [lark-base-dashboard.md](references/lark-base-dashboard.md)；组件 `data_config` 读 [dashboard-block-data-config.md](references/dashboard-block-data-config.md)；读取图表计算结果用 `+dashboard-block-get-data` |
| Workflow | `+workflow-*` | 创建/更新或理解 steps 时读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) 和 steps JSON SSOT [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)；list/get/enable/disable 只处理 workflow ID 与启停状态 |
| 高级权限与角色 | `+advperm-*` / `+role-*` | 角色操作先读入口 [lark-base-role-guide.md](references/lark-base-role-guide.md)；角色 create/update 或解读完整配置再读权限 JSON SSOT [role-config.md](references/role-config.md)；系统角色不可删除；关闭高级权限会影响自定义角色 |

## Base 心智模型

- Base 曾用名 Bitable；返回字段、错误或旧文档里的 `bitable` 多为历史兼容，不代表应改走裸 API 或另一套命令。
- `+base-block-list` 是查看一个 Base 内资源目录的新入口：它列出这个 Base 直接管理的 `folder/table/docx/dashboard/workflow`，适合先判断 Base 里有什么，再决定走 table、dashboard、workflow 或 docx 命令。
- `base-block` 只负责资源目录管理，包括创建资源、移动到 folder、重命名和删除；具体资源内容仍走 table/dashboard/workflow 命令。
- 表、字段、视图、workflow、dashboard block 的名称和 ID 必须来自真实返回，不要凭用户口述猜。
- 存储字段可写；系统字段、`formula`、`lookup` 只读；附件字段走专用 attachment 命令。
- 一次性原始记录查询优先用 `+record-list` / `+record-search` 的 filter/sort；聚合分析优先用 `+data-query`；需要长期显示在表中时，才新增 `formula` / `lookup` 字段。
- `formula` 适合常规计算、条件判断、文本/日期处理和长期派生指标；`lookup` 适合明确的跨表查找、筛选后取值或聚合引用。
- 写入、分析、公式、lookup、workflow、dashboard 前，先读取真实结构：表、字段、视图、关联表和 dashboard block 名称都以命令返回为准。
- 跨表场景必须读取目标表结构；link 单元格中的关联 `record_id` 只是连接键，最终回答要回查并展示用户可读字段。

## 身份与权限降级

- 默认显式使用 `--as user` 操作用户资源；只有用户明确要求应用身份时，才直接用 `--as bot`。
- user 身份报 scope/授权不足，或错误中包含 `permission_violations` / `hint`，先转 `lark-shared` 做用户授权恢复，不要直接降级 bot。
- user 身份报资源级无访问且无授权恢复提示时，才可用 `--as bot` 重试一次；bot 仍失败就停止重试并按权限错误处理。
- `91403` 或明确不可访问错误不要循环换身份重试。
- `+base-create` / `+base-copy` 若用 bot 身份执行，关注返回中的 `permission_grant`，并把用户是否可打开新 Base 告知用户。

## 查询与统计规则

涉及查询、统计或判断结论时，先阅读 [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md)，并遵守：

1. `+record-list` 的默认页、固定 `--limit` 和本地 `jq` 只能证明已读取范围内的事实，不能直接支撑全局最值、全量计数、Top/Bottom N、异常识别或分组结论。
2. 能由 Base 表达的筛选、排序、投影、聚合、分组和限制，应在 Base 云端查询能力中执行；不要先拉原始记录到本地上下文再手工筛选排序。
3. `has_more=true` 或等价分页信号表示当前结果不是全量；除非用户只要样例/前 N 条，不能基于该页回答全局问题。
4. 多表查询必须先确认关系字段和连接键；link 单元格里的 `record_id` 是关系键，不是用户可读答案。
5. 最终答案必须能追溯到真实表、真实字段、查询范围、筛选/排序/聚合条件和必要的连接键。
6. 一次性原始记录查询优先用 `+record-list` / `+record-search` 的 filter/sort；聚合分析优先用 `+data-query`；要把结果长期显示在表里，才考虑新增 `formula` / `lookup` 字段。
7. `+data-query` 可返回聚合结果或维度字段行，但维度行按字段组合去重且不返回 `record_id`；需要逐条记录、记录定位或完整行级字段时，再用 `+record-list` / `+record-search` / `+record-get` 回查。

## 写入前置规则

- 写记录前先读字段结构；只写存储字段。系统字段、附件字段、`formula`、`lookup` 不作为普通记录写入目标。
- 附件上传、下载、删除走专用 `+record-*-attachment` 命令。
- 写字段前先读 [lark-base-field-json.md](references/lark-base-field-json.md)；涉及 `formula` / `lookup` 时必须读 [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)。
- 表名、字段名、视图名、workflow 配置中的名称必须来自真实返回；跨表场景还要读取目标表结构。
- 删除、角色更新、字段更新等高风险操作遵循 CLI 的 confirmation gate；目标不明确时先用 get/list 消歧。
- 批量写入单批最多 200 条；连续写同一表时串行执行，遇到 `1254291` 按短暂等待后重试处理。
- `+record-batch-update` 是“同值批量更新”：同一份 patch 应用到全部 `record_id_list`，不要拿它做逐行不同值映射。
- select/multiselect 写入未知选项可能触发平台新增选项；不是要新增时，先用 `+field-list` 或 `+field-search-options` 确认可选值。

## 表单与视图细节

- `+form-submit` 前必须先跑 `+form-detail`，读取 `questions[].type`、`required`、`filter` 和附件场景需要的 `base_token`；不要填写被 filter 隐藏的问题。
- 表单附件不要写进 `fields`，放在 `--json.attachments`；提交附件时必须同时传表单所属 Base 的 `--base-token`。
- `+view-set-filter` 是唯一保留的 view reference；sort/group/card/timebar/visible-fields 这类配置先用对应 get 命令读现状，保留未修改字段，只替换用户要求变更的配置。
- 视图适合持久化、共享和 UI 复用；一次性筛选/排序可先用 `+record-list` / `+record-search` 的 filter/sort 验证结果，再按需要沉淀为持久视图。

## Token 与链接

| 输入类型 | 含义 / 正确处理方式 |
|---|---|
| `/base/{token}` | 普通 Base 链接；提取 `/base/` 后的 token 作为 `--base-token` |
| `/wiki/{token}` | Wiki 节点链接；先 `wiki +node-get`，当 `data.obj_type=bitable` 时使用 `data.obj_token` 作为 `--base-token` |
| `/base/{token}?table={id}` | `table` 参数用于定位 Base 内对象：`tbl` 开头是数据表 `--table-id`；`blk` 开头是 dashboard ID；`wkf` 开头是 workflow ID |
| `/base/{token}?view={id}` | `view` 参数用于定位表视图，提取为 `--view-id`；通常还需要确认 `table` 参数或先查表结构 |
| `/share/base/form/{shareToken}` | 表单分享链接；这是表单 share token，走 `+form-detail` / `+form-submit --share-token <shareToken>` |
| `/share/base/view/{shareToken}` | 视图分享链接；具有分享权限语义，暂不支持用 CLI 直接访问，引导用户在浏览器或飞书客户端打开 |
| `/share/base/dashboard/{shareToken}` | 仪表盘分享链接；具有分享权限语义，暂不支持用 CLI 直接访问，引导用户在浏览器或飞书客户端打开 |
| `/record/{shareToken}` | 记录分享链接；暂不支持用 CLI 直接访问，引导用户在浏览器或飞书客户端打开。若用户想生成现有记录的分享链接，用 `+record-share-link-create --base-token <base_token> --table-id <table_id> --record-ids <record_id>` |
| `/base/workspace/{token}` | BaseApp / workspace 链接；暂不支持用 CLI 直接访问 |

`wiki +node-get` 返回非 `bitable` 时，不继续使用 Base 命令：`docx` 转文档，`sheet` 转表格，其他云空间对象转对应 skill 或 drive。

## Dashboard / Workflow / Role

- Dashboard 的复杂点是 block 的 `data_config`，不是 list/get/create/delete 命令参数。创建或更新 block 前先读 [dashboard-block-data-config.md](references/dashboard-block-data-config.md)，组件必须串行创建；`+dashboard-arrange` 是服务端智能布局，只在用户明确要求重排/美化时执行。`+dashboard-block-get-data` 读取图表最终计算结果，不返回 block 名称、类型、布局或 `data_config`；需要元数据先用 `+dashboard-block-get`。
- Workflow 的复杂点是 `steps` 结构。创建、更新或解释完整 workflow 时读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) 和 steps JSON SSOT [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)；enable/disable/list 只需确认 workflow ID、当前启停状态和用户意图。
- Role 的复杂点是权限 JSON。角色操作先读入口 [lark-base-role-guide.md](references/lark-base-role-guide.md)；`+role-create` 只支持自定义角色；`+role-update` 是 delta merge；角色 create/update 或解读完整配置时读权限 JSON SSOT [role-config.md](references/role-config.md)。`+role-delete` 只适用于自定义角色，系统角色不可删除；删除角色和关闭高级权限前必须确认目标和影响。

## 常见恢复

| 错误 / 现象 | 恢复动作 |
|---|---|
| `param baseToken is invalid` / `base_token invalid` | 检查是否把 wiki token、workspace token 或完整 URL 当成了 `--base-token`；按 `Token 与链接` 重新定位真实 Base token |
| `not found` 且输入来自 Wiki 链接 | 优先检查是否把 wiki token 当成 base token，不要立刻改走裸 API |
| `1254045` 字段名不存在 | 重新 `+field-list`，使用真实字段名或字段 ID；注意空格、大小写和跨表字段 |
| `1254015` 字段值类型不匹配 | 先 `+field-list`，再按 [lark-base-cell-value.md](references/lark-base-cell-value.md) 构造 CellValue |
| 日期 / 人员 / 超链接字段报格式错误 | 日期用 `YYYY-MM-DD HH:mm:ss`；人员用 `[{ "id": "ou_xxx" }]`；超链接用 URL 或 markdown link 字符串 |
| formula / lookup 创建失败 | 先读 [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)，再按 guide 重建请求 |
| `ignored_fields` / `READONLY` | 移除只读字段，只写存储字段 |
| `1254104` | 批量超过 200，分批调用 |
| `1254291` | 并发写冲突，串行写入并在批次间短暂等待 |
| `91403` | 无权限访问该 Base，按 `lark-shared` 权限流程处理，不要盲目重试 |

## 保留 Reference

- [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md)：查询/统计/全局结论的选路 SOP
- [lark-base-data-query-guide.md](references/lark-base-data-query-guide.md) / [lark-base-data-query.md](references/lark-base-data-query.md)：聚合查询入口 fewshot 与 DSL SSOT
- [lark-base-cell-value.md](references/lark-base-cell-value.md)：记录 CellValue 构造
- [lark-base-field-json.md](references/lark-base-field-json.md)：字段 JSON 构造
- [formula-field-guide.md](references/formula-field-guide.md) / [lookup-field-guide.md](references/lookup-field-guide.md)：公式与 lookup 字段
- [lark-base-field-create.md](references/lark-base-field-create.md) / [lark-base-field-update.md](references/lark-base-field-update.md)：字段创建/更新命令级补充
- [lark-base-record-upsert.md](references/lark-base-record-upsert.md) / [lark-base-record-batch-create.md](references/lark-base-record-batch-create.md) / [lark-base-record-batch-update.md](references/lark-base-record-batch-update.md) / [lark-base-record-history-list.md](references/lark-base-record-history-list.md)：记录写入 JSON 与历史返回解释
- [lark-base-view-set-filter.md](references/lark-base-view-set-filter.md)：视图筛选 JSON
- [lark-base-form-detail.md](references/lark-base-form-detail.md) / [lark-base-form-submit.md](references/lark-base-form-submit.md) / [lark-base-form-questions-create.md](references/lark-base-form-questions-create.md) / [lark-base-form-questions-update.md](references/lark-base-form-questions-update.md)：表单详情、提交和复杂 JSON
- [lark-base-dashboard.md](references/lark-base-dashboard.md) / [dashboard-block-data-config.md](references/dashboard-block-data-config.md) / [lark-base-dashboard-block-get-data.md](references/lark-base-dashboard-block-get-data.md)：仪表盘、组件配置与图表结果协议
- [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md) / [lark-base-workflow-schema.md](references/lark-base-workflow-schema.md)：workflow 入口与 steps JSON SSOT
- [lark-base-role-guide.md](references/lark-base-role-guide.md) / [role-config.md](references/role-config.md)：角色入口与权限 JSON SSOT
