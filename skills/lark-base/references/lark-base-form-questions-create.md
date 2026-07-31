# base +form-questions-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

向多维表格表单/问卷中批量添加问题。

## 命令

```bash
# 添加一个文本必填问题
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名是？","required":true}]'

# 添加多个问题（按顺序排列）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名是？","required":true},{"type":"text","title":"您的联系方式是？","required":false}]'

# 添加单选题（带选项）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"select","title":"满意度评价","required":true,"multiple":false,"options":[{"name":"非常满意","hue":"Green"},{"name":"满意","hue":"Blue"},{"name":"一般","hue":"Yellow"}]}]'

# 添加评分题
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"number","title":"服务评分","style":{"type":"rating","icon":"star","min":1,"max":5}}]'
  
# 添加带描述的问题（纯文本）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"您的姓名","description":"请填写真实姓名"}]'
# 添加带描述的问题（含链接）
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"text","title":"反馈建议","description":"更多详情请查看[帮助文档](https://example.com/help)"}]'  

# 添加带显隐条件（visible_rule）的问题：当「是否需要发票」选择「是」时才显示「发票抬头」
lark-cli base +form-questions-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"type":"select","title":"是否需要发票","required":true,"options":[{"name":"是","hue":"Blue"},{"name":"否","hue":"Gray"}]},{"type":"text","title":"发票抬头","visible_rule":{"logic":"and","conditions":[["是否需要发票","==","是"]]}}]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token（base_token） |
| `--table-id <id>` | 是 | 数据表 ID |
| `--form-id <id>` | 是 | 表单 ID |
| `--questions <json>` | 是 | 问题 JSON 数组，最多 10 个（见下方格式） |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## `--questions` 格式

每个问题对象支持以下字段：

| 字段                    | 必填 | 说明 |
|-----------------------|------|------|
| `title`               | **是** | 问题标题（字段名） |
| `type`                | **是** | 题目类型：`text`、`number`、`select`、`datetime`、`user`、`attachment`、`location` |
| `description`         | 否 | 问题描述（纯文本或 Markdown 链接，如 `[文本](https://example.com)`） |
| `required`            | 否 | 是否必填（true/false） |
| `option_display_mode` | 否 | 选项展示方式（仅 `select` 有效）：`0`=下拉，`1`=纵向（默认），`2`=横向 |
| `multiple`            | 否 | 是否多选（`select`/`user` 类型有效，bool） |
| `options`             | 否 | 选项列表（仅 `select` 有效）：`[{"name":"选项1","hue":"Blue"}]`，hue 可选：`Red`/`Orange`/`Yellow`/`Green`/`Blue`/`Purple`/`Gray` |
| `style`               | 否 | 字段样式配置（见下方说明） |
| `visible_rule`        | 否 | 题目显隐条件（见下方「`visible_rule` 显隐条件」） |

### `style` 字段说明

| 类型 | style 结构 | 说明 |
|------|------|------|
| `text` | `{"type":"plain"}` | 当前仅支持 `plain` |
| `number` | `{"type":"plain","precision":2}` | precision 为小数位数 |
| `number`（评分） | `{"type":"rating","icon":"star","min":1,"max":5}` | icon 可选：`star`/`heart`/`thumbsup`/`fire`/`smile`/`lightning`/`flower`/`number` |
| `datetime` | `{"format":"yyyy/MM/dd"}` | format 可选：`yyyy/MM/dd`、`yyyy/MM/dd HH:mm`、`MM-dd`、`MM/dd/yyyy`、`dd/MM/yyyy` |

### `visible_rule` 显隐条件

> **仅当用户明确要求为题目设置显隐条件（显示/隐藏逻辑）时，才需要读下面的结构说明；否则忽略本节。**

`visible_rule` 控制题目在表单中的显示/隐藏：当条件满足时题目显示，不满足时隐藏；不传或 `conditions` 为空数组则题目始终显示。

- **结构与视图筛选 `filter` 完全一致**，即 `{logic?, conditions?}`，共用同一套公共协议。
- 与视图 `filter` 唯一的区别：`conditions` 中的 `field` 引用的是**同一表单内其他题目的题目名称或题目 ID**（推荐用题目 ID 以避免重名歧义），而不是数据表字段。
- **只能引用前序题目**：条件只能引用排在当前题目之前的题目——创建时按 `questions` 数组顺序判定（可引用同批次更靠前的新题目或表单中已有题目），不支持循环引用。
- 引用的题目必须真实存在，否则会报错。
- 列出题目（`+form-questions-list`）会在每个题目对象中**原样返回** `visible_rule`；未设置显隐条件的题目返回 `null` 或 `conditions` 为空数组。

```json
{
  "logic": "and",
  "conditions": [
    ["是否需要发票", "==", "是"],
    ["报销金额", ">=", 1000]
  ]
}
```

详细的 `visible_rule` 结构（顶层规则、operator 列表、各题目类型的 value 写法）请阅读 [lark-base-filter-condition.md](lark-base-filter-condition.md)。

## 输出格式

返回创建成功的问题列表：

```json
{
  "ok": true,
  "data": {
    "items": [
      {"id": "q_001", "title": "您的姓名是？", "required": true}
    ]
  }
}
```

## 工作流

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

1. 先确定表单所属的真实 `table_id`，并在整个表单管理工作流中复用它；仅在 ID 缺失或归属不明确时调用 `+table-list`。
2. 用 `+form-questions-list` 查看现有问题。问题 `id` 是承载该问题的 `field_id`，不是独立于数据表的临时 ID。
3. 除非用户明确要求同名的独立问题，否则目标标题已经存在时用 `+form-questions-update` 更新必填状态、标题或描述；不要创建同名问题后再删除旧问题。
4. 创建确实不存在的问题，或用户明确要求的同名独立问题，并报告新建的问题 ID。

`+form-questions-delete` 会删除承载问题的数据表字段，不能删除主字段问题。不要通过“新建重复问题再删除旧问题”来替换主字段。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-filter-condition.md](lark-base-filter-condition.md) — `visible_rule` / `filter` 条件结构公共协议
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
