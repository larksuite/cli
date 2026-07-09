# 折叠按钮组 `overflow`

折叠多个选项按钮，点击展开。适用于操作较多的场景。**Card 2.0**。

## 最小示例

```json
{
  "tag": "overflow",
  "options": [
    { "text": { "tag": "plain_text", "content": "选项A" }, "value": "a" },
    { "text": { "tag": "plain_text", "content": "选项B" }, "value": "b" }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `overflow` |
| `options` | 是 | Array | / | 选项按钮，见下 |
| `options[].text` | 否 | Object | / | `{tag:"plain_text", content}`，≤100 字符 |
| `options[].value` | 否 | String | / | 点击回传值，用于区分点了哪个选项（回调 `action.option`） |
| `options[].multi_url` | 否 | Object | / | 跳转链接 `{url, pc_url, ios_url, android_url}` |
| `behaviors` | 否 | Array | / | 额外回传：`[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}`（均 plain_text） |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 嵌套 / 易错点

- 可嵌套在 form / collapsible_panel / 循环容器 / interactive_container / column_set 内。
- 多按钮时务必给每个 `options[].value`，否则回调无法区分点了哪个。
- 点击触发 `card.action.trigger`，回传 `action.tag = "overflow"` + `action.option`。
