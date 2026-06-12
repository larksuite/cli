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

## 名词与概念

| 名词 | 含义 |
|---|---|
| Base / 多维表格 / Bitable | 同一个东西：`/base/{token}` 链接对应的整个文档容器，token 即 `--base-token`；Bitable 是曾用名，只出现在历史 API 和返回字段里 |
| Table（数据表） | Base 内的一张数据表，ID `tbl` 开头；列是 field，行是 record |
| Field（字段）/ Record（记录） | 表的列与行；字段 ID `fld` 开头，记录 ID `rec` 开头 |
| View（视图） | 同一张 table 的一种展示配置（筛选/排序/分组等），ID `viw` 开头 |
| Form（表单） | 收集数据的入口，提交结果写入对应 table 的记录 |
| Workflow（工作流） | Base 内的自动化流程，ID `wkf` 开头，由 steps（trigger + action）组成 |
| Dashboard（仪表盘） | 数据可视化容器，ID `blk` 开头(因为它本身是 Base 资源目录里的一个 block，见下方歧义说明) |
| Chart（图表/组件） | 又叫Dashboard block, 是 dashboard 内的单个可视化组件（柱状图/饼图/指标卡等）, ID `cht` 开头 |
| Base block （`+base-block-*`）| Base 资源目录里的节点，table/docx/dashboard/workflow/folder 在目录层面统称 block。 “这个 Base 里有哪些东西” → `+base-block-list`|

**`block` 是易混淆词，同名不同义，按命令域区分：base-block 和 dashboard-block**

### Base 心智模型

- `base-block` 只负责资源目录管理，包括创建资源、移动到 folder、重命名和删除；具体资源内容仍走 table/dashboard/workflow 命令。
- 新建 Base 时，强烈推荐一次性执行 `lark-cli base +base-create --name "<base>" --table-name "<table>" --fields '<field-json-array>'`，同时配置新 Base 里唯一一个初始数据表的 name 和 schema；使用 `--fields` 前先读 [lark-base-field-json.md](references/lark-base-field-json.md) 或复用 `+field-create` 的字段 JSON 形状，不要猜字段属性。
- `+base-create` 不传 `--table-name` 和 `--fields` 时，会创建一个默认 schema 的初始数据表。
- 表、字段、视图、workflow、dashboard block 的名称和 ID 必须来自真实返回，不要凭用户口述猜。
- 存储字段可写；系统字段、`formula`、`lookup` 只读；附件字段走专用 attachment 命令。
- 一次性原始记录查询优先用 `+record-list` / `+record-search` 的 filter/sort；聚合分析优先用 `+data-query`；需要长期显示在表中时，才新增 `formula` / `lookup` 字段。
- `formula` 适合常规计算、条件判断、文本/日期处理和长期派生指标；`lookup` 适合明确的跨表查找、筛选后取值或聚合引用。
- 写入、分析、公式、lookup、workflow、dashboard 前，先读取真实结构：表、字段、视图、关联表和 dashboard block 名称都以命令返回为准。
- 跨表场景必须读取目标表结构；link 单元格中的关联 `record_id` 只是连接键，最终回答要回查并展示用户可读字段。

## 快速路由

| 用户目标 | 优先命令 | 何时读 reference |
|---|---|---|
| 查 Base 本体 | `+base-get` | 用返回确认 Base 名称、owner、权限和可继续操作的 token |
| 创建/复制 Base | `+base-create` / `+base-copy` | 新建时强烈推荐用 `--table-name` + `--fields` 同时配置新 Base 里唯一一个初始数据表的 name 和 schema；写入后报告新 Base 标识和 `permission_grant` |
| 查看 Base 内资源目录 | `+base-block-list` | 先判断 Base 里有什么（table/docx/dashboard/workflow/folder），再决定走哪类命令 |
| 管理 Base 内资源目录 | `+base-block-create/move/rename/delete` | 创建或整理 Base 直接管理的 folder/table/docx/dashboard/workflow；资源内容继续用对应命令 |
| 管理数据表 | `+table-list/get/create/update/delete` | 处理 table 的列出、详情、创建、重命名和删除 |
| 列/查/删字段 | `+field-list/get/delete/search-options` | 字段发现默认用 `+field-list --compact`；需要 formula/lookup 细节或完整字段 JSON 再用 `+field-get` / 不带 compact 的 list；多表结构用 `+field-list-batch --compact --table-id <表1> --table-id <表2>` 一次取齐，不要逐表调用 |
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
| Workflow | `+workflow-*` | 先读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md)：它包含查询/启停/创建/修改的最短路径和常见 step 组合；只有创建/更新复杂 steps 时才继续读 schema 小文件；list/get/enable/disable 不读 schema |
| 高级权限与角色 | `+advperm-*` / `+role-*` | 先读入口 [lark-base-role-guide.md](references/lark-base-role-guide.md)（含安全边界）；权限 JSON 再读 [role-config.md](references/role-config.md) |

## 注意事项

### 批量执行

能批量的操作尽量批量，不要一轮对话只处理一个对象。

- 优先用原生批量能力：多表字段 `+field-list-batch`；批量写记录 `+record-batch-create` / `+record-batch-update`；部分命令参数本身支持多值（如 `+record-delete --record-id` 可重复传、`+record-share-link-create --record-ids`），先看 `--help`。
- 没有原生批量命令时，对多个对象做同类操作在**一条 Bash 命令**里用 shell 循环完成。
- 只读命令可用 `--jq` 收窄输出，避免无关字段灌入上下文。脚本输出只打印计数、ID 和失败项，不要回显完整 payload 或原始返回

