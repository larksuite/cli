# OKR ContentBlock 富文本格式

OKR 的 Objective、KeyResult 中的 content/notes 字段使用 `ContentBlock` 富文本格式。本文档描述其结构和使用方式。

## 两种输入输出风格

OKR shortcuts 支持 `--style` 标志控制 content/notes 字段的输入输出格式：

| `--style` 值  | 说明                                                                 | 适用场景                     |
|--------------|--------------------------------------------------------------------|--------------------------|
| `simple`（默认） | 半纯文本格式 `SemiPlainContent`，简化的 JSON 结构，仅包含 text、mention、docs、images | 大多数场景，简单易用               |
| `richtext`   | 原始 `ContentBlock` 富文本格式，完整的块结构和样式信息                                | 需要精确控制@提及用户位置、包含图片/文档链接时 |

**重要**：输入时严格根据 `--style` 值验证格式，不会自动检测。输出时读操作（如 `+cycle-detail`、`+progress-get`）根据 `--style` 返回对应格式。

## ContentBlock 结构概览

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "style": {
          "list": {
            "list_type": "bullet",
            "indent_level": 0,
            "number": 1
          }
        },
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "Hello World",
              "style": {
                "bold": true,
                "strike_through": false,
                "back_color": {
                  "red": 255,
                  "green": 0,
                  "blue": 0,
                  "alpha": 1
                },
                "text_color": {
                  "red": 0,
                  "green": 255,
                  "blue": 0,
                  "alpha": 1
                },
                "link": {
                  "url": "https://example.com"
                }
              }
            }
          },
          {
            "paragraph_element_type": "docsLink",
            "docs_link": {
              "url": "https://larkoffice.com/docx/xxx",
              "title": "Lark Document"
            }
          },
          {
            "paragraph_element_type": "mention",
            "mention": {
              "user_id": "ou_xxx"
            }
          }
        ]
      }
    },
    {
      "block_element_type": "gallery",
      "gallery": {
        "images": [
          {
            "file_token": "file_xxx",
            "src": "https://...",
            "width": 800,
            "height": 600
          }
        ]
      }
    }
  ]
}
```

## 类型定义

### ContentBlock

根级别内容块。

| 字段       | 类型                      | 说明      |
|----------|-------------------------|---------|
| `blocks` | `ContentBlockElement[]` | 内容块元素数组 |

### ContentBlockElement

内容块元素，支持段落或图库。

| 字段                   | 类型                 | 说明                                         |
|----------------------|--------------------|--------------------------------------------|
| `block_element_type` | `BlockElementType` | 块类型：`paragraph` \| `gallery`               |
| `paragraph`          | `ContentParagraph` | 段落内容（当 `block_element_type="paragraph"` 时） |
| `gallery`            | `ContentGallery`   | 图库内容（当 `block_element_type="gallery"` 时）   |

### ContentParagraph

段落内容。

| 字段         | 类型                          | 说明          |
|------------|-----------------------------|-------------|
| `style`    | `ContentParagraphStyle`     | 段落样式（列表类型等） |
| `elements` | `ContentParagraphElement[]` | 段落内元素数组     |

### ContentParagraphElement

段落内元素，支持文本、文档链接、提及。

| 字段                       | 类型                     | 说明                                        |
|--------------------------|------------------------|-------------------------------------------|
| `paragraph_element_type` | `ParagraphElementType` | 元素类型：`textRun` \| `docsLink` \| `mention` |
| `text_run`               | `ContentTextRun`       | 文本内容                                      |
| `docs_link`              | `ContentDocsLink`      | 飞书文档链接                                    |
| `mention`                | `ContentMention`       | 用户提及                                      |

### ContentTextRun

文本块。

| 字段      | 类型                 | 说明   |
|---------|--------------------|------|
| `text`  | `string`           | 文本内容 |
| `style` | `ContentTextStyle` | 文本样式 |

### ContentTextStyle

文本样式。

| 字段               | 类型             | 说明    |
|------------------|----------------|-------|
| `bold`           | `boolean`      | 是否粗体  |
| `strike_through` | `boolean`      | 是否删除线 |
| `back_color`     | `ContentColor` | 背景颜色  |
| `text_color`     | `ContentColor` | 文字颜色  |
| `link`           | `ContentLink`  | 链接    |

### ContentColor

颜色。

| 字段      | 类型        | 说明           |
|---------|-----------|--------------|
| `red`   | `int32`   | 红色通道 (0-255) |
| `green` | `int32`   | 绿色通道 (0-255) |
| `blue`  | `int32`   | 蓝色通道 (0-255) |
| `alpha` | `float64` | 透明度 (0-1)    |

### ContentParagraphStyle

段落样式。

| 字段     | 类型            | 说明   |
|--------|---------------|------|
| `list` | `ContentList` | 列表样式 |

### ContentList

列表样式。

| 字段             | 类型         | 说明                                                                  |
|----------------|------------|---------------------------------------------------------------------|
| `list_type`    | `ListType` | 列表类型：`bullet` \| `number` \| `checkBox` \| `checkedBox` \| `indent` |
| `indent_level` | `int32`    | 缩进层级                                                                |
| `number`       | `int32`    | 序号（当 `list_type="number"` 时）                                        |

### ContentGallery

图片块。目前仅有进展记录中的富文本支持展示图片。

由于 OKR 应用中进展页面的布局排版限制，一个 ContentGallery 元素中**仅可放置一个图片元素**，需要插入多张图片时需使用多个 ContentGallery 元素
(同一个 ContentGallery 中添加多个 image 会导致这些图片在狭窄的横向排版空间中互相挤占，效果很差)

| 字段       | 类型                   | 说明    |
|----------|----------------------|-------|
| `images` | `ContentImageItem[]` | 图片项数组 |

### ContentImageItem

图片项。

| 字段           | 类型        | 说明       |
|--------------|-----------|----------|
| `file_token` | `string`  | 文件 token |
| `src`        | `string`  | 图片 URL   |
| `width`      | `float64` | 宽度       |
| `height`     | `float64` | 高度       |

> **如何获取 `file_token`？** 使用 [`+upload-image`](lark-okr-image-upload.md) 命令上传本地图片，返回的 `file_token` 可用于构建 `ContentGallery` 图片块。

### ContentDocsLink

飞书文档链接。

| 字段      | 类型       | 说明     |
|---------|----------|--------|
| `url`   | `string` | 链接 URL |
| `title` | `string` | 链接标题   |

### ContentMention

提及。

| 字段        | 类型       | 说明    |
|-----------|----------|-------|
| `user_id` | `string` | 用户 ID |

### ContentLink

链接。

| 字段    | 类型       | 说明     |
|-------|----------|--------|
| `url` | `string` | 链接 URL |

## SemiPlainContent 半纯文本格式

`SemiPlainContent` 是 `ContentBlock` 的简化、有损表示形式，适用于大多数不需要复杂格式的场景。

### 结构

```json
{
  "text": "任务一 @{ou_zhangsan} ，任务二 @{ou_lisi} ",
  "mention": ["ou_zhangsan", "ou_lisi"],
  "docs": [
    {
      "title": "产品需求文档",
      "url": "https://larkoffice.com/docx/xxx"
    }
  ],
  "images": [
    "https://example.com/image.png"
  ]
}
```

### 类型定义

| 字段        | 类型               | 说明                                                                                                        |
|-----------|------------------|-----------------------------------------------------------------------------------------------------------|
| `text`    | `string`         | 纯文本内容（必填，不能为空）。**输出时**包含 ` @{userID} ` 占位符以保留提及的位置上下文；**输入时** `@{...}` 占位符会被自动 strip 掉，只识别 `mention` 字段内容 |
| `mention` | `string[]`       | 用户 ID 列表（可选），与 text 中的 `@{userID}` 占位符一一对应，输入时按顺序转换为 mention 元素**置于文本末尾**                                 |
| `docs`    | `SemiPlainDoc[]` | 文档列表（仅输出时包含，输入时 simple 风格不支持）                                                                             |
| `images`  | `string[]`       | 图片 URL 列表（仅输出时包含，输入时 simple 风格不支持）                                                                        |

### SemiPlainDoc

| 字段      | 类型       | 说明     |
|---------|----------|--------|
| `title` | `string` | 文档标题   |
| `url`   | `string` | 文档 URL |

### 双向转换说明

- **ContentBlock → SemiPlainContent**（输出时）：提取纯文本、提及用户、文档链接和图片 URL，丢弃格式信息（粗体、列表、颜色等）。**提及的位置信息通过 ` @{userID} ` 占位符保留在 text 中**，同时 userID 也会被收集到 mention 数组中
- **SemiPlainContent → ContentBlock**（输入时）：自动 strip 掉 text 中的 `@{...}` 占位符，然后将 text 和 mention 合并为单个段落，mention 按顺序附加在文本末尾。docs 和 images 在输入时被忽略（simple 风格不支持）

## 使用示例

### 示例 0：--style simple 半纯文本格式

```json
{
  "text": "提升用户满意度",
  "mention": ["ou_123"]
}
```

使用方式：
```bash
lark-cli okr +patch --level objective --style simple --target-id 123 --content '{"text":"提升用户满意度","mention":["ou_123"]}'
```

### 示例 1：简单文本段落（richtext 风格）

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "提升用户满意度"
            }
          }
        ]
      }
    }
  ]
}
```

