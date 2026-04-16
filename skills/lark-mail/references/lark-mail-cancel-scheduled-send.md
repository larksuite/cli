# mail +cancel-scheduled-send

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

取消一封已经安排好的定时发送邮件，并把它恢复为草稿。

本 skill 对应 shortcut：`lark-cli mail +cancel-scheduled-send`。

## 命令

```bash
lark-cli mail +cancel-scheduled-send --message-id <消息ID>
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <id>` | 是 | 需要取消定时发送的消息 ID |
| `--mailbox <email>` | 否 | 目标邮箱，默认 `me` |

## 返回值

```json
{
  "ok": true,
  "data": {
    "message_id": "msg_123",
    "draft_id": "draft_123",
    "status": "canceled"
  }
}
```

## 相关命令

- `lark-cli mail +send` — 新建邮件并支持定时发送
- `lark-cli mail +reply` — 回复邮件并支持定时发送
- `lark-cli mail +reply-all` — 回复全部并支持定时发送
- `lark-cli mail +forward` — 转发邮件并支持定时发送
