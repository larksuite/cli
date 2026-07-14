# base +form-questions-update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量更新多维表格表单/问卷中的问题配置（标题、描述、是否必填、显隐条件等）。

> [!CAUTION]
> `+form-questions-update` 是**题目配置全量覆盖**，不是 patch。对每个传入的题目，未携带的属性会回落为默认值，显式传空字符串 / `null` / 空数组会直接写入空或清空；如果要保留现有属性，必须先用 `+form-questions-list` 查出现状，再把要保留的字段一起带回 `--questions`。

## 命令

```bash
# 先读取现有题目配置，作为 read-modify-write 的基线
lark-cli base +form-questions-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id>

# 更新一个问题的标题，同时带回要保留的 required / description / visible_rule 等字段
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_001","title":"您的真实姓名是？","description":"请填写真实姓名","required":true,"visible_rule":null}]'

# 同时更新多个问题；每个对象都应是该题目的目标完整配置
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_001","title":"姓名（必填）","required":true},{"id":"q_002","title":"联系方式","required":false}]'
  
# 更新问题描述（纯文本），同时带回要保留的 title / required / visible_rule
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_001","title":"您的姓名","description":"请填写您的真实姓名","required":true,"visible_rule":null}]'
# 更新问题描述（含链接），同时带回要保留的 title / required / visible_rule
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_001","title":"反馈建议","description":"更多说明请参考[帮助文档](https://example.com/help)","required":false,"visible_rule":null}]'

# 更新题目显隐条件（visible_rule），同时带回要保留的 title / description / required
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_002","title":"发票抬头","description":"","required":false,"visible_rule":{"logic":"and","conditions":[["q_001","==","是"]]}}]'

# 清空题目显隐条件（使题目始终显示），同时带回要保留的 title / description / required
lark-cli base +form-questions-update \
  --base-token <base_token> \
  --table-id <table_id> \
  --form-id <form_id> \
  --questions '[{"id":"q_002","title":"发票抬头","description":"","required":false,"visible_rule":null}]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token（base_token） |
| `--table-id <id>` | 是 | 数据表 ID |
| `--form-id <id>` | 是 | 表单 ID |
| `--questions <json>` | 是 | 问题更新 JSON 数组，最多 10 个（见下方格式） |
| `--format` | 否 | 输出格式：json（默认）\| pretty \| table \| ndjson \| csv |
| `--as` | 否 | 身份：user（默认）\| bot |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## `--questions` 格式

每个问题对象必须包含 `id`。注意：对象不是增量 patch，而是该题目的目标完整配置；未携带字段会按服务端默认值重建。

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | **是** | 问题 ID（field_id），不可修改 |
| `title` | 否 | 目标问题标题；省略会回落为字段名，传空字符串会写入空标题（若服务端允许） |
| `description` | 否 | 目标问题描述（纯文本或 Markdown 链接，如 `[文本](https://example.com)`）；省略或传空字符串都会清空描述 |
| `required` | 否 | 目标是否必填；省略会回落为 `false` |
| `option_display_mode` | 否 | 目标选项展示方式（仅 `select` 有效）：`0`=下拉，`1`=纵向（默认），`2`=横向；省略会回落默认展示方式 |
| `visible_rule` | 否 | 目标题目显隐条件；传完整 `{logic, conditions}` 对象覆盖，传 `null` 或省略都会清空（见下方说明） |

## 全量覆盖语义

- 先执行 `+form-questions-list`，读取被更新题目的当前 `id`、`title`、`description`、`required`、`option_display_mode`、`visible_rule`。
- 构造 `--questions` 时，只改用户明确要求变化的字段；所有仍要保留的字段必须按当前值一并传回。
- 不要用“只传要改的字段”的方式更新题目。比如只传 `{"id":"q_002","title":"新标题"}` 会让 `description` 清空、`required` 回落为 `false`、`visible_rule` 清空。
- 用户明确要求清空时才传空值：`description:""` 清空描述，`visible_rule:null` 清空显隐条件，`conditions:[]` 也表示无条件显示。

### `visible_rule` 显隐条件

> **仅当用户明确要求为题目设置或修改显隐条件（显示/隐藏逻辑）时，才需要读下面的结构说明；否则忽略本节。**

`visible_rule` 控制题目显示/隐藏，**结构与视图筛选 `filter` 完全一致**（`{logic?, conditions?}`），共用同一套公共协议。

- `conditions` 中的 `field` 引用**同一表单内其他题目的题目名称或题目 ID**（推荐用题目 ID）。
- 更新时按表单中题目的**实际顺序**判定，只能引用排在当前题目之前的题目；不支持循环引用。
- 更新 `visible_rule` 需传**完整**的 `{logic, conditions}` 对象（整体覆盖）；要保留现有显隐条件就必须把当前 `visible_rule` 原样带回；传 `null`、省略 `visible_rule` 或传空 `conditions` 都会使题目始终显示。
- 列出题目（`+form-questions-list`）会在每个题目对象中**原样返回** `visible_rule`；未设置显隐条件的题目返回 `null` 或 `conditions` 为空数组。

```json
{
  "logic": "and",
  "conditions": [
    ["q_001", "==", "是"],
    ["q_003", ">=", 1000]
  ]
}
```

详细的 `visible_rule` 结构（顶层规则、operator 列表、各题目类型的 value 写法）请阅读 [lark-base-filter-condition.md](lark-base-filter-condition.md)。

## 输出格式

返回更新后的问题列表：

```json
{
  "ok": true,
  "data": {
    "items": [
      {"id": "q_001", "title": "姓名（必填）", "required": true}
    ]
  }
}
```

## 工作流

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

1. 先用 `+form-questions-list` 获取现有问题及其 `id` 和完整配置。
2. 以现有配置为基线，只修改用户明确要求变化的字段；要保留的字段必须原样带回。
3. 构造包含 `id` 和目标完整配置的更新数组。
4. 执行命令并报告更新结果。

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-filter-condition.md](lark-base-filter-condition.md) — `visible_rule` / `filter` 条件结构公共协议
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
