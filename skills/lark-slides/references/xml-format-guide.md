# XML 格式指南

本文档基于 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml) 整理，说明飞书 Slides XML Schema（SML 2.0）的核心结构和常用写法。

## 基本结构

```xml
<?xml version="1.0" encoding="UTF-8"?>
<presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
  <title>演示文稿标题</title>
  <slide>
    <style>
      <fill>
        <fillColor color="rgb(245, 245, 245)"/>
      </fill>
    </style>
    <data>
      <shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
        <content textType="title">
          <p>主标题</p>
        </content>
      </shape>
    </data>
    <note>
      <content textType="body">
        <p>这是演讲者备注。</p>
      </content>
    </note>
  </slide>
</presentation>
```

## 根元素

### `<presentation>`

协议标准写法应带命名空间 `http://www.larkoffice.com/sml/2.0`；当前服务端实现可能兼容不带 `xmlns` 的输入，但不作为协议保证。

**属性：**

| 属性 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `width` | positiveInteger | 是 | 演示文稿宽度，如 `960` |
| `height` | positiveInteger | 是 | 演示文稿高度，如 `540` |
| `id` | string | 否 | 演示文稿标识 |

**子元素：**

| 元素 | 必需 | 说明 |
|------|------|------|
| `<title>` | 否 | 演示文稿标题 |
| `<theme>` | 否 | 全局主题 |
| `<slide>` | 是 | 幻灯片页面，至少 1 页，最多 100 页 |

## 主题

### `<theme>`

`<theme>` 当前包含两部分：

- `<background>`：演示文稿级背景填充
- `<textStyles>`：主题文本样式集合

`<textStyles>` 下可选子元素：

- `<title>`
- `<headline>`
- `<sub-headline>`
- `<body>`
- `<caption>`

这些元素定义的是主题默认样式，不是页面结构。常用属性：

| 属性 | 说明 |
|------|------|
| `fontFamily` | 字体 |
| `fontSize` | 字号 |
| `fontColor` | 字体颜色 |

## 幻灯片元素

### `<slide>`

单张幻灯片的结构比较严格。

**属性：**

| 属性 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string | 否 | 幻灯片标识 |

**直接子元素只有：**

| 元素 | 必需 | 说明 |
|------|------|------|
| `<style>` | 否 | 页面样式 |
| `<data>` | 否 | 页面元素容器 |
| `<note>` | 否 | 演讲者备注 |

这意味着 `<title>`、`<headline>`、`<body>`、`<caption>` 不能直接放在 `<slide>` 下。

## 文本内容模型

### `<content>`

实际页面文本通常通过 `<content>` 表达，常见位置有：

- `shape` 内部
- `table/td` 内部
- `note` 内部

**常用属性：**

| 属性 | 说明 |
|------|------|
| `textType` | `title` / `headline` / `sub-headline` / `body` / `caption` |
| `verticalAlign` | 垂直对齐 |
| `textAlign` | 水平对齐 |
| `lineSpacing` | 行间距 |
| `fontSize` | 字号 |
| `fontFamily` | 字体 |
| `color` | 字体颜色 |
| `bold` / `italic` / `underline` / `strikethrough` | 内容级样式 |
| `wrap` | 是否自动换行 |

**可包含的子元素：**

- `<p>`
- `<ul>`
- `<ol>`

### `<p>`

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
  <p>普通文本 <strong>加粗</strong> <em>斜体</em> <a href="https://example.com">链接</a></p>
  <ul>
    <li><p>列表项 1</p></li>
    <li><p>列表项 2</p></li>
  </ul>
</content>
```

## 常用页面元素

所有页面元素都放在 `<data>` 中。

### `<shape>`

`shape` 可表示普通形状，也可表示文本框。文本框推荐使用 `type="text"`。

```xml
<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
  <content textType="title">
    <p>主标题</p>
  </content>
</shape>
```

```xml
<shape type="rect" topLeftX="700" topLeftY="120" width="180" height="120">
  <fill>
    <fillColor color="rgba(100, 149, 237, 0.25)"/>
  </fill>
  <border color="rgb(100, 149, 237)" width="2"/>
</shape>
```

**属性：**

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

**可选子元素：**

- `<fill>`
- `<border>`
- `<reflection>`
- `<shadow>`
- `<content>`

### `<line>`

```xml
<line startX="100" startY="200" endX="420" endY="200">
  <border color="rgb(43, 47, 54)" width="2"/>
</line>
```

`line` 使用的是 `startX` / `startY` / `endX` / `endY`，不是 `x1` / `y1` / `x2` / `y2`。

### `<img>`

```xml
<img src="file_token_or_url" topLeftX="100" topLeftY="220" width="320" height="180"/>
```

`img` 使用 `topLeftX` / `topLeftY`，不是 `x` / `y`。

### `<icon>`

```xml
<icon iconType="iconpark/Base/setting.svg" topLeftX="440" topLeftY="220" width="32" height="32"/>
```

### `<table>`

表格结构为：

- `<table>`
- `<colgroup>` / `<tr>`
- `<tr>` 内为 `<td>`
- `<td>` 内可放 `<content>`

### `<chart>`

图表元素必须至少包含：

- `<chartPlotArea>`
- `<chartData>`

同时还可以包含：

- `<chartTitle>`
- `<chartSubTitle>`
- `<chartStyle>`
- `<chartLegend>`
- `<chartTooltip>`

如果要写图表 XML，建议直接以 XSD 为准，不要自行发明更简化的 chart DSL。

## 样式元素

### `<fill>`

```xml
<fill>
  <fillColor color="rgb(100, 149, 237)"/>
</fill>
```

### `<border>`

```xml
<border color="rgb(0, 0, 0)" width="2" dashArray="solid"/>
```

### 颜色格式

```xml
<fillColor color="rgb(255, 0, 0)"/>
<fillColor color="rgba(255, 0, 0, 0.5)"/>
<fillColor color="linear-gradient(90deg, rgb(255,0,0) 0%, rgb(0,0,255) 100%)"/>
<fillColor color="radial-gradient(circle at 50% 50%, rgb(255,0,0) 0%, rgb(0,0,255) 100%)"/>
```

## 演讲者备注

### `<note>`

```xml
<note>
  <content textType="body">
    <p>这是演讲者备注内容。</p>
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
4. 优先使用 `rgb` / `rgba` 颜色格式
5. 特殊字符按 XML 规则转义
6. 标准 16:9 页面建议使用 `width="960"` 和 `height="540"`

## 参考文档

- [xml-schema-quick-ref.md](xml-schema-quick-ref.md)
- [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)
- [examples.md](examples.md)
- [slides_demo.xml](slides_demo.xml)
