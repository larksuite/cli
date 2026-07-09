# 勾选器 `checker`

任务勾选场景的交互组件，支持配置回调响应。仅支持手写 JSON，搭建工具不支持构建。**Card 2.0**。

## 最小示例

```json
{
  "tag": "checker",
  "name": "check_1",
  "checked": false,
  "text": { "tag": "plain_text", "content": "完成新品上市计划报告" },
  "behaviors": [{ "type": "callback", "value": { "key": "todo1" } }]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `checker` |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `checked` | 否 | Boolean | false | 初始勾选状态 |
| `text` | 否 | Object | / | `{tag:"plain_text"\|"lark_md", content, text_size?, text_color?, text_align?}`（text_color 见 `../resource/colors.md`） |
| `overall_checkable` | 否 | Boolean | true | 悬浮时整体是否有阴影效果 |
| `button_area` | 否 | Object | / | `{pc_display_rule:"always"|"on_hover", buttons:[<=3 个 button]}` |
| `checked_style` | 否 | Object | / | `{show_strikethrough, opacity}`，勾选后的内容样式 |
| `disabled` / `disabled_tips` | 否 | Boolean/Object | false / 空 | 禁用及禁用提示 |
| `hover_tips` | 否 | Object | 空 | 悬浮提示；与 `disabled_tips` 同配时后者生效 |
| `behaviors` | 否 | Array | / | `[{type:"callback", value:{...}}]`；**未配置时仅本地勾选生效，不触发回调** |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |
| `padding`/`margin` | 否 | String | 0 | [-99,99]px |

## 嵌套 / 易错点

- 可嵌套在 form / 交互容器 / column_set / collapsible_panel 内。
- 不配置 `behaviors` 时勾选仅前端本地生效，不会触发服务端回调——需要业务侧感知必须显式配置。
- 回调：`action.tag="checker"` + `action.checked`（布尔值）；form 内则读 `form_value[name]`。
