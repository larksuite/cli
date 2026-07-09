# 图片 `img`

展示图片。需先调上传图片接口拿 `img_key`。**Card 2.0**。

## 最小示例

```json
{
  "tag": "img",
  "img_key": "img_v3_xxx",
  "alt": { "tag": "plain_text", "content": "" }
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `img` |
| `img_key` | 是 | String | / | 图片 key，上传图片接口获取 |
| `alt` | 是 | Object | / | hover 说明，`{tag:"plain_text", content:""}`，不需要传空 |
| `title` | 否 | Object | / | 图片标题，plain_text 对象 |
| `scale_type` | 否 | String | crop_center | `crop_center` / `crop_top` / `fit_horizontal`（不裁剪） |
| `size` | 否 | String | / | 仅 `crop_*` 生效：`stretch`/`large`(160)/`medium`(80)/`small`(40)/`tiny`(16)，或 `"100px 100px"` |
| `corner_radius` | 否 | String | / | 圆角，`[0,∞]px` 或 `[0,100]%` |
| `transparent` | 否 | Boolean | false | 是否透明底 |
| `preview` | 否 | Boolean | true | 点击是否放大；配 `card_link` 跳转时设 false |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 易错点

- 通栏效果：2.0 不再支持 `size: stretch_without_padding`，改用负 `margin`（如 `"4px -12px"`）。
- 上传规范：≤10M、尺寸 ≤1500×3000px、高:宽 ≤16:9。
