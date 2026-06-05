# base +form-detail

通过表单分享 token 读取表单详情。只读操作，适合在提交表单前解析题目结构、必填项、显示条件和附件提交所需的 Base token。

## 何时使用

- 用户给出 `/share/base/form/{shareToken}` 表单分享链接，先提取最后一段作为 `--share-token`。
- 准备调用 `+form-submit` 前，必须先用 `+form-detail` 读取 `questions[]`。
- 只知道分享链接、还不知道 `base-token` / `table-id` / `form-id` 时，用 `+form-detail`；已在 Base 内部管理表单时，才用 `+form-get`。

```bash
lark-cli base +form-detail --share-token <share_token> --format pretty
```

## 读取重点

`+form-detail` 返回的关键字段：

| 字段 | 用途 |
|---|---|
| `base_token` | 表单所属 Base；提交附件时必须传给 `+form-submit --base-token` |
| `questions[].id` | 题目标识，通常对应字段 ID |
| `questions[].title` | 提交时使用的字段名/题目名，以真实返回为准 |
| `questions[].type` | 决定值格式；与字段类型和 `lark-base-cell-value.md` 对齐 |
| `questions[].required` | 判断必填项 |
| `questions[].filter` | 判断题目是否对当前提交可见；被隐藏的问题不要填写 |

题目除固定字段外，会按类型携带动态配置，例如 `select.options` / `select.multiple`、`number.style`、`datetime.style.format`、`user.multiple`、`link.link_table`、`formula.expression`、`lookup.from/select/where/aggregate`。提交前按返回结构构造值，不要猜题目类型或选项。

## filter 显示条件

`questions[].filter` 控制题目显示/隐藏：

```json
{
  "conjunction": "and",
  "conditions": [
    {"field_name": "是否携带家属", "operator": "is", "value": ["是"]},
    {"field_name": "参与人数", "operator": "isGreater", "value": [1]}
  ]
}
```

- `conjunction` 为 `and` / `or`，表示条件全部满足或任一满足。
- `conditions[].field_name` 引用其他题目的 `title`。
- `conditions[].operator` 常见为 `is`、`isNot`、`contains`、`doesNotContain`、`isEmpty`、`isNotEmpty`、`isGreater`、`isGreaterEqual`、`isLess`、`isLessEqual`。
- `isEmpty` / `isNotEmpty` 不需要 `value`。
- 附件题目的 filter 只适合 `isEmpty` / `isNotEmpty`。

如果当前已填写值不满足某题目的 `filter`，该题目视为隐藏，不应放入 `+form-submit --json.fields` 或 `--json.attachments`。

## 与 form-submit 的关系

提交普通字段：

```bash
lark-cli base +form-submit \
  --share-token <share_token> \
  --json '{"fields":{"姓名":"张三","评分":5}}'
```

提交附件字段：

```bash
lark-cli base +form-submit \
  --share-token <share_token> \
  --base-token <base_token_from_form_detail> \
  --json '{"fields":{"姓名":"张三"},"attachments":{"附件":["./report.pdf"]}}'
```

附件字段不要写进 `fields`；放在顶层 `attachments`，值为本地文件路径数组。
