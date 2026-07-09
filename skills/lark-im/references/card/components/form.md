# 表单容器 `form`

批量录入表单项后一次提交：用户在前端填写多个表单项，点击提交按钮后将所有值打包一次性回调到服务端。**Card 2.0**。

## 最小示例

```json
{
  "tag": "form",
  "name": "form_1",
  "elements": [
    { "tag": "input", "name": "reason", "required": true },
    {
      "tag": "button",
      "text": { "tag": "plain_text", "content": "提交" },
      "type": "primary",
      "form_action_type": "submit",
      "name": "Button_submit"
    }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `form` |
| `name` | 是 | String | / | 表单容器唯一标识，卡片内全局唯一，用于识别提交数据归属 |
| `elements` | 是 | Element[] | [] | 子节点，支持除 `table` 和 `form` 外的所有组件 |
| `direction` | 否 | String | vertical | `vertical` / `horizontal` |
| `horizontal_spacing`/`vertical_spacing` | 否 | String | 8px/12px | 间距枚举 `small`(4)/`medium`(8)/`large`(12)/`extra_large`(16) 或 `[0,99]px` |
| `horizontal_align` | 否 | String | left | `left`/`center`/`right` |
| `vertical_align` | 否 | String | top | `top`/`center`/`bottom` |
| `padding`/`margin` | 否 | String | 0 | [-99,99]px，支持单值/双值/四值写法 |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

### 子组件内嵌字段（交互组件嵌在 form 内时生效）

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | 是 | 表单内组件唯一标识，卡片全局唯一，否则提交失败 |
| `required` | 否 | 是否必填；为 true 且未填时点提交会本地拦截，不发起回调 |
| `form_action_type` | 是（按钮） | `submit`（提交）/ `reset`（重置初始值）；表单内按钮**不用** `behaviors` |

## 嵌套 / 易错点

- `form` 不支持嵌套 `table` 和 `form`；且 `form` 本身只能放卡片根节点下，不能被其他组件嵌套。
- form 内所有交互组件的 `name` 必须填且全局唯一，否则提交失败。
- 表单内必须包含一个 `form_action_type: submit` 的按钮。
- 回调来源：`card.action.trigger` 中 `action.tag="button"` + `action.form_value`（按组件 `name` 映射各字段值）。
