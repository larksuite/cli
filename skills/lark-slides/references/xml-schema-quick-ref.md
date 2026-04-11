# XML Schema 快速参考

本文档是 [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml) 的精简版摘要；如果两者不一致，以 XSD 原文为准。

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
| `width` | 是 | 演示文稿宽度，正整数 |
| `height` | 是 | 演示文稿高度，正整数 |
| `id` | 否 | 演示文稿标识 |

**子元素：** `<title>?`, `<theme>?`, `<slide>+`

## slide 元素

| 属性 | 必需 | 说明 |
|------|------|------|
| `id` | 否 | 幻灯片标识 |

**子元素：**
- `<style>?` - 页面样式，目前可放 `<fill>`
- `<data>?` - 页面元素容器，可放 `shape`、`line`、`polyline`、`img`、`table`、`icon`、`chart`、`undefined`
- `<note>?` - 演讲者备注，内部可放 `<content>`

## theme 与文本类型

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

## content 内容模型

`<content>` 可出现在 `shape`、`table/td`、`note` 中，常用属性包括：

| 属性 | 说明 |
|------|------|
| `textType` | `title` / `headline` / `sub-headline` / `body` / `caption` |
| `textAlign` | 文本对齐方式 |
| `lineSpacing` | 行间距，schema 默认 `multiple:1.5` |
| `fontSize` | 字号 |
| `fontFamily` | 字体 |
| `color` | 字体颜色 |
| `bold` / `italic` / `underline` / `strikethrough` | 文本样式 |

`<content>` 的子元素只能是：

- `<p>`
- `<ul>`
- `<ol>`

### content 示例

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

### shape

```xml
<shape type="rect" topLeftX="120" topLeftY="120" width="240" height="120">
  <fill>
    <fillColor color="rgb(100, 149, 237)"/>
  </fill>
  <border color="rgb(0, 0, 0)" width="2"/>
</shape>
```

| 属性 | 必需 | 说明 |
|------|------|------|
| `type` | 是 | 形状类型，`text` 表示文本框 |
| `topLeftX` | 是 | 左上角 X 坐标 |
| `topLeftY` | 是 | 左上角 Y 坐标 |
| `width` | 是 | 宽度 |
| `height` | 是 | 高度 |
| `rotation` | 否 | 旋转角度 |

### line

```xml
<line startX="120" startY="120" endX="420" endY="120">
  <border color="rgb(43, 47, 54)" width="2"/>
</line>
```

### img

```xml
<img src="file_token_or_url" topLeftX="80" topLeftY="120" width="320" height="180"/>
```

### icon

```xml
<icon iconType="iconpark/Base/setting.svg" topLeftX="80" topLeftY="120" width="32" height="32"/>
```

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

## 详细参考

- [slides_xml_schema_definition.xml](slides_xml_schema_definition.xml)
- [xml-format-guide.md](xml-format-guide.md)
- [examples.md](examples.md)
- [slides_demo.xml](slides_demo.xml)

## Schema 版本信息

- **版本**: 2.0.0
- **命名空间**: http://www.larkoffice.com/sml/2.0
- **发布日期**: 2025-11-03
