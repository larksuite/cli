---
name: lark-markdown
version: 1.0.0
description: "飞书 Markdown：查看、创建、上传和编辑 Markdown 文件。当用户需要创建或编辑 Markdown 文件、读取或修改时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli markdown --help"
---

# markdown (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 快速决策

- 用户要**上传、创建一个原生 `.md` 文件**，使用 `lark-cli markdown +create`
- 用户要**读取 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +fetch`
- 用户要**覆盖更新 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +overwrite`
- 用户要把本地 Markdown **导入成在线新版文档（docx）**，不要用本 skill，改用 [`lark-drive`](../lark-drive/SKILL.md) 的 `lark-cli drive +import --type docx`
- 用户要对 Markdown 文件做**rename / move / delete / 搜索 / 权限 / 评论**等云空间操作，不要留在本 skill，切到 [`lark-drive`](../lark-drive/SKILL.md)

## 核心边界

- 本 skill 处理的是 **Drive 中作为普通文件存储的 Markdown**，不是 docx 文档
- `--name` 和本地 `--file` 文件名都必须显式带 `.md` 后缀；不满足时 shortcut 会直接报错
- `--content` 支持：
  - 直接传字符串
  - `@file` 从本地文件读取内容
  - `-` 从 stdin 读取内容
- `--file` 只接受本地 `.md` 文件路径

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli markdown +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-markdown-create.md) | Create a Markdown file in Drive |
| [`+fetch`](references/lark-markdown-fetch.md) | Fetch a Markdown file from Drive |
| [`+overwrite`](references/lark-markdown-overwrite.md) | Overwrite an existing Markdown file in Drive |

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证和全局参数
- [lark-drive](../lark-drive/SKILL.md) — Drive 文件管理、导入 docx、move/delete/search 等