示例——一次取多个视图的配置：

```bash
for v in vewAAA vewBBB vewCCC; do
  echo "== $v"
  lark-cli base +view-get --base-token <base_token> --table-id <table_id> --view-id "$v" --as user
done
```

### 善用 help

- 参数不确定、要构造复杂 JSON、或命令带批量/隐藏选项时，先看对应reference或 `--help`，不要猜参数名或 JSON 结构；`+table-list` / `+base-create` 这类参数显而易见的简单命令直接执行，报参数错误再查 help，不要为它单花一轮。
- 需要看多个命令的 help 时，合并在一条 Bash 命令里一次看完。

### 身份与权限降级

- 默认显式使用 `--as user` 操作用户资源；只有用户明确要求应用身份时，才直接用 `--as bot`。
- user 身份报 scope/授权不足，或错误中包含 `permission_violations` / `hint`，先转 `lark-shared` 做用户授权恢复，不要直接降级 bot。
- user 身份报资源级无访问且无授权恢复提示时，才可用 `--as bot` 重试一次；bot 仍失败就停止重试并按权限错误处理。
- `91403` 或明确不可访问错误不要循环换身份重试。
- `+base-create` / `+base-copy` 若用 bot 身份执行，关注返回中的 `permission_grant`，并把用户是否可打开新 Base 告知用户。

### 查询与统计

- 涉及筛选、排序、Top/Bottom N、聚合、分组、多表关联或任何全局结论时，先读 [lark-base-data-analysis-sop.md](references/lark-base-data-analysis-sop.md) 并按其 Hard Rules 执行。
- 两条红线随时生效：能由 Base 云端表达的筛选/排序/聚合不要拉原始记录到本地手工处理；`has_more=true` 等分页信号未消除前，不能基于当前页下全局结论。

### 写入前置

- 写记录/字段前先读真实结构；表名、字段名、视图名必须来自真实返回，跨表场景还要读目标表结构。
- 复杂 JSON 按快速路由读对应 reference：字段读 [lark-base-field-json.md](references/lark-base-field-json.md)，记录读 [lark-base-cell-value.md](references/lark-base-cell-value.md)（写入红线：只写存储字段、批量上限、并发冲突等，见其顶层规则）。
- 删除、角色更新、字段更新等高风险操作遵循 CLI 的 confirmation gate；目标不明确先用 get/list 消歧；workflow/role 等复杂写操作创建后用 get 回读确认，必要时先 `--dry-run` 预演。

### 表单与视图

- `+form-submit` 前必须先 `+form-detail`；提交规则（filter 隐藏题不填、附件写在 `attachments` 并带 `--base-token`）见 [lark-base-form-submit.md](references/lark-base-form-submit.md)。
- 视图配置先用对应 get 命令读现状，只替换要变更的部分；一次性筛选/排序先用 `+record-list` / `+record-search` 验证，再按需沉淀为持久视图。

### Dashboard / Workflow / Role

- Dashboard 的复杂点是 block 的 `data_config`：创建/更新 block 前读 [dashboard-block-data-config.md](references/dashboard-block-data-config.md)，组件串行创建；布局/换图表类型/删除具名图表等操作要点见 [lark-base-dashboard.md](references/lark-base-dashboard.md) 的「执行要点」。`+dashboard-block-get-data` 只返回图表数据，元数据用 `+dashboard-block-get`。
- Workflow 的复杂点是 `steps`：先读入口 [lark-base-workflow-guide.md](references/lark-base-workflow-guide.md)，用其中的最短路径和场景表完成查询/启停/常见创建修改；需要具体 step 字段再按需读 schema 小文件；创建后 `+workflow-get` 回读验证。
- Role 的复杂点是权限 JSON：先读 [lark-base-role-guide.md](references/lark-base-role-guide.md)（含安全边界），权限 JSON SSOT 读 [role-config.md](references/role-config.md)；删除角色、关闭高级权限前确认目标和影响。

## Token 与链接

| 输入类型 | 含义 / 正确处理方式 |
|---|---|
| `/base/{token}` | 普通 Base 链接；提取 `/base/` 后的 token 作为 `--base-token` |
| `/wiki/{token}` | Wiki 节点链接；先 `wiki +node-get`，当 `data.obj_type=bitable` 时使用 `data.obj_token` 作为 `--base-token` |
| `/base/{token}?table={id}` | `table` 参数用于定位 Base 内对象：`tbl` 开头是数据表 `--table-id`；`blk` 开头是 dashboard ID；`wkf` 开头是 workflow ID |
| `/base/{token}?view={id}` | `view` 参数用于定位表视图，提取为 `--view-id`；通常还需要确认 `table` 参数或先查表结构 |
| `/share/base/form/{shareToken}` | 表单分享链接；这是表单 share token，走 `+form-detail` / `+form-submit --share-token <shareToken>` |
| `/share/base/view/...` / `/share/base/dashboard/...` / `/record/...` / `/base/workspace/...` | 分享链接与 workspace 链接，暂不支持用 CLI 直接访问，引导用户在飞书客户端打开；要生成记录分享链接用 `+record-share-link-create` |

`wiki +node-get` 返回非 `bitable` 时，不继续使用 Base 命令：`docx` 转文档，`sheet` 转表格，其他云空间对象转对应 skill 或 drive。

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
