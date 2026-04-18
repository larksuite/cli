---
name: lark-slides
version: 1.0.0
description: "飞书幻灯片：创建和编辑幻灯片，接口通过 XML 协议通信。创建演示文稿、读取幻灯片内容、管理幻灯片页面（创建、删除、读取、局部替换）。当用户需要创建或编辑幻灯片、读取或修改单个页面时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**编辑已有幻灯片页面**：优先用 [`+replace-slide`](references/lark-slides-replace-slide.md)（块级替换/插入，不动页序）；选择 action 和完整读-改-写流程见 [`lark-slides-edit-workflows.md`](references/lark-slides-edit-workflows.md)。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli slides +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-slides-create.md) | Create a Lark Slides presentation |
| [`+media-upload`](references/lark-slides-media-upload.md) | Upload a local image to a slides presentation and return the file_token (use as <img src=...>) |
| [`+replace-slide`](references/lark-slides-replace-slide.md) | Replace elements on a slide via block_replace / block_insert parts (auto-injects id + \<content/\> on shape elements) |

## API Resources

```bash
lark-cli schema slides.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli slides <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### xml_presentations

  - `get` — 读取演示文稿全文信息，XML 格式返回

### xml_presentation.slide

  - `create` — 在指定 XML 演示文稿下创建页面
  - `delete` — 在指定 XML 演示文稿下删除页面
  - `get` — 获取指定 XML 演示文稿的单个页面 XML 内容
  - `replace` — 对指定 XML 演示文稿页面进行元素级别的局部替换

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `xml_presentations.get` | `slides:presentation:read` |
| `xml_presentation.slide.create` | `slides:presentation:update` |
| `xml_presentation.slide.delete` | `slides:presentation:update` |
| `xml_presentation.slide.get` | `slides:presentation:read` |
| `xml_presentation.slide.replace` | `slides:presentation:update` |

## 参考文档

| 文档 | 说明 |
|------|------|
| [lark-slides-edit-workflows.md](references/lark-slides-edit-workflows.md) | 编辑已有页面的读-改-写流程与 action 决策树 |
| [slide-templates.md](references/slide-templates.md) | 可复制的 Slide XML 模板（封面页、内容页、数据卡片页、结尾页） |

