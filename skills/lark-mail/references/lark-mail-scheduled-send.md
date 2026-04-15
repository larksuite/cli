# Scheduled Send

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

定时发送邮件功能，支持在 `+send`、`+reply`、`+reply-all`、`+forward` 中使用 `--send-time` 参数安排定时发送，以及通过 `+cancel-scheduled-send` 取消已安排的定时发送。

## 定时发送

在任意发信 Shortcut 中添加 `--send-time` 参数（Unix 时间戳，秒），配合 `--confirm-send` 使用：

```bash
# 定时发送新邮件
lark-cli mail +send --to alice@example.com --subject '周报' \
  --body '<p>本周进展</p>' \
  --send-time 1744610400 --confirm-send

# 定时回复
lark-cli mail +reply --message-id msg_xxx --body '<p>收到</p>' \
  --send-time 1744610400 --confirm-send

# 定时回复全部
lark-cli mail +reply-all --message-id msg_xxx --body '<p>同意</p>' \
  --send-time 1744610400 --confirm-send

# 定时转发
lark-cli mail +forward --message-id msg_xxx --to bob@example.com \
  --send-time 1744610400 --confirm-send
```

### 参数说明

| 参数 | 类型 | 说明 |
|------|------|------|
| `--send-time` | string | Unix 时间戳（秒）。必须至少在当前时间 5 分钟之后。需配合 `--confirm-send` 使用。 |

### 注意事项

- `--send-time` 必须与 `--confirm-send` 一起使用
- 时间必须至少在当前时间 5 分钟之后
- 不带 `--confirm-send` 时，`--send-time` 无效（仅创建草稿）

## 取消定时发送

```bash
# 取消定时发送（邮件还原为草稿）
lark-cli mail +cancel-scheduled-send --message-id msg_sched_123

# 指定邮箱
lark-cli mail +cancel-scheduled-send --message-id msg_sched_123 --user-mailbox-id mailbox_abc
```

### 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--message-id` | string | 是 | 定时发送邮件的 Message ID |
| `--user-mailbox-id` | string | 否 | 用户邮箱 ID（默认: me） |

### 输出

```json
{
  "message_id": "msg_sched_123",
  "status": "cancelled",
  "restored_as_draft": true
}
```

## AI Agent 工作流

1. 用户请求定时发送（如"明天 9 点发邮件给 Alice"）
2. Agent 将自然语言时间转换为 Unix 时间戳
3. 先创建草稿（不带 `--confirm-send`），展示给用户确认
4. 用户确认后，通过 L2 API 发送草稿并附带 `--send-time`：
   ```bash
   lark-cli mail user_mailbox.drafts send \
     --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}' \
     --data '{"send_time":"<unix_timestamp>"}'
   ```
