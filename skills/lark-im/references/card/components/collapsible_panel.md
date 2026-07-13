# 折叠面板 `collapsible_panel`

折叠次要内容（备注、长文本），点标题展开/收起。**Card 2.0**。

## 最小示例

```json
{
  "tag": "collapsible_panel",
  "expanded": false,
  "header": { "title": { "tag": "plain_text", "content": "面板标题" } },
  "elements": [{ "tag": "markdown", "content": "折叠的内容" }]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `collapsible_panel` |
| `header` | 是 | Object | / | 标题区，见下 |
| `elements` | 否 | Array | / | 面板内组件；**不能放 `form`** |
| `expanded` | 否 | Boolean | false | 是否默认展开 |
| `background_color` | 否 | String | 透明 | 面板背景，颜色枚举（见 `../resource/colors.md`） |
| `border` | 否 | Object | / | `{ color, corner_radius }` |
| `direction` | 否 | String | vertical | `vertical` / `horizontal` |
| `vertical_spacing`/`horizontal_spacing` | 否 | String | 8px | 间距枚举或 [0,99]px |
| `padding` | 否 | String | 0 | 内边距 [0,99]px |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

**header 字段**：

| 字段 | 必填 | 说明 |
|---|---|---|
| `title` | 否 | `{tag:"plain_text"\|"markdown", content}` |
| `background_color` | 否 | 标题区背景，颜色枚举 |
| `width` | 否 | `fill` / `auto` / `auto_when_fold`(收起时自适应) |
| `vertical_align` | 否 | `top`/`center`/`bottom` |
| `icon` | 否 | 图标 `{tag, token, color, size}`（同 `div.icon`，多 `size`） |
| `icon_position` | 否 | `left` / `right` / `follow_text` |
| `icon_expanded_angle` | 否 | 展开时图标旋转角：`-180`/`-90`/`90`/`180` |

## 嵌套 / 易错点

- 内部不支持 `form`；容器最多嵌套 5 层。
- 仅支持写 JSON，搭建工具不支持。
