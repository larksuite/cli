---
name: lark-html
version: 1.0.0
description: "飞书 / Lark HTML：发布本地 HTML 及其静态资源，下载 HTML 文档的源码 ZIP。当用户需要发布 HTML 页面、更新同一路径的 HTML 文档或根据 HTML token 下载源码时使用。不负责妙搭应用（app_id，使用 lark-apps）、普通云盘文件（使用 lark-drive）或云文档内容编辑（使用 lark-doc）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli html --help"
---

# html (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## HTML 域

本域用于发布 HTML 单品文档及下载源码 ZIP，与妙搭 apps 的应用对象分开。

- 发布本地 HTML：先读 [发布指引](references/lark-html-publish.md)。
- 下载已有 HTML 的源码包：先读 [下载指引](references/lark-html-fetch.md)。
- 两个命令支持用户及 Agent Employee profile，不支持普通应用 bot 身份。沿用当前 profile 的 brand 和授权，不接受 UserID、TenantID 或单独的区域参数。
- 网页公开可见不等于有源码下载权限。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli html +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+publish`](references/lark-html-publish.md) | Publish an HTML file and its local static assets |
| [`+fetch`](references/lark-html-fetch.md) | Download an HTML document's source ZIP |
