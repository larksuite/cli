---
name: lark-base
version: 1.2.22
description: "飞书多维表格（Base）操作：建表、字段、记录、视图、统计、公式/lookup、表单、仪表盘、应用模式（BaseApp/AppMode 页面与组件）、Workspace 目录、workflow、角色权限、模板中心（多维表格模板分类/列表/搜索）；遇到 Base/多维表格/bitable、BaseApp/AppMode、/base/ 或 /app/ 链接时使用。BaseApp 不走 lark-apps；文件导入/导出转 lark-drive，认证/授权转 lark-shared。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli base --help"
---

# Base

普通 Base 是数据容器，由一棵 Base Block 资源树和 Base 级配置组成。`folder`、`table`、`docx`、`dashboard`、`workflow` 都是 Block 类型；Advanced Permission / Role 是 Base 级配置，不属于 Block。Table 是其中承载业务数据的核心 Block。Workspace 是组织 Base 与 BaseApp 的外层容器；BaseApp（AppMode）通过 Page 和组件组织 Base 数据，不是 Base 的别名。

## 身份选择（优先）

操作 Base 优先使用 `--as user`；用户明确要求应用身份时使用 `--as bot`。权限失败按 `lark-shared` 以原身份修复 scope 或资源 ACL；只有用户明确同意更换操作者时才切换身份。

## 进入前必做：解析目标实体

开始操作前先确定 `base_token` 和目标实体类型；上下文已提供 `<bitable>` / `<base_refer>` 标签及资源 ID 时直接使用。其余情况按意图选择入口：

1. **URL 或分享链接：** `lark-cli base +url-resolve --url '<url>' --as user`。Base URL 根据返回的 `resource_type` / `block_type` 及 `table_id`、`view_id`、`record_id`、`dashboard_id`、`workflow_id`、`docx_token`、`share_token` 等坐标进入对应模块；BaseApp `/app/` URL 返回 `app_token`，并在链接携带时返回 `workspace_token` 和 `page_id`。实体类型以解析结果为准。
2. **Base 标题或关键词：** `lark-cli base +title-resolve --title '<keyword>' --as user`。单一结果直接取得 `base_token`；多个候选结合标题、所有者和更新时间消歧，仍无法唯一确定时请用户选择。随后按下方 Base Block 资源模型定位目标实体。
3. **已有 Base 候选列表：** 用户要列出已有 Base 候选，且需要按最近访问、owner、创建人、时间、类型等维度筛选/排序时，转 `lark-cli drive +search --doc-types bitable --as user`。按标题/关键词定位单个 Base 仍用 `+title-resolve`。常见候选列表命令：
   - 最近访问：`lark-cli drive +search --doc-types bitable --sort open_time --opened-since 3m --page-size 20 --as user`
   - 只列我拥有的：加 `--mine`；如果要列“我创建的”，用 `--created-by-me`。
   - 从候选项拿到 URL 或 token 后，再用 `+url-resolve` 或 `+base-get` 进入 Base 业务命令。
4. **BaseApp：** 优先使用真实 `/app/` URL；已有 `workspace_token` 时可用 `+workspace-entity-list --type baseapp` 定位。两者都没有时请用户补充应用链接或 Workspace，不按名称全局猜测 `app_token`。

**读取 Base：** Base 信息用 `+base-get`，资源目录按下方 Base Block 资源模型读取。

**写入 Base：** 创建新 Base 使用一次 `+base-create --name <base-name> --table-name <table-name> --fields '<field-array>'` 同时创建 Base、首表和 fields；`+base-copy` 复制整个 Base；Base 内资源统一按下方 Block 生命周期管理。

## Base 模板中心

模板中心是公开的 Base 模板库，不是用户云空间里的已有 Base。用户想用现成模板创建新 Base，且没有指向已有对象的锚点（没有 Base URL、没有“我的/最近访问的表”、没有具体已存在的 Base 名）时，可读取 [lark-base-template-center.md](references/lark-base-template-center.md) 查找模板中心模板；`+template-categories` 列出公开模板分类，`+template-list` 按分类列出公开模板，`+template-search` 按业务关键词搜索公开模板。

## Base Block 资源模型

