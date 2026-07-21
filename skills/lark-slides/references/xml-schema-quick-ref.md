# XML Schema 快速参考

本文档是 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml) 的精简版摘要，并合并了常用 XML 格式写法；如果两者不一致，以 XSD 原文为准。

## 最重要的规则

1. 协议标准写法应使用 `<presentation xmlns="http://www.larkoffice.com/sml/2.0">`；当前服务端实现可能兼容不带 `xmlns` 的输入，但不作为协议保证
2. `<presentation>` 直接子元素只有 `<title>`、`<theme>`、`<slide>`
3. `<slide>` 直接子元素只有 `<style>`、`<data>`、`<note>`
4. 页面中的文本通常通过 `<content>` 表达，而不是把 `<title>`、`<body>` 直接挂在 `<slide>` 下

## 最小可用示例

```xml
<?xml version="1.0" encoding="UTF-8"?>
<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
  <slide>
    <data>
      <shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
        <content textType="title">
          <p>标题</p>
        </content>
      </shape>
    </data>
  </slide>
</presentation>
```

## presentation 根元素

| 属性 | 必需 | 说明 |
|------|------|------|
| `width` | 是 | 演示文稿宽度，正整数，标准 16:9 页面建议使用 `960` |
| `height` | 是 | 演示文稿高度，正整数，标准 16:9 页面建议使用 `540` |
| `id` | 否 | 演示文稿标识 |

**子元素：** `<title>?`, `<theme>?`, `<slide>+`

`<slide>` 至少 1 页，最多 100 页。

## theme 与文本类型

`<theme>` 当前包含两部分：

- `<background>`：演示文稿级背景填充
- `<textStyles>`：主题文本样式集合

`<textStyles>` 下可选子元素包括 `<title>`、`<headline>`、`<sub-headline>`、`<body>`、`<caption>`。这些元素定义的是主题默认样式，不是页面结构。

常用属性：

| 属性 | 说明 |
|------|------|
| `fontFamily` | 字体 |
| `fontSize` | 字号 |
| `fontColor` | 字体颜色 |

XSD 中的 `title`、`headline`、`sub-headline`、`body`、`caption` 主要出现在：

- `<theme><textStyles>...</textStyles></theme>` 中，作为主题文本样式
- `<content textType="...">` 中，作为内容的文本类型

`textStyles` 的 schema 默认值如下：

| textType | 默认字号 |
|----------|----------|
| `title` | 54 |
| `headline` | 38 |
| `sub-headline` | 32 |
| `body` | 16 |
| `caption` | 12 |

默认字号是省略 `fontSize` 时的兜底字号，不是推荐值。字号必须显式设置 `<content>` 的 `fontSize` 属性，不要依赖 `textType` 的默认字号兜底，这些兜底值明显偏大。

## slide 元素

| 属性 | 必需 | 说明 |
|------|------|------|
| `id` | 否 | 幻灯片标识 |

**子元素：**

- `<style>?` - 页面样式，目前可放 `<fill>`
- `<data>?` - 页面元素容器，可放 `shape`、`line`、`polyline`、`img`、`table`、`icon`、`chart`、`undefined`
- `<note>?` - 演讲者备注，内部可放 `<content>`

这意味着 `<title>`、`<headline>`、`<body>`、`<caption>` 不能直接放在 `<slide>` 下。

## content 内容模型

`<content>` 可出现在 `shape`、`table/td`、`note` 中，常用属性包括：

| 属性 | 说明 |
|------|------|
| `textType` | `title` / `headline` / `sub-headline` / `body` / `caption` |
| `verticalAlign` | 垂直对齐 |
| `textAlign` | 文本对齐方式 |
| `lineSpacing` | 行间距，schema 默认 `multiple:1.5` |
| `fontSize` | 字号 |
| `fontFamily` | 字体 |
| `color` | 字体颜色 |
| `bold` / `italic` / `underline` / `strikethrough` | 内容级样式 |
| `wrap` | 是否自动换行 |
| `autoFit` | 是否自动缩排 |

