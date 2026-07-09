# 标题 `header`

卡片顶部标题区（主/副标题、后缀标签、图标、主题色）。**Card 2.0**。挂在卡片根的 `header` 键下，不在 `body.elements` 内，单卡仅一个。

## 最小示例

```json
{
  "header": {
    "title": { "tag": "plain_text", "content": "卡片标题" },
    "template": "blue"
  }
}
```

## 字段

| 字段 | 必填 | 类型 | 说明 |
|---|---|---|---|
| `title` | 是 | Object | 主标题，`{tag:"plain_text"\|"lark_md", content}`，最多 4 行 |
| `subtitle` | 否 | Object | 副标题，同 title，最多 1 行；只配副标题会按主标题展示 |
| `template` | 否 | String | 主题色枚举，见下；默认 `default` |
| `text_tag_list` | 否 | Array | 后缀标签，最多 3 个，每项 `{tag:"text_tag", text:{tag:"plain_text",content}, color}` |
| `i18n_text_tag_list` | 否 | Object | 多语言后缀标签；与 `text_tag_list` 二选一，同配以多语言为准 |
| `icon` | 否 | Object | 前缀图标（同 `div.icon`） |
| `padding` | 否 | String | 内边距，默认 12px，[0,99]px |

**template 枚举**（13 色）：`blue` / `wathet` / `turquoise` / `green` / `yellow` / `orange` / `red` / `carmine` / `violet` / `purple` / `indigo` / `grey` / `default`。

**标签 color 枚举**：`neutral`/`blue`/`turquoise`/`lime`/`orange`/`violet`/`indigo`/`wathet`/`green`/`yellow`/`red`/`purple`/`carmine`。深浅档位及 RGBA 见 `../resource/colors.md`。

## 选色建议

按场景选 template 颜色见 `../lark-im-card-style.md` 意图表。常见语义：green=成功/完成，orange=警告，red=错误/危险，grey=失效/归档，blue=通用信息。
