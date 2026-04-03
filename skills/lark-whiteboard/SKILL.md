---
name: lark-whiteboard
version: 1.0.0
description: "飞书画板：查询和编辑飞书云文档中的画板。支持导出画板为预览图片、导出原始节点结构、使用 PlantUML/Mermaid 代码或 OpenAPI 原生格式更新画板内容。当用户需要查看画板内容、导出画板图片、或编辑画板时使用。"
metadata:
  requires:
    bins: [ "lark-cli" ]
  cliHelp: "lark-cli whiteboard --help"
---

# whiteboard (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 核心概念

### 画板 Token

画板 token 是画板的唯一标识符。飞书画板嵌入在云文档中，可以从云文档的 `docs +fetch` 结果中获取（`<whiteboard token="xxx"/>`
标签），或从 `docs +update` 新建画板后的 `data.board_tokens` 字段中获取。

## 快速决策

| 用户需求                        | 推荐 Shortcut                                                                                                                                              |
|-----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| "查看这个画板的内容"                 | [`+query --output_as image`](references/lark-whiteboard-query.md)                                                                                        |
| "导出画板为图片"                   | [`+query --output_as image`](references/lark-whiteboard-query.md)                                                                                        |
| "获取画板的 PlantUML/Mermaid 代码" | [`+query --output_as code`](references/lark-whiteboard-query.md)                                                                                         |
| "修改画板某个节点的颜色或文字"            | [`+query --output_as raw`](references/lark-whiteboard-query.md) 后 [`+update`](references/lark-whiteboard-update.md)                                      |
| "用 PlantUML 绘制画板"           | [`+update --input_format plantuml`](references/lark-whiteboard-update.md)                                                                                |
| "用 Mermaid 绘制画板"            | [`+update --input_format mermaid`](references/lark-whiteboard-update.md)                                                                                 |
| "在画板绘制复杂图表"                 | [`+update --input_format raw`](references/lark-whiteboard-update.md), 需要使用 whiteboard-cli 工具，参见 [lark-whiteboard-draw](../lark-whiteboard-draw/SKILL.md) |

## Shortcuts

| Shortcut                                          | 说明                                          |
|---------------------------------------------------|---------------------------------------------|
| [`+query`](references/lark-whiteboard-query.md)   | 查询画板，导出为预览图片、代码或原始节点结构                      |
| [`+update`](references/lark-whiteboard-update.md) | 更新画板内容，支持 PlantUML、Mermaid 或 OpenAPI 原生格式输入 |

## 与 lark-doc 的配合使用

### 场景 1：从文档中获取画板 token

1. 使用 `lark-doc` 的 [`+fetch`](../lark-doc/references/lark-doc-fetch.md) 获取文档内容
2. 从返回的 markdown 中解析 `<whiteboard token="xxx"/>` 标签，记录画板 token
3. 使用本 skill 的 `+query` 或 `+update` 读取或操作画板

### 场景 2：新建画板并编辑

1. 使用 `lark-doc` 的 [`+update`](../lark-doc/references/lark-doc-update.md) 创建空白画板（在 markdown 中传入
   `<whiteboard type="blank"></whiteboard>`）
2. 从响应的 `data.board_tokens` 中获取新建画板的 token
3. 根据用户需求，设计相应的画板内容代码(使用 PlantUML、Mermaid 或 [lark-whiteboard-draw](../lark-whiteboard-draw/SKILL.md)
   生成 OpenAPI 原生格式代码)
4. 使用本 skill 的 `+update` shortcut 编辑画板内容