```text
Base
├── Base Block 资源树
│   ├── Table Block
│   │   ├── Field schema
│   │   ├── Records / CellValue
│   │   ├── Views
│   │   └── Forms / Questions
│   ├── Dashboard Block（布局容器）
│   │   └── Dashboard 内部 Blocks（图表、指标卡、文本）
│   ├── Workflow Block
│   │   └── Workflow definition（title、status、steps 执行图）
│   ├── Docx Block → docx_token / lark-doc
│   └── Folder Block → 子 Block
└── Base 级配置
    └── Advanced Permission / Roles
```

每个 Base Block 都有 `id`、`type`、可修改的 `name`、所在 Folder 的 `parent_id`，并在同级目录中具有顺序。`+base-block-list` 是统一发现入口；`+base-block-create` 创建 Block，`+base-block-rename` 修改名称，`+base-block-move` 通过 `--parent-id` 调整目录并通过 `--before-id` / `--after-id` 调整顺序，`+base-block-delete` 删除 Block。类型专属内容再由对应模块命令处理。

创建时已经明确类型专属初始内容，可直接使用对应构造命令一次完成：Table 用 `+table-create --fields`，Dashboard 用 `+dashboard-create` 设置主题，Workflow 用 `+workflow-create --json` 提交完整定义；Folder 和 Docx 使用 `+base-block-create`。

Block 的 `id` 按类型直接作为对应模块坐标：

| Block type | 模块坐标与内部内容 |
|---|---|
| `table` | `id` 即 `table_id`；内部包含 Field、Record、View 和 Form |
| `dashboard` | `id` 即 `dashboard_id`；内部包含图表、指标卡和文本等 Dashboard 组件 |
| `workflow` | `id` 即 `workflow_id`；内部包含 title、status 和 steps 执行图 |
| `docx` | Block 另带 `docx_token`；正文由 `lark-doc` 处理 |
| `folder` | `id` 是目录 Block ID，也可作为 `--parent-id`；只组织子 Block |

## Table Block（The Core）

Table 本身是 Base Block，也是 Base 的核心数据存储层；Field、Record、View 和 Form 是 Table 内部对象，不是 Base Block。业务数据查询、写入、关联、统计和分析都从 Table 开始。先用 `+table-list` 定位 Table；字段名和目标已知的普通读取可直接进入 Record 命令，只有写入、筛选或关联等依赖字段类型/schema 的任务才补 `+field-list`。多表的 `+field-list` 可以并发执行。基础的 Record / CellValue 读写直接按下方路径；reference 只承载高级分析、完整协议和边界细节。

**读取 Table：** `+table-list` 定位表，`+table-get` 读取详情。Table 专属复制使用 `+table-copy`，异步状态用 `+table-copy-status`；schema 和 records 由下方内部对象操作。

Table 下的大多数更新通过异步链路生效，接口成功返回后立即读取可能暂时看不到最新状态。优先以写入成功响应作为操作结果；任务必须确认最终状态时，先完成本轮相关变更，再统一读取验收，避免逐项写后立即读回。

### Field

Field 定义列 schema。`field_id` 是稳定列标识，`name` 是可修改的展示名称；Formula、Lookup、Link、Select 等属于 Field 类型或能力。

**读取 Field：** `+field-list` / `+field-get` / `+field-search-options`。**写入 Field：** 已有 Table 中创建多个字段时，优先向一次 `+field-create --json` 传字段对象数组；单字段更新和删除用 `+field-update` / `+field-delete`。创建和更新分别读取 [field-create](references/lark-base-field-create.md) / [field-update](references/lark-base-field-update.md)，由命令文档继续路由 Field JSON、Formula 和 Lookup 协议。`字段插件` 用于扩展基础字段能力：按同一行其他字段内容触发 LLM 生成，并写回已有目标字段；当前已确认目标字段支持文本、单选、数字，配置或触发前先读 [field-extension](references/lark-base-field-extension.md)。

### Record

Record 是 Table 中的一行数据，包含该记录在各个 Field 下的 CellValue。系统 `record_id` 是表内稳定、非空且唯一的主键，Table 的主字段只是展示字段。

#### 1. 读取记录或单元格

