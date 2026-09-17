# 人员选择-单选 `select_person`

从候选人员中单选一人。**Card 2.0**。

## 最小示例

```json
{
  "tag": "select_person",
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
| `tag` | 是 | String | / | 固定 `select_person` |
| `options` | 否 | Array | / | 候选人，每项 `{value: open_id}`；**为空或全无效时，候选项为会话内全体成员** |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `type` | 否 | String | default | `default`(带框) / `text`(纯文本) |
| `placeholder` | 否 | Object | / | 占位文本，plain_text |
| `initial_option` | 否 | String | / | 初始选中的 open_id，须在 options 内 |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用 |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- `options[].value` 只接受 **open_id**。
- 回调 `action.tag="select_person"` + `action.option`（选中人的 open_id）。
