# 图表 `chart`

基于 VChart 的可视化图表（折线/柱/饼/词云等）。**Card 2.0**。

## 最小示例

```json
{
  "tag": "chart",
  "chart_spec": {
    "type": "line",
    "title": { "text": "趋势" },
    "data": { "values": [
      { "time": "周一", "value": 8 },
      { "time": "周二", "value": 14 }
    ] },
    "xField": "time",
    "yField": "value"
  }
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `chart` |
| `chart_spec` | 是 | Object | / | VChart 图表定义，见下 |
| `aspect_ratio` | 否 | String | 16:9(PC)/1:1(移动) | `1:1` / `2:1` / `4:3` / `16:9` |
| `color_theme` | 否 | String | brand | `brand` / `rainbow` / `complementary` / `converse` / `primary`；chart_spec 里声明了样式则此项无效 |
| `height` | 否 | String | auto | `auto`(按宽高比) 或 `[1,999]px`（设固定高则 aspect_ratio 失效） |
| `preview` | 否 | Boolean | true | 是否可独立窗口/全屏查看 |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## chart_spec 常用类型

`chart_spec` 是标准 VChart spec。核心字段：`type`、`data.values`（数据数组）、`xField`/`yField`（轴字段）、`seriesField`（分组）、`title.text`、`legends`。

| 图表 | type | 关键字段 |
|---|---|---|
| 折线 | `line` | `xField`, `yField` |
| 面积 | `area` | `xField`, `yField` |
| 柱状 | `bar` | `xField`, `yField`，分组加 `seriesField` |
| 条形（横向） | `bar` | `direction:"horizontal"`，`xField`=值，`yField`=类别 |
| 饼/环 | `pie` | `valueField`, `categoryField`，环图加 `innerRadius` |
| 散点 | `scatter` | `xField`, `yField` |
| 词云 | `wordCloud` | `nameField`, `valueField` |

完整属性参考 [VChart 官方文档](https://www.visactor.io/vchart/option/barChart)。

## 易错点

- 不支持 JavaScript 语法，`chart_spec` 必须是纯 JSON。
- 单卡建议 ≤5 个图表。
- 移动端不支持部分 VChart 属性（纹理 texture、conical 渐变、grid 词云布局等），用了会在移动端加载失败。
- 平台默认给 chart_spec 追加 media query 自适应；要自控可设 `"media": []`。
