# 人员选择-多选 `multi_select_person`

从候选人员中多选。**Card 2.0**。字段与 `select_person` 基本一致，差别在多选默认值。

## 最小示例

```json
{
  "tag": "multi_select_person",
  "name": "reviewers",
  "placeholder": { "tag": "plain_text", "content": "请选择" },
  "options": [
    { "value": "ou_xxx" },
    { "value": "ou_yyy" }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `multi_select_person` |
| `options` | 否 | Array | / | 候选人 `{value: open_id}`；为空或全无效时候选项为会话全体成员 |
| `selected_values` | 否 | String[] | / | 默认选中的 open_id 数组 |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `type` | 否 | String | default | `default`(带框) / `text`(纯文本) |
| `placeholder` | 否 | Object | / | 占位文本，plain_text |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用 |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- `options[].value` 只接受 **open_id**；默认选中用 `selected_values`（数组）。
- 回调返回选中的多个 open_id。