- 已知若干个 `record_id`：`+record-get --record-id <id1> --record-id <id2>`
- 关键词搜索：`+record-search --keyword <text> --search-field <field>`；至少指定一个搜索字段。
- 其余读取：`+record-list`；结构化条件和排序分别用 `--filter-json` / `--sort-json`。

行数较大、需要服务端谓词下推时，`--filter-json` 使用 tuple condition；最常用的筛选与完整日期范围写法：

```jsonc
{
  "logic": "and", // 全部条件成立；任一条件成立改为 "or"
  "conditions": [
    ["状态", "intersects", ["进行中", "暂停"]], // Select 命中任一选项
    ["标题", "intersects", "urgent"], // 文本包含
    ["备注", "non_empty"], // 非空；判断为空改用 "empty"，两者都不传 value
    ["金额", ">=", 100], // 数字比较；支持 ==、!=、>、>=、<、<=
    ["关联项目", "intersects", [{ "id": "recxxx" }]], // Link 包含目标记录
    ["业务日期", "==", "ExactDate(2026-08-07)"], // 具体一天：按 Base 时区匹配 2026-08-07 当天
    ["发生时间", ">", "ExactDate(2024-01-31 23:59:59.999)"], // 日期不支持 >=；用 > 前一天最后一毫秒表达含当天的下界
    ["发生时间", "<", "ExactDate(2024-03-01 00:00:00)"] // 2024 年 2 月范围上界：小于 3 月 1 日零点
  ]
}
```

完整操作符和各字段取值结构读取 [Filter 条件结构](references/lark-base-filter-condition.md)。

所有读取都重复传 `--field-id` 做最小字段投影，并统一写入 NDJSON artifact：`--format ndjson --output <path>.ndjson`。每行是一条 Record JSON，stdout 摘要包含 `records_count` 和 `has_more` 用于分页判断。

```bash
# Example: 行数较大时先筛选 Status 包含 Doing 的记录，再导出 20 条作为局部预览
lark-cli base +record-list \
  --base-token <base_token> --table-id <table_id> \
  --filter-json '{"logic":"and","conditions":[["Status","intersects",["Doing"]]]}' \
  --field-id Name --field-id Status --field-id Score --limit 20 \
  --format ndjson --output ./records-preview.ndjson --as user

PREVIEW_ROWS=5
head -n "$PREVIEW_ROWS" ./records-preview.ndjson
tail -n "$PREVIEW_ROWS" ./records-preview.ndjson
```

预计记录数少于 500 行时，建议不做谓词下推，直接拉取到本地用 jq 或 Python 处理；行数较大时可用 `--filter-json` 下推可表达的条件，正则、派生等无法下推的条件继续在本地处理。

```bash
# jq：对服务端筛选结果追加名称格式筛选，再投影必要字段
jq -c 'select((.Name // "") | test("^Task-[0-9]+$")) | {record_id, Name}' ./records-preview.ndjson

# Python：按行读取并做简单汇总
python3 - <<'PY'
import json

with open("records-preview.ndjson", encoding="utf-8") as stream:
    rows = (json.loads(line) for line in stream if line.strip())
    print(sum((row.get("Score") or 0) for row in rows))
PY
```

`--limit` 的缺省值是 2000，最大值是 2000，通常无需手动指定 limit 参数；支持 `--offset` 参数；只有 `has_more=false` 且查询范围符合问题时，才能当作完整结果。大表完整读取、View 范围读取、复杂 JOIN、集合/多值、时序、语义或专业统计分析时，读取 [Record 查询与分析 SOP](references/lark-base-record-query-and-analysis-sop.md)。

#### 2. 新增记录或更新记录单元格

一条 Record 是 `{字段名或 field_id: CellValue}`，常见 CellValue：

