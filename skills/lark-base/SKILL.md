---
name: lark-base
version: 1.2.28
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

**写入 Base：** 创建新 Base 使用一次 `+base-create --name <base-name> --table-name <table-name> --fields '<field-array>'` 同时创建 Base、首表和 fields；`+base-copy` 复制整个 Base；更新 Base 级时区使用 `+base-update --base-token <base-token> --time-zone <IANA-time-zone>`（如 `Asia/Shanghai`）；Base 内资源统一按下方 Block 生命周期管理。

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

Table 本身是 Base Block，也是 Base 的核心数据存储层；Field、Record、View 和 Form 是 Table 内部对象，不是 Base Block。业务数据查询、写入、关联、统计和分析都从 Table 开始。先用 `+table-list` 定位 Table；字段名和目标已知的普通读取可直接进入 Record 命令，只有写入、筛选或关联等依赖字段类型/schema 的任务才补 `+field-list`。多表的 `+field-list` 可以并发执行。基础的 Record / CellValue 读写直接按下方路径。

**读取 Table：** `+table-list` 定位表，`+table-get` 读取详情。Table 专属复制使用 `+table-copy`，异步状态用 `+table-copy-status`；schema 和 records 由下方内部对象操作。

Table 下的实体更新均异步生效。更新接口返回成功即表示变更成功，应以该响应为准；立即回读可能仍返回旧状态，不能据此判定更新失败或重复提交。部分失败只处理未成功项。

### Field

Field 定义列 schema。`field_id` 是稳定列标识，`name` 是可修改的展示名称；Formula、Lookup、Link、Select 等属于 Field 类型或能力。

**读取 Field：** `+field-list` / `+field-get`；需要查看更多 select 选项时用 `+field-search-options`。**写入 Field：** 已有 Table 中创建多个字段时，优先向一次 `+field-create --json` 传字段对象数组；静态 select 字段的选项增删改用 `+manage-select-options`，其他单字段属性更新和删除用 `+field-update` / `+field-delete`。创建和更新分别读取 [field-create](references/lark-base-field-create.md) / [field-update](references/lark-base-field-update.md)，由命令文档继续路由 Field JSON、Formula 和 Lookup 协议。需要从人员或创建人字段生成部门、直属上级、工号等通讯录信息列时，读取 [contact-field-enrichment](references/lark-base-contact-field-enrichment.md)。`字段插件` 用于扩展基础字段能力：按同一行其他字段内容触发 LLM 生成，并写回已有目标字段；当前已确认目标字段支持文本、单选、数字，配置或触发前先读 [field-extension](references/lark-base-field-extension.md)。

**字段编组（Field group）：** 将相关字段组织在一起，在 WebUI 中聚合展示并提供共同说明，方便用户浏览和操作。创建字段较多的数据表时，建议按业务用途编组；操作方法见 [字段编组](references/lark-base-field-groups.md)。

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
    {
      "logic": "or",
      "conditions": [
        ["优先级", "intersects", ["P0"]],
        ["负责人", "non_empty"]
      ]
    }, // 嵌套条件组：优先级是 P0 或负责人非空
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

嵌套条件组仅支持 `+record-list` / `+record-search` 的 `--filter-json`。其他使用 Filter 条件的场景支持且仅支持单层 `conditions`，不得传入嵌套条件组。

完整操作符和各字段取值结构读取 [Filter 条件结构](references/lark-base-filter-condition.md)。

```bash
# Record 读取统一使用 --format ndjson --output <path>.ndjson；stdout 摘要中的 records_count / has_more 用于分页判断。
# 按条件读取记录；--limit 最大值为 10000
lark-cli base +record-list \
  --base-token <base_token> --table-id <table_id> \
  --filter-json '{"logic":"and","conditions":[["Status","intersects",["Doing"]]]}' \
  --limit 10000 \
  --format ndjson --output ./records.ndjson --as user

# 按关键词搜索记录；--limit 最大值为 10000
lark-cli base +record-search \
  --base-token <base_token> --table-id <table_id> \
  --keyword <keyword> --search-field <field> \
  --limit 10000 \
  --format ndjson --output ./search.ndjson --as user
```

优先拉取到本地用 jq 或 Python 分析；行数较大时可用 `--filter-json` 下推可表达的条件，或按下方“记录分析”选择云端聚合。

局部数据示例（JSONC 注释仅用于说明；实际 NDJSON 每行一个完整 JSON 对象，无注释，以下展开其中一条）：

`records.ndjson`：