注意事项：

- 字号必须显式设置 `<content>` 的 `fontSize` 属性，不要依赖 `textType` 的默认字号兜底，这些兜底值明显偏大。
- 大数字、字号大或字数多的 `<content>` 必须设置 `wrap="true" autoFit="normal-auto-fit"` 属性自动换行和缩排，避免文字溢出。
- 文字颜色必须用 `<content>` 的 `color` 属性而不是 `fontColor` 属性。
- 文字行间距必须设置 `<content>` 的 `lineSpacing="multiple:xx"` 或 `lineSpacing="fixed:xx"` 而不是 `lineSpacing="xx"`。

`<content>` 直接子元素只有：

- `<p>`
- `<ul>`
- `<ol>`

### p 段落与内联标签

`<p>` 是段落元素，可混排纯文本和内联标签：

- `<br/>`
- `<strong>`
- `<em>`
- `<u>`
- `<span>`
- `<del>`
- `<a>`
- `<shadow>`
- `<outline>`

示例：

```xml
<content textType="body" textAlign="left">
  <p>正文内容 <strong>加粗</strong> <em>斜体</em> <a href="https://example.com">链接</a></p>
  <ul>
    <li><p>列表项 1</p></li>
    <li><p>列表项 2</p></li>
  </ul>
</content>
```

## data 常用元素

所有页面元素都放在 `<data>` 中。

### shape

`shape` 可表示普通形状，也可表示文本框。文本框推荐使用 `type="text"`。

```xml
<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
  <content textType="title">
    <p>主标题</p>
  </content>
</shape>
```

```xml
<shape type="rect" topLeftX="120" topLeftY="120" width="240" height="120">
  <fill>
    <fillColor color="rgb(100, 149, 237)"/>
  </fill>
  <border color="rgb(0, 0, 0)" width="2"/>
</shape>
```
`<shape type="rect">` 只是形状不是容器，`<icon>`、`<img>`、`<shape type="text">` 和其他 `<shape>` 必须与它平级靠坐标叠放。


| 属性 | 必需 | 说明 |
|------|------|------|
| `type` | 是 | 形状类型，`text` 表示文本框 |
| `topLeftX` | 是 | 左上角 X 坐标 |
| `topLeftY` | 是 | 左上角 Y 坐标 |
| `width` | 是 | 宽度 |
| `height` | 是 | 高度 |
| `rotation` | 否 | 旋转角度 |
| `flipX` / `flipY` | 否 | 翻转 |
| `alpha` | 否 | 透明度 |

可选子元素：

- `<fill>`
- `<border>`
- `<reflection>`
- `<shadow>`
- `<content>`

### line

```xml
<line startX="120" startY="120" endX="420" endY="120">
  <border color="rgb(43, 47, 54)" width="2"/>
</line>
```

`line` 使用的是 `startX` / `startY` / `endX` / `endY`，不是 `x1` / `y1` / `x2` / `y2`。

### img

```xml
<img src="file_token_or_url" topLeftX="80" topLeftY="120" width="320" height="180"/>
```

`img` 使用 `topLeftX` / `topLeftY`，不是 `x` / `y`。

