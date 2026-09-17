# 多图混排 `img_combination`

多张图片按预设版式拼排。**Card 2.0**。

## 最小示例

```json
{
  "tag": "img_combination",
  "combination_mode": "double",
  "img_list": [{ "img_key": "img_v3_a" }, { "img_key": "img_v3_b" }]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `img_combination` |
| `combination_mode` | 是 | String | / | `double`(≤2) / `triple`(≤3) / `bisect`(双列，≤6) / `trisect`(三列，≤9) |
| `img_list` | 是 | Array | / | 每项 `{ img_key }`，顺序即排列顺序 |
| `combination_transparent` | 否 | Boolean | false | 是否透明底 |
| `corner_radius` | 否 | String | / | 圆角，`[0,∞]px` 或 `[0,100]%` |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 易错点

- 图片数超过 mode 上限：只显示靠前的，其余丢弃；不足则留空白。
- 上传规范：≤10M、≤1500×3000px、高:宽 ≤16:9。
