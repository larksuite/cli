# mail +thread-modify

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

如果操作对象是具体邮件 `message_id`，不是整个会话，使用 [`mail +message-modify`](./lark-mail-message-modify.md)。

## 命令

```bash
# 给多个会话添加未读标签
lark-cli mail +thread-modify --thread-id <thread_id1> --thread-id <thread_id2> --add-label-id unread

# 移除星标标签
lark-cli mail +thread-modify --thread-id <thread_id> --remove-label-id FLAGGED

# 归档会话
lark-cli mail +thread-modify --thread-id <thread_id> --folder-id archive

# 指定公共邮箱或共享邮箱
lark-cli mail +thread-modify --mailbox-id shared@example.com --thread-id <thread_id> --folder-id folder_xxx

# 使用 bot 身份时必须显式指定邮箱
lark-cli mail +thread-modify --as bot --mailbox-id user@example.com --thread-id <thread_id> --folder-id archive

# Dry Run：只预览请求，不执行
lark-cli mail +thread-modify --thread-id <thread_id> --add-label-id custom_label_id --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox-id <email>` | 否 | 会话所属邮箱，默认 `me`；使用 `--as bot` 时必须显式传邮箱地址 |
| `--thread-id <id>` | 是 | 会话 ID；可重复传参，CLI 按首次出现顺序去重 |
| `--add-label-id <id>` | 否 | 要添加的标签 ID；可重复传参。系统标签可传 `unread` / `important` / `other` / `flagged` |
| `--remove-label-id <id>` | 否 | 要移除的标签 ID；可重复传参，不能与 `--add-label-id` 重复 |
| `--folder-id <id>` | 否 | 要移动到的文件夹。首尾空白会被移除，并且只映射到请求字段 `add_folder` |

`--add-label-id`、`--remove-label-id`、`--folder-id` 至少传一个。`--thread-id`、`--add-label-id`、`--remove-label-id` 只公开单数可重复形式；复数拼写不受支持。`--add-folder` 和 `--mailbox` 仍作为兼容别名接受。

`TRASH` 不允许通过本 shortcut 作为目标文件夹传入。需要软删除会话时，使用 [`mail +thread-trash`](./lark-mail-thread-trash.md)，并在用户确认后加 `--yes` 执行。

`READ_RECEIPT_REQUEST` / `read_receipt_request` 不允许通过本 shortcut 添加或移除。已读回执请求必须先读取具体 message、确认用户意图，再使用 [`mail +send-receipt`](./lark-mail-send-receipt.md) 或 [`mail +decline-receipt`](./lark-mail-decline-receipt.md)。

## 注意事项

- `thread_id` 必须来自 `+triage`、`+message`、`+thread`、会话列表或搜索等真实查询结果；不要用数字主键或占位符。
- 命令在本地按首次出现顺序去重，然后一次调用 `POST /open-apis/mail/v1/user_mailboxes/<mailbox>/threads/batch_modify`。
- `--folder-id` 未传时省略 `add_folder`；标签集合为空时也省略对应字段。dry-run 只显示规范化后的请求，不访问 API。

## 返回值

成功时原样输出 OpenAPI 返回的 `data`。CLI 不根据提交的 ID 数量生成 `updated_count`、逐项成功结果或其他推断字段；空 `data` 保持为空对象。API 报错由统一错误链原样返回，修正 mailbox、ID 或权限后再重试。

## 原生 API 适用场景

只有在需要精确复现后端/API 行为做诊断，或需要 shortcut 未暴露的请求结构时，才直接调用 `mail user_mailbox.threads batch_modify`。普通会话整理优先使用本 shortcut，因为它内置了 ID 校验、稳定去重、字段安全映射和 dry-run 预览。

## 相关命令

- `lark-cli mail +triage` — 浏览邮件摘要，获取 `thread_id`
- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +message-modify` — 按 `message_id` 修改具体邮件
- `lark-cli mail +thread-trash` — 按 `thread_id` 软删除会话