```jsonc
{
  "标题": "Created from shortcut", // text: string
  "官网": "[官网](https://example.com)", // text(url): 裸 URL 或 Markdown link
  "联系电话": "13800000000", // text(phone): 合法电话号码字符串
  "邮箱": "owner@example.com", // text(email): 合法邮箱字符串
  "单选": ["Todo"], // select: array<string>；单选时数组最多一个值；
  "标签": ["高优", "外部依赖"], // 多选 select: array<string>；必须是当前字段存在的选项；
  "工时": 8, // number: double，不经过格式化的纯数字
  "带时区时间": "2026-03-24T10:00:00+08:00", // datetime：带时区，遵循传入的时区
  "不带时区时间": "2026-03-24 10:00", // datetime：不带时区，自动按当前 Base 时区转换
  "毫秒时间戳": 1774317600000, // datetime：也支持 Unix 毫秒时间戳
  "已完成": false, // checkbox: boolean
  "负责人": [{ "id": "ou_123" }], // user(multiple=false): 数组最多一个元素
  "协作人": [{ "id": "ou_123" }, { "id": "ou_456" }], // user(multiple=true): 数组可包含多个元素
  "群聊": [{ "id": "oc_123" }, { "id": "oc_456" }], // group_chat(multiple=true)
  "关联任务": [{ "id": "rec456" }], // link: array<{id}>，record_id 来自目标表
  "坐标": { "lng": 116.397428, "lat": 39.90923 }, // location: {lng,lat}
  "清空": null, // 清空单元格，传 null
  "清空数组": [] // 清空数组类单元格，空数组和 null 都可以
}
```

附件使用专用 shortcut 上传、下载或移除。created_at, updated_at, created_by, updated_by, auto_number, formula, lookup 类型字段只读，若误写入单元格会返回 `ignored_fields` 表示这些字段被静默过滤，其余字段正常写入。

```bash
# 新增：成功时返回 record_id_list
lark-cli base +record-batch-create \
  --base-token <base_token> --table-id <table_id> \
  --json '{"create_records":[{"Name":"Task A","Status":["Todo"]},{"Name":"Task B","Score":20}]}' --as user

# 更新：每条记录只提交要改变的字段
lark-cli base +record-batch-update \
  --base-token <base_token> --table-id <table_id> \
  --json '{"update_records":{"<record_id_a>":{"Status":["Done"]},"<record_id_b>":{"Score":100}}}' --as user
```

大 payload 可用脚本生成 json 后用 `--json @file.json`。单批最多 200 条，超过后分批，同一 Table 串行写入；并行可能触发 `1254291` 并发冲突错误。

#### 3. 其他 Record 操作

- `+record-delete --base-token <base_token> --table-id <table_id> --record-id <id1> --record-id <id2>` 删除若干个记录
- `+record-share-link-create --base-token <base_token> --table-id <table_id> --record-id <id1> --record-id <id2>` 创建记录分享链接
- `+record-history-list` 查询单条记录的变更事件，读取 [历史记录协议](references/lark-base-record-history-list.md)
- 附件必须使用 `+record-upload-attachment` / `+record-download-attachment` / `+record-remove-attachment` 操作。

### View

View 是同一 Table records 上的持久化筛选、排序、分组和展示配置，共享底层 records，不产生数据副本。一次性查询直接使用 Record 读取；需要在 Base UI 中长期保存、共享或复用访问方式时使用 View。

**读取 View：** 使用 `+view-list` / `+view-get`，并通过 `+view-get-filter` / `+view-get-sort` / `+view-get-group` / `+view-get-visible-fields` / `+view-get-timebar` / `+view-get-card` 读取持久化配置。**写入 View：** 使用 `+view-create` / `+view-rename` / `+view-delete` 管理 View，并通过对应的 `+view-set-*` 更新筛选、排序、分组、可见字段、时间轴和卡片配置；筛选结构读 [View filter](references/lark-base-view-set-filter.md)，由该文档继续路由公共 condition 协议。

### Form

Form 依附于 Table，以 Field 作为题目，每次有效提交会创建一条 Record，适合信息收集、外部填写、条件题目和附件提交。

