# 富文本 `markdown`

支持 Markdown + 部分 HTML 的富文本。最常用的内容组件。**Card 2.0**。

## 最小示例

```json
{
  "tag": "markdown",
  "content": "**标题**\n正文，<font color='red'>红字</font>，[链接](https://x)"
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `markdown` |
| `content` | 是 | String | / | Markdown 文本；JSON 里用 `\n` 换行 |
| `text_size` | 否 | String | normal | `heading-0`~`heading-4` / `normal`(14px) / `notation`(12px) 等；可在 `config.style.text_size` 自定义 pc/mobile 字号 |
| `text_align` | 否 | String | left | `left` / `center` / `right` |
| `icon` | 否 | Object | / | 前缀图标（同 `div.icon`） |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 常用语法

| 效果 | 语法 |
|---|---|
| 粗 / 斜 / 删除线 | `**粗**`、`*斜*`、`~~删~~`（前后留空格更稳） |
| 换行 | JSON 内 `\n`；或 `<br>` |
| 文字链接 | `[文字](https://x)`（必须带 http/https） |
| 带图标链接 | `<link icon='chat_outlined' …>文案</link>`（icon token 见 `../resource/icons.md`） |
| 彩色文本 | `<font color='red'>红字</font>`（color 枚举见 `../resource/colors.md`；链接文本不可着色） |
| 标签 | `<text_tag color='blue'>标签</text_tag>`（color：neutral/blue/turquoise/lime/orange/violet/indigo/wathet/green/yellow/red/purple/carmine） |
| @ 人 | `<at id=ou_xxx></at>`、`<at id=all></at>`、`<at ids=id1,id2></at>` |
| @所有人 | `<at id=all></at>`（需群主开权限，否则发送失败） |
| 人员卡片 | `<person id='ou_xxx' show_name=true show_avatar=true style='normal'></person>` |
| 数字角标 | `<number_tag>1</number_tag>`（0-99，可加 background_color/font_color/url） |
| 国际化时间 | `<local_datetime millisecond='' format_type='date_num'></local_datetime>` |
| 标题 | `# 一级` ~ `###### 六级`（大标题显丑，正文优先用加粗，见易错点） |
| 列表 | `- 项`（无序）/ `1. 项`（有序），4 空格一层缩进 |
| 引用 | `> 引用文字` |
| 行内/块代码 | `` `code` `` / ```` ```go ... ``` ````（可指定语言） |
| 分割线 | `<hr>` 或 `---`（需单独一行） |
| 图片 | `![hover文案](img_key)` |
| 表格 | 标准 MD 表格；除标题最多 5 行（超出分页），单组件 ≤4 表 |
| 飞书表情 | `:DONE:`、`:OK:` |

## 易错点

- **慎用大标题**：`#` / `##` / `###` 一~三级标题字号过大、显丑，正文里一律用 `**加粗**` 替代来突出重点。**唯一例外**是「指标卡」里用 `##` 放大数值（见 `../lark-im-card-style.md` 视觉规范）。
- **少用 `markdown` 的 `margin`**：间距优先交给父容器的 `vertical_spacing` / `padding`，多数情况置 `0px`；仅精细缩进时设非零值（见 `../lark-im-card-style.md` 间距纪律）。
- 2.0 不再支持旧的 `[xx]($urlVal)` + `href` 差异化跳转语法，改用 `<link>`。
- 要展示 Markdown 特殊字符（`* ~ > < [ ] ( ) # : _` 等）须 HTML 转义，如 `<`→`&#60;`、`*`→`&#42;`。
- `content` 里的引号注意与 JSON 转义；属性值用单引号可减少冲突。
