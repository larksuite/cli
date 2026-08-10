---
name: lark-whiteboard
version: 1.0.0
description: >
  飞书画板：查询和编辑飞书云文档中的画板。支持导出画板为预览图片、导出原始节点结构、使用多种格式更新画板内容，按节点增量创建、更新和删除，并支持把本地图片提交给 ParseImage 自动解析写入画板。
  当用户需要查看画板内容、导出画板图片、编辑画板时使用此 skill。不负责：飞书云文档内容编辑（lark-doc）、文档内嵌电子表格/Base（lark-sheets / lark-base）。
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli whiteboard --help"
---

> [!IMPORTANT]
> - 运行 `lark-cli --version`，确认可用，无需询问用户。
> - 运行 `npx -y @larksuite/whiteboard-cli@^0.2.13 -v`，确认可用，无需询问用户。

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理和 `--as user` / `--as bot` 的差异。**

## 快速决策

路由前必须按顺序完成以下准备：

1. 用[范围守卫](references/lark-whiteboard-workflow.md#范围守卫)判断用户要操作文档结构还是同一画布；范围不明时先澄清。
2. 先[拆分复合请求](references/lark-whiteboard-workflow.md#请求原子化)，再逐个原子操作匹配；first-match 只对单个原子操作生效。
3. 任何写操作都先读取实际 board state；用户声称画板为空只能作为线索。
4. 按 `lark-shared` 的身份选择原则和目标资源权限选择 `user` 或 `bot`：用户个人空间或以用户权限分享的资源优先 `user`，应用自有、明确授权给应用的资源或 bot-only 环境使用 `bot`。确定后在读取、请求预览、写入和验证中保持同一身份。

| 原子目标 | 入口 |
|---|---|
| 查看、导出、获取源码或原始节点，不改变画板 | `read/export` → [`+export`](references/lark-whiteboard-export.md) |
| 向已确认的空白画板写入第一批内容 | `initialize` → [创作 Workflow](references/lark-whiteboard-workflow.md#创作-workflow) |
| 向非空画板只新增内容，不修改既有内容 | `append` → [编辑 Workflow](references/lark-whiteboard-workflow.md#编辑-workflow) |
| 只修改既有内容 | `patch` → [编辑 Workflow](references/lark-whiteboard-workflow.md#编辑-workflow) |
| 只删除既有内容 | `delete` → [编辑 Workflow](references/lark-whiteboard-workflow.md#编辑-workflow) |
| 丢弃非空画板的全部旧内容并写入完整最终状态 | `replace` → [编辑 Workflow](references/lark-whiteboard-workflow.md#编辑-workflow) |
| 把一张本地图片解析成画板内容并自动写入目标画板 | `parse-image` → [`+parse-image`](references/lark-whiteboard-parse-image.md) |

输入格式、输入是否就绪、目标是否已定位，只能缩小已经选定的操作如何执行，不能成为新的主路由。当前 Shortcut 的可执行边界、确认规则和禁止 fallback 统一由 [Workflow](references/lark-whiteboard-workflow.md) 决定。

## Shortcuts

| Shortcut | 说明 |
|---|---|
| [`+export`](references/lark-whiteboard-export.md) | 导出 preview、SVG、代码或原始节点结构。 |
| [`+update`](references/lark-whiteboard-update.md) | 初始化空白画板、向非空画板追加新内容，或在明确整板替换时覆盖完整内容。 |
| [`+node-create`](references/lark-whiteboard-node-create.md) | 向已有画板追加已编译的 OpenAPI 节点。 |
| [`+node-update`](references/lark-whiteboard-node-update.md) | 按精确 node id 批量更新已有节点字段。 |
| [`+node-delete`](references/lark-whiteboard-node-delete.md) | 按已确认的 node id 删除节点；真实执行需要 `--yes`。 |
| [`+parse-image`](references/lark-whiteboard-parse-image.md) | 提交一张本地图片，由服务端自动解析并写入目标画板。 |
| [`+parse-image-result`](references/lark-whiteboard-parse-image.md#parse-image-result) | 查询或等待 ParseImage 任务结果。 |

## 不在本 skill 范围

- 文档内容编辑 → [lark-doc](../lark-doc/SKILL.md)
- 在文档中创建、插入或移动画板 block → [lark-doc-whiteboard.md](../lark-doc/references/lark-doc-whiteboard.md)
- 表格 / Base 操作 → [lark-sheets](../lark-sheets/SKILL.md) / [lark-base](../lark-base/SKILL.md)
