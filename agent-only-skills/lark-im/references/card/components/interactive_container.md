# 交互容器 `interactive_container`

整块可点击区域，统一定义内嵌内容的样式和交互（callback/open_url），适合卡片内的列表项、可点击卡片块。**Card 2.0**。

## 最小示例

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "has_border": true,
  "border_color": "grey",
  "corner_radius": "8px",
  "padding": "4px 12px 4px 12px",
  "behaviors": [{ "type": "callback", "value": { "key": "value" } }],
  "elements": [
    { "tag": "markdown", "content": "帮我生成一篇产品方案的框架" }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `interactive_container` |
| `elements` | 是 | Element[] | [] | 子节点，支持除 `form`/`table` 外的所有组件 |
| `behaviors` | 是 | Array | / | 点击整容器的交互：`callback`（回传）/ `open_url`（跳转），可同数组共存 |
| `width` | 否 | String | fill | `fill`/`auto`/`[16,999]px` |
| `height` | 否 | String | auto | `auto`/`[10,999]px` |
| `direction` | 否 | String | vertical | `vertical`/`horizontal` |
| `horizontal_align`/`vertical_align` | 否 | String | left/top | 对齐方式 |
| `background_style` | 否 | String | default | `default`/`laser`/颜色枚举/RGBA（见 `../resource/colors.md`） |
| `has_border` | 否 | Boolean | false | 是否展示 1px 边框 |
| `border_color` | 否 | String | grey | `has_border` 为 true 时生效 |
| `corner_radius` | 否 | String | 0px | `[0,∞]px` 或 `[0,100]%` |
| `padding`/`margin` | 否 | String | 4px,12px / 0px | 同间距写法 |
| `disabled` / `disabled_tips` | 否 | Boolean/Object | false / 空 | 禁用整容器及禁用提示 |
| `hover_tips` | 否 | Object | 空 | PC 端悬浮提示 |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |

## 嵌套 / 易错点

- 可嵌套除 `form`/`table` 外的所有组件，包括嵌套自身（列表项常见写法）。
- 若容器内有交互组件（如内部 `button`），优先响应该子组件的交互，容器级 `behaviors` 不会触发。
- 回调来源：`card.action.trigger`，`action.tag` 取决于内部触发的具体组件；容器本身被点击时 `action.value` 即容器 `behaviors.value`。