### 示例 2：带格式的文本段落

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "Q2 目标",
              "style": {
                "bold": true
              }
            }
          },
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": " - 提升产品质量"
            }
          }
        ]
      }
    }
  ]
}
```

### 示例 3：带列表的段落

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "style": {
          "list": {
            "list_type": "bullet",
            "indent_level": 0
          }
        },
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "完成功能开发"
            }
          }
        ]
      }
    },
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "style": {
          "list": {
            "list_type": "bullet",
            "indent_level": 0
          }
        },
        "elements": [
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": "进行用户测试"
            }
          }
        ]
      }
    }
  ]
}
```

### 示例 4：带用户提及和图片（仅进展记录支持）的段落

```json
{
  "blocks": [
    {
      "block_element_type": "paragraph",
      "paragraph": {
        "elements": [
          {
            "paragraph_element_type": "mention",
            "mention": {
              "user_id": "ou_example_user"
            }
          },
          {
            "paragraph_element_type": "textRun",
            "text_run": {
              "text": " 请关注此进度并查看以下图片"
            }
          }
        ]
      }
    },
    {
      "block_element_type": "gallery",
      "gallery": {
        "images": [
          {
            "file_token": "img_example_token",
            "src": "https://example.com/image.png",
            "width": 800,
            "height": 600
          }
        ]
      }
    }
  ]
}
```