1. **读取 Table 中的表单配置：** 使用 `+form-list` / `+form-get` 读取表单，使用 `+form-questions-list` 读取题目配置；这些命令使用表单所属的 `base_token + table_id`。
2. **创建或修改 Table 中的表单配置：** 使用 `+form-create` / `+form-update` / `+form-delete` 管理表单；题目由 Table Field 承载，question ID 对应 `field_id`，创建和更新分别读取 [questions create](references/lark-base-form-questions-create.md) / [questions update](references/lark-base-form-questions-update.md)，删除使用 `+form-questions-delete`。
3. **调整表单题目显隐和顺序：** Form 在 `visible_fields` 接口中作为 View，`form_id` 传给 `--view-id`。用 `+view-get-visible-fields` 读取当前可见题目，再用 `+view-set-visible-fields` 提交最终需要展示的完整有序题目 ID 列表；省略当前可见题目会隐藏它，加入已有隐藏 Form 成员会重新展示，空列表会隐藏全部题目。目标只能包含已有 Form 成员，不会自动补主字段，也不会修改题目展示配置；仍显示题目的 `visible_rule` 只能引用位于它之前的可见题目。
4. **管理表单分享：** 使用 `+form-share-get` / `+form-share-update` 管理启停、访问范围和匿名/登录要求；更新前先读取现状，每次只修改一个字段，布尔值显式传 `true` 或 `false`。
5. **填写分享表单并提交：** 对表单分享链接使用 `+url-resolve` 取得 `share_token`，按 [Form detail](references/lark-base-form-detail.md) 执行 `+form-detail` 读取真实题目、必填项和显示条件，再按 [Form submit](references/lark-base-form-submit.md) 构造字段与附件并执行 `+form-submit`。

表单题目和字段的关系：

- `+form-questions-create` 支持两种形态：新建字段题目需要 `title` + `type`；已有字段题目需要 `use_existing_field:true` + `field_id`。已有字段题目只是把该字段加入表单，不创建新字段，也不改变已有记录数据；不要给该形态携带 `type`、`style`、`options` 等字段定义属性。
- 创建问题前先 `+form-questions-list`。若目标标题已经存在，除非用户明确要求同名独立问题，否则优先用 `+form-questions-update` 修改题目配置，不要先创建同名问题再删除旧问题。
- `+form-questions-delete` 是高风险写操作。默认会删除承载问题的底层 Field 及该字段所有记录数据；只想把题目移出表单并保留字段/数据时必须传 `--keep-field`。保留字段后可用 `+form-questions-create --questions '[{"use_existing_field":true,"field_id":"<field_id>"}]'` 加回表单。

## Dashboard Block

Dashboard Block 是 Base Block 树中的仪表盘容器，负责承载页面主题、布局和内部组件集合，本身不表示某一项图表数据。使用 `+dashboard-list` 定位容器，`+dashboard-get` 读取容器信息，`+dashboard-update` 修改主题，`+dashboard-arrange` 统一编排内部组件布局。

**管理 Dashboard 分享：** 使用 `+dashboard-share-get` / `+dashboard-share-update` 管理启停、访问范围和返回源 Base 入口；更新前先读取现状，每次只修改一个字段，显式 `false` 会被保留。

容器内部的图表、指标卡和文本等组件在 Dashboard API 中也称为 Block，但不属于 Base Block 树。内部 Block 分为三条操作路径：

1. **读取配置：** `+dashboard-block-list` / `+dashboard-block-get` 读取组件类型、布局和 `data_config`；文本组件的正文也属于配置。
2. **写入配置：** `+dashboard-block-create` / `+dashboard-block-update` / `+dashboard-block-delete` 管理组件，`data_config` 定义数据源、维度、指标、聚合或文本内容。
3. **读取内容：** `+dashboard-block-get-data` 读取图表、指标卡等数据组件的计算结果。

操作内部 Block 前先读 [Dashboard](references/lark-base-dashboard.md)，由该入口继续路由组件配置和结果协议。

## 应用模式与 Workspace 心智模型

Workspace 是组织 Base 和 BaseApp 的空间容器；BaseApp 创建时必须归属一个 Workspace。BaseApp 用 Page 组织界面，每个 Page 包含图表、列表或富文本组件；组件通过 `data_config` 引用 Base 数据，但不会改变 Base、Table、Field 和 Record 的归属关系。Workspace 负责资源归属，App 负责页面和组件，Base 负责数据。

1. **Workspace：** 使用 `+workspace-create`、`+workspace-entity-list` 和 `+workspace-move-in` 创建目录、列出其中的 Base/BaseApp 或移入资源。
2. **应用：** 使用 `+app-create` / `+app-get`；应用查询和创建依赖真实 `app_token` / `workspace_token`。
3. **页面：** 使用 `+app-page-list/get/create/rename/delete` 管理 Page。
4. **组件：** 使用 `+app-block-list/get/create/update` 读写组件配置，使用 `+app-block-get-data` 读取组件计算结果。