`src` 只支持：`slides +media-upload` 返回的 `file_token`，或 `@<本地路径>` 占位符（仅 `+create --slides` 自动上传并替换）。**禁止使用 http(s) 外链 URL**——飞书 slides 渲染端不会代理外链图，外链 src 在 PPT 里通常不显示。本地图片详见 [lark-slides-create.md](lark-slides-create.md#本地图片path-占位符) / [lark-slides-media-upload.md](lark-slides-media-upload.md)。

本地图片的两种姿势：

- 新建带图 PPT：`+create --slides` 里直接写 `src="@./pic.png"`，CLI 在创空白 PPT 后、加 slides 前自动上传并替换 token
- 给已有 PPT 加带图新页：先 `slides +media-upload --file ./pic.png --presentation $PID` 拿 token，再用 token 写进 `xml_presentation.slide create` 的 XML

> **注意**：`width`/`height` 是**裁剪后**的显示尺寸。比例和原图不一致时会自动裁剪（无法靠属性关闭），想避免裁剪就让 `width:height` 对齐原图比例。

### icon

```xml
<icon iconType="iconpark/Charts/chart-line.svg" topLeftX="80" topLeftY="120" width="32" height="32">
  <fill>
    <fillColor color="rgba(37, 99, 235, 1)"/>
  </fill>
</icon>
```

图标必须填充颜色并和背景有足够对比。

禁止盲猜 iconType，必须先检索 IconPark，再写 `<icon iconType="...">`。检索方式和更多规则见 [iconpark.md](iconpark.md)。


### table

表格结构为：

- `<table>` 直接子元素只有 `<colgroup>` 和 `<tr>`，`width` 和 `height` 分别表示表格的目标总宽度和总高度。
- `<colgroup>` 直接子元素只有 `<col width="...">`，width 定义列宽，默认 110。
- `<tr height="...">` 直接子元素只有 `<td>`，height 定义行高，默认 37。
- `<td>` 直接子元素只有 `<fill>`（背景）、`<content>`（文字）和边框配置（一般不用），不能嵌套 `<shape>`、`<img>`、`<icon>`。

表头默认的白底白字视觉效果极差，必须设置背景和文字颜色，需在首行每个 `<td>` 上加 `<fill>`（配合 `bold` 与对比文字色）与正文行区分。

表格里的文字默认是居中对齐，可以设置 `textAlign` 调整对齐方式。

表格宽高设置：

- 已设置的列宽和行高优先保留，未设置的列宽、行高会使用表格的目标总宽度、总高度分配剩余空间
- **必须设置 `<table>` 的 `width` 和 `height` 固定表格大小，同时设置需要保留列宽或行高的 `<col>` 的 `width` 和 `<tr>` 的 `height`，其余自动分配。**

不同字号的行高参考：

| `fontSize` | 内容行数 | 紧凑 `height` | 适中 `height` | 宽松 `height` |
|------|------|------|------|------|
| 10 | 单行 | 16 | 20 | 24 |
| 12 | 单行 | 20 | 24 | 28 |
| 10 | 双行 | 32 | 36 | 42 |
| 12 | 双行 | 36 | 42 | 48 |

示例：

```xml
<table topLeftX="80" topLeftY="140" width="520" height="52">
  <colgroup>
    <col width="160"/>
    <col width="120"/>
    <col />
  </colgroup>
  <tr height="28">
    <td>
      <fill><fillColor color="rgba(30,60,114,1)"/></fill>
      <content textType="body" fontSize="12" bold="true" color="rgba(255,255,255,1)" textAlign="center"><p>项目</p></content>
    </td>
    <td>
      <fill><fillColor color="rgba(30,60,114,1)"/></fill>
      <content textType="body" fontSize="12" bold="true" color="rgba(255,255,255,1)" textAlign="right"><p>营收</p></content>
    </td>
    <td>
      <fill><fillColor color="rgba(30,60,114,1)"/></fill>
      <content textType="body" fontSize="12" bold="true" color="rgba(255,255,255,1)" textAlign="left"><p>备注说明</p></content>
    </td>
  </tr>
  <tr>
    <td><content textType="body" fontSize="10" textAlign="center"><p>线上业务</p></content></td>
    <td><content textType="body" fontSize="10" textAlign="right"><p>195</p></content></td>
    <td><content textType="body" fontSize="10" textAlign="left"><p>同比增长 8%，主要来自新客</p></content></td>
  </tr>
</table>
```

### chart

图表语法十分复杂，必须阅读 [slides_chart_demo.xml](slides_chart_demo.xml)，直接照抄其中的柱状、条形、折线、面积、饼（环）、雷达、组合图。

`<chart>` 直接子元素必须有 `<chartPlotArea>`（绘图区）和 `<chartData>`（数据）；`<chartTitle>`、`<chartSubTitle>`、`<chartStyle>`、`<chartLegend>`、`<chartTooltip>` 可选，如果想不展示标题、副标题、图例或悬浮提示，省略相应元素标签即可。

隐藏 `<chart>` 的图例只能通过不写或删除 `<chartLegend>` 实现，`<chartLegend>` 不支持 `position="none"`。

详细用法见 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)。

