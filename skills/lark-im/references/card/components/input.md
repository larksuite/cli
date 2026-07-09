# 输入框 `input`

收集用户文本输入。常嵌在 `form` 内配合提交按钮使用。**Card 2.0**。

## 最小示例

```json
{
  "tag": "input",
  "name": "comment",
  "placeholder": { "tag": "plain_text", "content": "请输入" },
  "label": { "tag": "plain_text", "content": "备注：" }
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `input` |
| `name` | 否* | String | / | 唯一标识；**在 form 内必填且全局唯一**，用于识别提交数据 |
| `required` | 否 | Boolean | false | 是否必填（仅 form 内生效） |
| `placeholder` | 否 | Object | / | 占位文本，plain_text，≤100 字符 |
| `default_value` | 否 | String | / | 预填内容 |
| `label` | 否 | Object | / | 描述文本，plain_text |
| `label_position` | 否 | String | top | `top` / `left`（窄屏自动转 top） |
| `input_type` | 否 | String | text | `text` / `multiline_text`(多行，回调含 `\n`) / `password` |
| `rows` | 否 | Number | 5 | 多行时默认行数 |
| `auto_resize` | 否 | Boolean | false | 多行时高度自适应（仅 PC） |
| `max_rows` | 否 | Number | / | `auto_resize` 时最大行数 |
| `max_length` | 否 | Number | 1000 | 最大字符数，[1,1000] |
| `show_icon` | 否 | Boolean | true | password 时是否显示前缀图标 |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用（配 `disabled_tips` plain_text） |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- 在 form 内为**异步提交**：用户填完点提交按钮才一次性回调全部表单数据。
- 回调里 `action.tag="input"` + `action.input_value`（用户输入值）；form 提交则值在 `form_value` 内。