BaseApp、Workspace、Page 或组件任务开始前完整读取 [应用模式与 Workspace](references/lark-base-app.md)；构造组件 `data_config` 时继续读取 [应用组件配置](references/lark-base-app-block-data-config.md)。BaseApp 不走 `lark-apps`。当前不支持 BaseApp 复制、Page 完整复制、页面图标以及从 Workspace 移出资源；遇到这些目标按 reference 的能力边界处理，不以新建空对象或 Drive 移动冒充。

- BaseApp（应用模式）中的 Page 和组件使用 `app_token` / `page_id` / `block_id`，表、字段和记录仍使用组件所引用 Base 的 `base_token`；不要混用 token 或把 BaseApp 当作 Base 的别名。
- 复用现有 BaseApp block 的 `data_config` 只能作为结构模板，首次 Create/Update 前仍要逐项对齐用户显式要求；用户要求排序时必须显式写 `group_by[].sort.order` 或顶层 `sort.order`，不能用旧配置省略的方向或当前 `get-data` 结果顺序代替。
- 应用页面的 block 与仪表盘 block 是同一套底层实体，但 ID 体系不通用；按当前模块 reference 选择命令和配置协议。

## Workflow Block

Workflow 本身是 Base Block，其内部是一张由 `next` / `children` 连接的 steps 执行图；触发器、动作、条件分支和循环都是 step 类型。它适合定时执行、Record 新增或变更联动、消息通知、记录读写和跨系统调用。Workflow 分为三条操作路径：

1. **读取配置：** `+workflow-list` 定位流程，`+workflow-get` 读取 `title`、`status` 和完整 `steps` 执行图。
2. **写入配置：** `+workflow-create` 创建完整定义，`+workflow-update` 更新完整定义；构造或修改配置前读取 [Workflow](references/lark-base-workflow.md)，由该入口继续路由 step 类型和 schema。
3. **运行状态控制：** `+workflow-enable` / `+workflow-disable` 启用或停用已有 Workflow，不修改 steps 执行图。

## Advanced Permission（AdvPerm）

AdvPerm 为 Base 开启细粒度权限模式；Role 在此基础上配置 Base、Table、View、Field、Record、Dashboard 和 Docx 等资源的访问能力，适合按团队或职责限制可见范围、编辑能力、复制下载和数据访问规则。

**读取 AdvPerm：** `+base-get` 查看 `is_advanced`，`+role-list` / `+role-get` 查看角色。**写入 AdvPerm：** `+advperm-enable` / `+advperm-disable` 启停高级权限，`+role-create` / `+role-update` / `+role-delete` 管理角色。先读 [权限与角色](references/lark-base-advanced-permission-and-role.md)，由该入口继续路由权限 JSON 协议。

## Docx Block

Docx Block 是组织在 Base 目录中的飞书文档资源，适合把说明、方案和报告与数据表、仪表盘及流程放在同一 Base 中；正文仍使用标准 Docx 数据模型。

从 Base Block 资源目录按 `--type docx` 定位文档并取得 `docx_token`；正文读取、创建与编辑使用 `lark-doc`。

## Folder Block

Folder Block 只承担 Base 目录分组和层级组织。用 `+base-block-list --parent-id <folder_block_id>` 读取直接子项。

## 通用执行契约

- Update 先确认命令是完整替换还是 delta：完整替换使用可信当前配置做 read-modify-write，delta 只提交目标变更。
- 优先用写入返回确认结果；返回不足以确认或任务明确要求核验时再读回目标。
- 命令具有 confirmation gate 时，确认目标和影响后使用 `--yes`。

## 不在本 Skill 范围

- 认证、初始化、scope、身份切换和授权恢复 → `lark-shared`
- Excel、CSV、`.base` 等本地文件与 Base 之间的导入/导出转 `lark-drive`；在线复制走 `+base-copy`
- Base 内嵌 Docx 的正文编辑 → `lark-doc`；电子表格内容操作 → `lark-sheets`
