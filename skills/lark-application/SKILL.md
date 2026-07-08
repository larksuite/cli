---
name: lark-application
version: 1.0.0
description: "飞书开放平台应用自管理：目前仅实现管理当前绑定应用的斜杠指令（Slash Command 是一种允许用户在飞书聊天框中通过输入 / 快速触发机器人服务的交互方式）——列出、创建、更新、删除。当用户提到Slash Command、发消息斜杠指令面板、斜杠指令时使用。注意区分：这不是妙搭应用（lark-apps）；也不负责指令触发后的回调消费（lark-event）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli application --help"
---

# application

## 身份权限和风险处理

开始前先读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)（认证、权限处理、`--dry-run`、exit 10 高风险确认协议）。
`--as bot` 或 `--as user`均支持。
两个权限：`application:app_slash_command:read`（列出）/ `application:app_slash_command:write`（创建/更新/删除）

## Shortcuts

| Shortcut | 说明 |
|----------|------|
| [`+slash-command-list`](references/lark-application-slash-command-list.md) | 列出当前绑定应用的全部斜杠指令；是 update/delete 定位 `command_id` 的数据来源 |
| [`+slash-command-create`](references/lark-application-slash-command-create.md) | 注册新斜杠指令；`--force` 可把撞名冲突转成幂等的 update |
| [`+slash-command-update`](references/lark-application-slash-command-update.md) | 更新已有指令的描述 / 多语言描述 / 图标，按 `--command-id` 或 `--command` 定位 |
| [`+slash-command-delete`](references/lark-application-slash-command-delete.md) | 删除斜杠指令（高危写操作，走 exit 10 确认协议） |

## 关键语义

- **`--command` 不带前导 `/`**：传 `/greet` 会被拒绝，正确写法是 `--command greet`（斜杠是隐含的，客户端展示时自动加上）。
- **客户端缓存 ~5 分钟，服务端立即生效**：写操作成功后 `+slash-command-list` 立刻能看到最新状态，但 Feishu 客户端指令面板可能仍显示旧值，这是客户端缓存导致，不代表写入失败，不要因为客户端没刷新就重试写操作。
- **改名 = 删除 + 创建，`command_id` 会变**：API 不支持直接修改 `command` 字段本身（`+slash-command-update` 只能改描述/图标）。要重命名指令，必须先 `+slash-command-delete` 旧指令，再 `+slash-command-create` 新名字，产生的是全新 `command_id`，不是原地重命名。

## 不在本 skill 范围

- 妙搭应用的创建、发布、可见范围管理 → [lark-apps](../lark-apps/SKILL.md)
- 指令触发后的消息以普通 IM 消息送达（`im.message.receive_v1` 事件），消费走 lark-event（`lark-cli event consume`） → [lark-event](../lark-event/SKILL.md)
- bot 菜单、应用版本管理、应用统计（未实现）
