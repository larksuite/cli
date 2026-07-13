# 日期选择器 `date_picker`

提供日期选项的交互组件，默认拥有交互能力（无需显式 `behaviors` 也会回调）。**Card 2.0**。

## 最小示例

```json
{
  "tag": "date_picker",
  "placeholder": { "tag": "plain_text", "content": "请选择" },
  "initial_date": "2024-01-01"
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `date_picker` |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `initial_date` | 否 | String | / | 初始值，格式 `yyyy-MM-dd`，会覆盖 `placeholder` |
| `placeholder` | 否 | Object | / | 占位文本，plain_text；未设 `initial_date` 时必填 |
| `width` | 否 | String | default | `default`/`fill`/`[100,∞)px` |
| `disabled` | 否 | Boolean | false | 是否禁用（需端版本 V7.4+） |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `margin` | 否 | String | 0 | [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / 交互容器内；搭建工具中暂不支持嵌套在交互容器中。
- 提醒用户注意时区语境（如预定海外酒店用酒店所在地时区）；服务端只返回用户当前时区作为参考，不代表用户选的就是该时区。
- 回调：`action.tag="date_picker"` + `action.option`（日期字符串，如 `"2025-06-10 +0800"`）+ `action.timezone`；form 内则读 `form_value[name]`。
