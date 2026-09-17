# 下拉单选 `select_static`

下拉菜单单选。**Card 2.0**。

## 最小示例

```json
{
  "tag": "select_static",
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
| `tag` | 是 | String | / | 固定 `select_static` |
| `options` | 否 | Array | / | 选项，见下 |
| `options[].text` | 是 | Object | / | 选项名，plain_text |
| `options[].value` | 是 | String | / | 选项回调值，**同组件内不可重复** |
| `options[].icon` | 否 | Object | / | 选项前缀图标（同 `div.icon`） |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `type` | 否 | String | default | `default`(带框) / `text`(纯文本) |
| `placeholder` | 否 | Object | / | 占位文本，plain_text |
| `initial_option` | 否 | String | / | 初始选中内容（覆盖 placeholder 和 initial_index） |
| `initial_index` | 否 | Int | / | 初始选中序号，0=不选，1=第一个 |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用 |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- 选项 `value` 必须唯一，否则交互异常、服务端无法区分选了哪个。
- 回调 `action.tag="select_static"` + `action.option`（选中项的 value）。