## 颜色与样式

### fill

```xml
<fill>
  <fillColor color="rgb(255, 0, 0)"/>
</fill>
```

### border

```xml
<border color="rgb(43, 47, 54)" width="2" dashArray="solid"/>
```

### 颜色格式

```xml
<fillColor color="rgb(255, 0, 0)"/>
<fillColor color="rgba(255, 0, 0, 0.5)"/>
<fillColor color="linear-gradient(90deg, rgb(255,0,0) 0%, rgb(0,0,255) 100%)"/>
<fillColor color="radial-gradient(circle at 50% 50%, rgb(255,0,0) 0%, rgb(0,0,255) 100%)"/>
```

> **注意**：渐变色必须使用 `rgba()` 格式并带百分比停靠点，例如 `linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)`。使用 `rgb()` 或省略停靠点会导致服务端将其回退为白色。此规则对页面背景和 shape fill 均适用。

### 页面背景

```xml
<!-- 纯色背景 -->
<slide>
  <style>
    <fill>
      <fillColor color="rgb(245, 245, 245)"/>
    </fill>
  </style>
</slide>

<!-- 渐变背景（必须用 rgba + 百分比停靠点） -->
<slide>
  <style>
    <fill>
      <fillColor color="linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)"/>
    </fill>
  </style>
</slide>
```

## 备注示例

```xml
<note>
  <content textType="body">
    <p>这是演讲者备注。</p>
  </content>
</note>
```

## 完整示例

```xml
<?xml version="1.0" encoding="UTF-8"?>
<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
  <title>季度报告</title>
  <theme>
    <textStyles>
      <title fontFamily="思源黑体" fontSize="54" fontColor="rgba(0, 0, 0, 1)"/>
      <body fontFamily="思源黑体" fontSize="18" fontColor="rgba(43, 47, 54, 1)"/>
    </textStyles>
  </theme>
  <slide>
    <style>
      <fill>
        <fillColor color="rgb(245, 245, 245)"/>
      </fill>
    </style>
    <data>
      <shape type="text" topLeftX="80" topLeftY="72" width="760" height="100">
        <content textType="title">
          <p>2024 年第一季度报告</p>
        </content>
      </shape>
      <shape type="text" topLeftX="80" topLeftY="200" width="520" height="180">
        <content textType="body">
          <p>核心指标</p>
          <ul>
            <li><p>用户增长：+25%</p></li>
            <li><p>收入增长：+30%</p></li>
            <li><p>市场份额：15%</p></li>
          </ul>
        </content>
      </shape>
      <shape type="rect" topLeftX="660" topLeftY="180" width="180" height="140">
        <fill>
          <fillColor color="rgba(100, 149, 237, 0.25)"/>
        </fill>
        <border color="rgb(100, 149, 237)" width="2"/>
      </shape>
    </data>
    <note>
      <content textType="body">
        <p>讲到增长率时补充样本范围。</p>
      </content>
    </note>
  </slide>
</presentation>
```

## 最佳实践

1. 始终带上命名空间 `xmlns="http://www.larkoffice.com/sml/2.0"`
2. 用 `shape type="text"` + `content` 表达页面文本
3. 用 `topLeftX` / `topLeftY`、`startX` / `startY` 等 schema 中定义的属性名
4. 优先使用 `rgb` / `rgba` 颜色格式；渐变必须使用 `rgba()` 且带百分比停靠点
5. 特殊字符按 XML 规则转义
6. 标准 16:9 页面建议使用 `width="960"` 和 `height="540"`

## 详细参考

- [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)
- [slides_chart_demo.xml](slides_chart_demo.xml)

## Schema 版本信息

- **版本**: 2.0.0
- **命名空间**: http://www.larkoffice.com/sml/2.0
- **发布日期**: 2025-11-03