```jsonc
{
  "record_id": "rec_a", // 表内唯一主键；写回时保留所属表和 record_id
  // 其余 key 是字段名；不假设非空或唯一，未排序时不依赖记录顺序
  // 单元格采用下方 Record Cell Schema；标量空值通常为 null，多值空值为 []
  "标题": "任务 A",
  "工时": null, // 空值不等于零
  "状态": ["Todo"], // 单选也返回数组
  "标签": [], // 多值展开会改变粒度，同时展开多个数组会产生笛卡尔积
  "负责人": [{"id": "ou_123", "name": "张三"}], // 人员 / 群组读取可带 name；按 ID 连接、去重，名称不代表唯一身份
  "关联任务": [{"id": "rec_target"}], // 目标表由 Field schema 指定，id 是该表的 record_id
  "附件": [{"file_token": "file_example", "size": 128, "name": "sample.txt"}],
  "日期": "2026-09-17T10:00:00+08:00", // RFC3339；绝对时刻比较用完整时间，本地日历分组用 Base 时区
  "计算结果": "16" // Formula / Lookup 读取为文本，运算前验证转换规则
}
```

配套 `records.manifest.json`（部分字段）：

```jsonc
{
  // 结合 NDJSON 解析；manifest 另有来源、时区等元数据
  // 整表读取省略 --view-id；指定 View 则传 --view-id，结论限于该 View
  // --filter-json / --sort-json 分别覆盖 View 筛选 / 排序，覆盖后不是原 View 口径
  "rev": 42,
  "records_count": 10000, // 本页条数
  "has_more": true, // 完整读取从 --offset 0 开始，直到 has_more=false；每页保存到不同文件
  "next_offset": 10000, // has_more=true 时使用返回值继续，不自行推算
  // 翻页保持同一表、View、筛选、排序和字段投影；多表分别完成读取
  // 预览、固定前 N 条或未到末页不能支持全局结论；交付时说明范围与完整性
  "columns": { // 字段名 → 稳定 ID 和类型；字段改名后通过这里核对映射
    "标题": {"field_id": "fld_title", "field_type": "text"},
    "工时": {"field_id": "fld_hours", "field_type": "number"},
    "负责人": {"field_id": "fld_owner", "field_type": "user"}
  }
}
```

#### 记录分析

1. **默认用本地分析** 用 record-list 完整拉取分析依赖的数据，然后用 Python / jq 处理 ndjson，这是功能最强大的分析方式，但在数据量较大时效率不高（比如分析范围超过 10000 行）。
2. **行数大且只需单表简单聚合时，可用 `+data-query`。** 支持筛选、分组、计数、数值汇总、去重计数、排序和 Top-K；读取 [data-query DSL](references/lark-base-data-query.md) 并按其流程使用。
3. **Base SQL：仅显式启用后可用。** 当上下文提供 `BASE_SQL_ENABLED=true` 时，读取 [Base SQL](references/lark-base-sql.md) 并按其流程使用；标记缺失或不为 true 时，沿用上述路径。

SQL / data-query 无法表达的计算转完整明细本地处理。

#### 2. 新增记录或更新记录单元格

**Record Cell Schema** 用于 Record 读取的单元格及记录写入值。写入字段映射为 `{字段名或 field_id: CellValue}`，常见 CellValue：

```jsonc
{
  "标题": "Created from shortcut", // text: string
  "待确认": "请 <at user_id=ou_123/> 确认", // text(plain): 人员提及；实际 open_id 用 lark-contact 查询；不发送通知
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

附件使用专用 shortcut 上传、下载或移除。created_at, updated_at, created_by, updated_by, auto_number, formula, lookup 类型字段只读，写入时会跳过不可写字段，`ignored_fields` 返回字段及原因，其余合法字段正常写入；向用户说明未能写入的字段及原因。

```bash
# 新增：成功时返回 record_id_list
lark-cli base +record-batch-create \
  --base-token <base_token> --table-id <table_id> \
  --json '{"create_records":[{"Name":"Task A","Status":["Todo"]},{"Name":"Task B","Score":20}]}' --as user

# 更新：提交值替换单元格，未提交字段不变；数组整体替换，不自动追加或合并。
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

View 共享 Table 的底层记录；没有特殊展示需求时优先使用 `grid`。读取已有视图用 `+view-list` / `+view-get`。

**所有 View 编辑前必读 [View 类型与生命周期](references/lark-base-view.md)**，包括创建、改名、配置修改（筛选、排序、分组、字段显隐、时间条、卡片）和删除。视图选型、适用配置及完整操作示例统一在该 reference 中。

