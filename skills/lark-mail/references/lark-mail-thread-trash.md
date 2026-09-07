# mail +thread-trash

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

已有 `thread_id` 且要按会话维度软删除邮件时，优先使用 `mail +thread-trash`。执行前必须先拿到真实 `thread_id`，并让用户确认删除预览。

如果操作对象是具体邮件 `message_id`，不是整个会话，使用 [`mail +message-trash`](./lark-mail-message-trash.md)。

## 命令

```bash
# 软删除多个会话
lark-cli mail +thread-trash --thread-id <thread_id1> --thread-id <thread_id2> --yes

# 指定公共邮箱或共享邮箱
lark-cli mail +thread-trash --mailbox-id shared@example.com --thread-id <thread_id> --yes

# 使用 bot 身份时必须显式指定邮箱
lark-cli mail +thread-trash --as bot --mailbox-id user@example.com --thread-id <thread_id> --yes

# Dry Run：只预览请求，不执行
lark-cli mail +thread-trash --thread-id <thread_id1> --thread-id <thread_id2> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox-id <email>` | 否 | 会话所属邮箱，默认 `me`；使用 `--as bot` 时必须显式传邮箱地址 |
| `--thread-id <id>` | 是 | 会话 ID；可重复传参，CLI 按首次出现顺序去重 |
| `--yes` | 执行时必填 | 高风险写操作确认。只有用户确认删除预览后才加 |

## 注意事项

- `thread_id` 必须来自 `+triage`、`+message`、`+thread`、会话列表或搜索等真实查询结果；不要用数字主键或占位符。
- 软删除属于高风险写操作。先用真实查询结果展示删除预览，包括受影响会话数量和关键邮件摘要；用户确认后再执行并加 `--yes`。
- 旧拼写 `--thread-ids` 和 `--mailbox` 仍作为兼容别名接受。
- 命令在本地按首次出现顺序去重，然后一次调用 `POST /open-apis/mail/v1/user_mailboxes/<mailbox>/threads/batch_trash`；请求体只含 `thread_ids`。
- dry-run 只显示规范化后的 endpoint 和请求体，绝不会访问 API。

## 返回值

成功时原样输出 OpenAPI 返回的 `data`。CLI 不根据提交数量生成 `trashed_count`、逐项成功结果或其他推断字段；空 `data` 保持为空对象。API 报错由统一错误链原样返回。软删除后如需恢复，使用已发布的邮件会话修改/移动能力将对象移出 `TRASH`；不要通过重复 trash 调用猜测状态。

## 原生 API 适用场景

只有在需要精确复现后端/API 行为做诊断时，才直接调用 `mail user_mailbox.threads batch_trash`。普通会话软删除优先使用本 shortcut，因为它内置了 ID 校验、分批、批量输出、dry-run 预览和 `--yes` 确认。

## 相关命令

- `lark-cli mail +triage` — 浏览邮件摘要，获取 `thread_id`
- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +message-trash` — 按 `message_id` 软删除具体邮件
- `lark-cli mail +thread-modify` — 按 `thread_id` 修改会话标签或移动文件夹
