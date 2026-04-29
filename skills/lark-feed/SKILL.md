---
name: lark-feed
version: 1.0.0
description: "飞书消息流（feed）：管理 Feed Card（群消息卡片）的即时提醒（时间敏感/置顶）状态。核心场景：为指定群聊的指定用户开启或关闭即时提醒。仅支持 bot 身份调用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli feed --help"
---

# feed

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**
**CRITICAL — 所有的 Shortcuts 在执行之前，务必先使用 Read 工具读取其对应的说明文档，禁止直接盲目调用命令。**
**CRITICAL — feed 域所有操作仅支持 bot 身份（`--as bot`），不支持 user 身份。**

## 核心场景

### 1. 开启/关闭即时提醒

为群消息卡片的指定用户开启或关闭即时提醒（时间敏感/置顶）状态。

**MUST 先读取 [`references/lark-feed-sensitive.md`](references/lark-feed-sensitive.md)**，然后使用 `feed +sensitive`。

**典型用法：**
```bash
# 开启
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids ou_yyy

# 关闭
lark-cli feed +sensitive --feed-card-id oc_xxx --disable --user-ids ou_yyy
```

## Shortcuts

| 命令 | 说明 | 文档 |
|------|------|------|
| `+sensitive` | 开启/关闭 Feed Card 即时提醒 | [lark-feed-sensitive.md](references/lark-feed-sensitive.md) |
