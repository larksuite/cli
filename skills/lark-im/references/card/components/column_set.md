# 分栏 `column_set` + `column`

横向多列布局容器。`column_set` 装若干 `column`，每个 `column` 内再放组件。**Card 2.0**。

## 最小示例

```json
{
  "tag": "column_set",
  "flex_mode": "none",
  "columns": [
    { "tag": "column", "width": "weighted", "weight": 1,
      "elements": [{ "tag": "markdown", "content": "左列" }] },
    { "tag": "column", "width": "weighted", "weight": 1,
      "elements": [{ "tag": "markdown", "content": "右列" }] }
  ]
}
```

## column_set 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `column_set` |
| `columns` | 是 | column[] | / | 列数组，子节点只能是 `column` |
| `flex_mode` | 否 | String | none | 窄屏自适应：`none`(按比例压缩) / `stretch`(变上下堆叠) / `flow`(自动换行) / `bisect`(两等分) / `trisect`(三等分) |
| `horizontal_spacing` | 否 | String | 8px | `small`(4)/`medium`(8)/`large`(12)/`extra_large`(16) 或 `[0,99]px` |
| `horizontal_align` | 否 | String | left | `left` / `center` / `right` |
| `background_style` | 否 | String | default | `default` 或颜色枚举/RGBA（见 `../resource/colors.md`）；嵌套时上层覆盖下层 |
| `action` | 否 | Object | / | 整块点击跳转 `{ multi_url:{url,pc_url,ios_url,android_url} }` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |

## column 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `column` |
| `elements` | 否 | Element[] | / | 列内组件；**不能放 `form` 和 `table`**，可放 `column_set` |
| `width` | 否 | String | auto | 仅 `flex_mode:none` 生效：`auto` / `weighted`(配 weight) / `[16,600]px` |
| `weight` | 否 | Number | 1 | `width:weighted` 时的宽度占比，1~5 整数 |
| `vertical_align` | 否 | String | top | `top` / `center` / `bottom` |
| `direction` | 否 | String | vertical | `vertical` / `horizontal` |
| `horizontal_spacing`/`vertical_spacing` | 否 | String | 8px | 同上间距枚举或 `[0,99]px` |
| `padding` | 否 | String | 0 | 内边距 [0,99]px |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `background_style` | 否 | String | default | 同上 |
| `action` | 否 | Object | / | 点击列跳转，同 column_set.action |

## 嵌套 / 易错点

- **column_set 的直接子节点只能是 `column`**；不能 `column_set → column_set`。二级分栏要走 `column_set → column → column_set`。
- column 内可放除 `form` / `table` 外的所有组件。
- 最多嵌套 5 层，过深会压缩展示空间。
