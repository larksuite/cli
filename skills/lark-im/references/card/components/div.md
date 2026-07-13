# 普通文本 `div`

带样式的文本块，支持前缀图标和 label-value 字段对。**Card 2.0**。富文本用 `markdown` 组件。

## 最小示例

```json
{
  "tag": "div",
  "text": { "tag": "plain_text", "content": "示例文本" }
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `div` |
| `text` | 否 | Object | / | 文本对象，见下 |
| `text.tag` | 是 | String | plain_text | `plain_text` 或 `lark_md`（部分 Markdown，语法见 `markdown.md`） |
| `text.content` | 是 | String | / | 文本内容 |
| `text.text_size` | 否 | String | normal | `heading-0`~`heading-4` / `normal`(14px) / `notation`(12px) 等；可在 `config.style.text_size` 自定义 pc/mobile 不同字号 |
| `text.text_color` | 否 | String | default | 颜色枚举（见 `../resource/colors.md`），仅 `plain_text` 生效 |
| `text.text_align` | 否 | String | left | `left` / `center` / `right` |
| `text.lines` | 否 | Int | / | 最大显示行数，超出 `...` 省略 |
| `icon` | 否 | Object | / | 前缀图标，见下 |
| `icon.tag` | 否 | String | / | `standard_icon`（用 `token`+`color`，token 见 `../resource/icons.md`）或 `custom_icon`（用 `img_key`） |
| `width` | 否 | String | fill | `fill` / `auto` / `[16,999]px` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符；流式更新时 `text.element_id` 指定文本 |

> `fields` 字段（多列 label-value）：数组，每项 `{ is_short, text:{tag,content} }`，`is_short:true` 可并排。

## 易错点

- `text_color` 只在 `text.tag` 为 `plain_text` 时生效；`lark_md` 用内联 `<font color=red>` 着色。