### Form

Form 依附于 Table，以 Field 作为题目，每次有效提交会创建一条 Record，适合信息收集、外部填写、条件题目和附件提交。

1. **读取 Table 中的表单配置：** 使用 `+form-list` / `+form-get` 读取表单，使用 `+form-questions-list` 读取题目配置；这些命令使用表单所属的 `base_token + table_id`。
2. **创建或修改 Table 中的表单配置：** 使用 `+form-create` / `+form-update` / `+form-delete` 管理表单；题目由 Table Field 承载，question ID 对应 `field_id`，创建和更新分别读取 [questions create](references/lark-base-form-questions-create.md) / [questions update](references/lark-base-form-questions-update.md)，删除使用 `+form-questions-delete`。
3. **调整表单题目显隐和顺序：** Form 在 `visible_fields` 接口中作为 View，`form_id` 传给 `--view-id`；用 `+view-get-visible-fields` 读、`+view-set-visible-fields` 写。提交语义、回读核验与题序调整步骤读取 [questions update](references/lark-base-form-questions-update.md) 的「调整题目顺序」一节。
4. **设置题目输入范围限制：** 题目支持限制填写的字数、数值区间和可选日期区间，配置项是题目上的 `input_range_limit`，写在 `+form-questions-create` / `+form-questions-update` 的 `--questions` 里。三类 `type` 的字段名、相对日期与固定日期的写法，构造 JSON 前读取 [questions create](references/lark-base-form-questions-create.md) 的「`input_range_limit` 输入范围限制」一节。
5. **管理表单分享：** 表单可以开启/关闭分享，并控制谁能填写（组织内 / 任何人）、是否允许匿名、是否要求登录。读写用 `+form-share-get` / `+form-share-update`，字段含义、命令写法、访问范围取值与回读验收读取 [Form share](references/lark-base-form-share.md)。
6. **填写分享表单并提交：** 对表单分享链接使用 `+url-resolve` 取得 `share_token`，按 [Form detail](references/lark-base-form-detail.md) 执行 `+form-detail` 读取真实题目、必填项和显示条件，再按 [Form submit](references/lark-base-form-submit.md) 构造字段与附件并执行 `+form-submit`。
7. **配置提交后回查（查询页）：** 表单可以配一个查询入口，让填写人提交后凭自己的提交记录回查、修改已填内容。读写用 `+form-query-page-get` / `+form-query-page-create`，创建流程与失败后的恢复方式读取 [Form query page](references/lark-base-form-query-page.md)。
8. **切换填写页布局：** 填写页支持两种模式——题目列表（默认，一页展示全部题目，信息密度高、便于通览和回改）和一页一题（每屏只显示一道题，减少长表单的压迫感，移动端填写和逐题引导更友好），适合不同风格的收集场景。读写用 `+form-display-settings-get` / `+form-display-settings-update --display-mode`，取值与使用方法读取 [Form display settings](references/lark-base-form-display-settings.md)。
9. **获取分享二维码：** 表单的分享入口除链接外还有二维码，便于线下张贴或群内扫码填写。用 `+form-share-qrcode-get` 查看二维码结构（响应含 `share_url` 与 base64 图片），用 `+form-share-qrcode-download` 把二维码存成 PNG 文件；只要分享链接本身用第 5 条的 `+form-share-get`。具体用法读取 [Form share QR code](references/lark-base-form-share-qrcode.md)。
10. **管理提交与通知配置：** 使用 `+form-submission-settings-get/update` 管理提交时间、次数和修改权限，使用 `+form-notifications-get/update` 管理提交或定时通知，使用 `+form-submit-actions-get/update` 管理结果页和跳转，使用 `+form-lottery-get/action` 管理抽奖；每组都先 get，再只更新用户要求的配置。执行前读取 [表单配置](references/lark-base-form-config.md)。

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

仪表盘及内部组件操作前先读 [Dashboard](references/lark-base-dashboard.md)，由该入口按任务继续读取对应专项文档。

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
- 以写入响应确认成功项；检查失败项和 `ignored_fields`，只处理未成功项并说明原因。
- 命令具有 confirmation gate 时，确认目标和影响后使用 `--yes`。

## 不在本 Skill 范围

- 认证、初始化、scope、身份切换和授权恢复 → `lark-shared`
- Excel、CSV、`.base` 等本地文件与 Base 之间的导入/导出转 `lark-drive`；在线复制走 `+base-copy`
- Base 内嵌 Docx 的正文编辑 → `lark-doc`；电子表格内容操作 → `lark-sheets`
