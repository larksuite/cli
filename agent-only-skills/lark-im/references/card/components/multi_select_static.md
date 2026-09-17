# 下拉多选 `multi_select_static`

下拉菜单多选。**Card 2.0**。字段与 `select_static` 基本一致，差别在多选默认值。

## 最小示例

```json
{
  "tag": "multi_select_static",
  "name": "tags",
  "placeholder": { "tag": "plain_text", "content": "请选择" },
  "options": [
    { "text": { "tag": "plain_text", "content": "选项1" }, "value": "1" },
    { "text": { "tag": "plain_text", "content": "选项2" }, "value": "2" }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `multi_select_static` |
| `options` | 否 | Array | / | 选项 `{text:{plain_text}, value, icon?}`，`value` 不可重复 |
| `selected_values` | 否 | String[] | / | 默认选中的 value 数组 |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `type` | 否 | String | default | `default`(带框) / `text`(纯文本) |
| `placeholder` | 否 | Object | / | 占位文本，plain_text |
| `width` | 否 | String | default | `default`(带框固定282px) / `fill` / `[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用 |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- 选项 `value` 唯一；默认选中用 `selected_values`（数组）而非单选的 `initial_*`。
- 回调返回选中的多个值。
