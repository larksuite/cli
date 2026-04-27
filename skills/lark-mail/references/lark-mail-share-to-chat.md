# mail +share-to-chat

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

将邮件以卡片形式分享到飞书 IM 会话（群聊或个人对话）。内部两步完成：创建分享凭证 → 发送卡片到 IM。

**依赖 Scope：** `mail:user_mailbox.message:readonly`、`im:message`、`im:message.send_as_user`

## 命令

```bash
# 分享单封邮件到群聊（默认 receive-id-type=chat_id）
lark-cli mail +share-to-chat --message-id <邮件ID> --receive-id oc_xxx

# 分享整个会话到群聊
lark-cli mail +share-to-chat --thread-id <会话ID> --receive-id oc_xxx

# 通过邮箱分享给个人
lark-cli mail +share-to-chat --message-id <邮件ID> --receive-id user@example.com --receive-id-type email

# Dry Run
lark-cli mail +share-to-chat --message-id <邮件ID> --receive-id oc_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <id>` | 否（二选一） | 要分享的邮件 ID，与 `--thread-id` 互斥 |
| `--thread-id <id>` | 否（二选一） | 要分享的邮件会话 ID，与 `--message-id` 互斥 |
| `--receive-id <id>` | 是 | 目标接收者 ID，类型由 `--receive-id-type` 决定 |
| `--receive-id-type <type>` | 否 | 接收者 ID 类型（默认 `chat_id`）。可选：`chat_id` / `open_id` / `user_id` / `union_id` / `email` |
| `--mailbox <email>` | 否 | 邮箱地址（默认 `me`） |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 返回值

```json
{
  "ok": true,
  "data": {
    "card_id": "550e8400-e29b-41d4-a716-446655440000",
    "im_message_id": "om_dc13264520392913993dd051dba21dcf"
  }
}
```

## 典型场景

### 场景 1：用户说"帮我把这封邮件分享到项目群"

```bash
# Step 1: 搜索群聊获取 chat_id
lark-cli im +chat-search --query "项目群"
# → 获取 chat_id: oc_xxx

# Step 2: 分享邮件
lark-cli mail +share-to-chat --message-id <邮件ID> --receive-id oc_xxx
```

### 场景 2：分享整个邮件会话

```bash
lark-cli mail +share-to-chat --thread-id <会话ID> --receive-id oc_xxx
```

### 场景 3：通过邮箱分享给个人

```bash
lark-cli mail +share-to-chat --message-id <邮件ID> --receive-id alice@example.com --receive-id-type email
```

## 常见错误

| 症状 | 原因 | 解决 |
|------|------|------|
| `either --message-id or --thread-id is required` | 两个参数都未传 | 传入其中一个 |
| `--message-id and --thread-id are mutually exclusive` | 两个参数同时传 | 只传一个 |
| 403 `user not in chat` | 用户不在目标会话中 | 确认用户是群成员 |
| 404 `message not found` | 邮件 ID 无效 | 确认邮件 ID 正确 |
| 403 `permission not granted` | 缺少 `im:message` 或 `im:message.send_as_user` scope | 重新授权：`lark-cli auth login --scope "im:message,im:message.send_as_user"` |

## 相关命令

- `lark-cli im +chat-search` — 搜索群聊获取 chat_id
- `lark-cli mail +message` — 查看邮件内容
- `lark-cli mail +thread` — 查看邮件会话
