---
name: lark-apps
version: 1.0.0
description: "飞书妙搭应用（lark-cli apps）：创建、更新、列出妙搭应用，把本地 HTML 文件或目录部署为可访问的妙搭应用，设置应用可用范围（指定可见 / 互联网公开 / 企业全员）。当用户提到 妙搭, 妙搭应用, 创建应用, 发布到妙搭, 把 HTML 发到妙搭, 部署到妙搭, 应用可用范围, miaoda 等关键词时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli apps --help"
---

# apps (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 妙搭应用（apps）域介绍

妙搭是飞书的低代码 / 无代码应用平台。本域命令围绕"妙搭应用"展开：

- **App（应用）**：用户创建的妙搭应用对象，含 `app_id`、`name`、`description`、`icon_url`；通过 `+html-publish` 发布 HTML 内容
- **Access Scope（可用范围）**：`specific`（指定可见）/ `public`（互联网公开）/ `tenant`（企业全员）三选一

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli apps +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-apps-create.md) | 创建妙搭应用 |
| [`+update`](references/lark-apps-update.md) | 部分更新应用名 / 描述（只发传入字段） |
| [`+list`](references/lark-apps-list.md) | 列出当前用户的妙搭应用（cursor 分页） |
| [`+access-scope-set`](references/lark-apps-access-scope-set.md) | 设置应用可用范围（specific / public / tenant，互斥校验） |
| [`+html-publish`](references/lark-apps-html-publish.md) | 把本地 HTML 文件或目录部署为可访问的妙搭应用，返回访问链接 |
