---
name: lark-markdown
version: 1.2.1
description: "飞书 Markdown：查看、创建、上传、编辑和比较 Markdown 文件。当用户需要创建或编辑 Markdown 文件、读取、修改、局部 patch 或比较差异时使用。不负责将 Markdown 导入为飞书在线文档，也不负责文件搜索、权限、评论、移动、删除等云空间管理操作。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli markdown --help"
---

# markdown (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 快速决策

- 身份：Markdown 文件通常属于用户云空间资源，优先使用 `--as user`。如为自动化场景，或应用已创建并持有目标文件权限，可按场景使用 `--as bot`。首次以 `user` 身份访问前执行 `lark-cli auth login`
- `markdown +create` / `+overwrite` 失败时，先判断是不是身份和权限问题：`bot` 更常见的是 app scope 或目标目录 ACL，`user` 更常见的是用户授权或用户 ACL；不要不加判断地来回切身份重试。

- 用户要**上传、创建一个原生 `.md` 文件**，使用 `lark-cli markdown +create`
- 用户要**比较原生 `.md` 文件的历史版本差异**，或比较远端 Markdown 与本地草稿，使用 `lark-cli markdown +diff`
- 用户要**读取 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +fetch`
- 用户要对 Markdown 文件做**局部文本替换 / 正则替换**，优先使用 `lark-cli markdown +patch`
- 用户要**覆盖更新 Drive 里某个 `.md` 文件内容**，使用 `lark-cli markdown +overwrite`
- 用户要先拿 Markdown 文件的历史版本号，再做比较/下载/回滚，先用 [`lark-drive`](../lark-drive/SKILL.md) 的 `lark-cli drive +version-history`
- 用户要把本地 Markdown **导入成在线新版文档（docx）**，不要用本 skill，改用 [`lark-drive`](../lark-drive/SKILL.md) 的 `lark-cli drive +import --type docx`
- 用户要对 Markdown 文件做**rename / move / delete / 搜索 / 权限 / 评论**等云空间（云盘/云存储）操作，不要留在本 skill，切到 [`lark-drive`](../lark-drive/SKILL.md)
- `markdown +create` / `+overwrite` 命中 `missing scope`、`permission denied`、`not found`、`version limit` 时，默认停止重试并按报错 hint 处理；只有 `rate limit` 或临时网络错误才做有限重试。

## 核心边界

- 本 skill 处理的是 **Drive 中作为普通文件存储的 Markdown**，不是 docx 文档
- `--name` 和本地 `--file` 文件名都必须显式带 `.md` 后缀；不满足时 shortcut 会直接报错
- `--content` 支持：
  - 直接传字符串
  - `@file` 从本地文件读取内容
  - `-` 从 stdin 读取内容
- `markdown +patch` 的内部语义是：**先完整下载 Markdown，再本地替换，再整文件覆盖上传**
- `markdown +patch` 不是服务端原子 patch；它是 CLI 侧编排出来的局部更新能力
- `markdown +patch` 当前只支持**单组** `--pattern` / `--content`
- `markdown +patch` 替换后的最终内容**不能为空**；CLI 会拒绝上传空文件，因为 Drive 不支持零字节 Markdown，且空文件通常是误操作
- `--file` 只接受本地 `.md` 文件路径

正则替换时要特别注意 `--pattern` 的转义：

```bash
# BAD: 未转义正则特殊字符，可能匹配到错误位置
lark-cli markdown +patch --file-token boxcnxxxx --regex --pattern "version (1.0)" --content "version (2.0)"

# GOOD: 显式转义括号和点号
lark-cli markdown +patch --file-token boxcnxxxx --regex --pattern "version \\(1\\.0\\)" --content "version (2.0)"
```

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli markdown +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-markdown-create.md) | Create a Markdown file in Drive |
| [`+diff`](references/lark-markdown-diff.md) | Compare two remote Markdown versions, or compare remote Markdown against a local file |
| [`+fetch`](references/lark-markdown-fetch.md) | Fetch a Markdown file from Drive |
| [`+patch`](references/lark-markdown-patch.md) | Patch a Markdown file in Drive via fetch-local-replace-overwrite |
| [`+overwrite`](references/lark-markdown-overwrite.md) | Overwrite an existing Markdown file in Drive |

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证和全局参数
- [lark-drive](../lark-drive/SKILL.md) — Drive 文件管理、导入 docx、move/delete/search 等
