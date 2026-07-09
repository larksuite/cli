# 表格 `table`

多列数据表，支持文本/数字/选项/人员/日期等列类型。**Card 2.0**。

## 最小示例

```json
{
  "tag": "table",
  "columns": [
    { "name": "city", "display_name": "城市", "data_type": "text" },
    { "name": "qty", "display_name": "数量", "data_type": "number" }
  ],
  "rows": [
    { "city": "北京", "qty": 12 },
    { "city": "上海", "qty": 8 }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `table` |
| `columns` | 是 | column[] | / | 列定义，≤50 列，见下 |
| `rows` | 是 | Object[] | / | 行数据，按 `列name: 值` 填充 |
| `page_size` | 否 | Number | 5 | 每页行数，[1,10] |
| `row_height` | 否 | String | low | `low`/`middle`/`high`/`auto`/`[32,124]px` |
| `row_max_height` | 否 | String | 124px | `row_height:auto` 时最大行高 [32,999]px |
| `freeze_first_column` | 否 | Boolean | false | 冻结首列 |
| `header_style` | 否 | Object | / | 表头样式：`{text_align, text_size, background_style:grey\|none, text_color, bold, lines}` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

**column 字段**：`name`(必填，键名) / `display_name`(表头名) / `data_type`(见下) / `width`(`auto`/`[80,600]px`/`%`) / `horizontal_align` / `vertical_align`；`number` 列可加 `format:{precision, symbol, separator}`；`date` 列可加 `date_format`(如 `YYYY/MM/DD`)。

**data_type 与行值结构**：

| data_type | 行值 |
|---|---|
| `text` | `"飞书"` |
| `lark_md` | `"[链接](https://x)"` |
| `number` | `168.23` |
| `options` | `[{text:"S2", color:"blue"}]`（颜色枚举见 `../resource/colors.md`，文本勿过长） |
| `persons` | `"ou_xxx"` 或 `["ou_a","ou_b"]` |
| `date` | `1699341315000`（毫秒时间戳，按本地时区显示） |
| `markdown` | `"![img](img_key)"` 完整 Markdown |

## 嵌套 / 易错点

- **table 只能放卡片根 `body.elements`**：不能被任何容器嵌套，自身也不能嵌别的组件。
- 单卡最多 5 个 table（多语言每语言 5 个）。
- `rows` 的键必须与 `columns[].name` 对应。
