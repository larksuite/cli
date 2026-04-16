---
name: lark-feed
version: 1.0.0
description: "飞书应用消息流卡片：向指定用户的消息流发送应用卡片。当用户需要给其他用户发送消息流卡片（app feed card）时使用。需要飞书客户端 v7.6 及以上版本。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli feed --help"
---

# feed (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## Core Concepts

- **App Feed Card**: 应用消息流卡片，让应用直接在用户消息流中发送带标题和跳转链接的卡片。需要飞书客户端 v7.6+。
- **Bot-only**: 所有 feed 操作仅支持 bot 身份（`--as bot`）。

## Shortcuts（推荐优先使用）

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/lark-feed-create.md) | Create an app feed card for users; bot-only; sends a clickable card to one or more users' message feeds |

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `+create` | `im:app_feed_card:write` |
