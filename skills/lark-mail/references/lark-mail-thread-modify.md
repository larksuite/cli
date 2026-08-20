# mail +thread-modify

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

如果操作对象是具体邮件 `message_id`，不是整个会话，使用 [`mail +message-modify`](./lark-mail-message-modify.md)。

本 skill 对应 shortcut `lark-cli mail +thread-modify`，内部调用：

- `POST /open-apis/mail/v1/user_mailboxes/{mailbox}/threads/batch_modify` — 按会话批量修改标签或移动文件夹

## 命令

```bash
# 给多个会话添加未读标签
lark-cli mail +thread-modify --thread-ids <thread_id1>,<thread_id2> --add-label-ids unread

# 移除星标标签
lark-cli mail +thread-modify --thread-ids <thread_id> --remove-label-ids FLAGGED

# 归档会话
lark-cli mail +thread-modify --thread-ids <thread_id> --add-folder archive

# 指定公共邮箱或共享邮箱
lark-cli mail +thread-modify --mailbox shared@example.com --thread-ids <thread_id> --add-folder folder_xxx

# Dry Run：只预览请求，不执行
lark-cli mail +thread-modify --thread-ids <thread_id> --add-label-ids custom_label_id --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 会话所属邮箱，默认 `me` |
| `--thread-ids <ids>` | 是 | 会话 ID 列表，支持逗号分隔和重复传参；超过 20 个时自动分批提交 |
| `--add-label-ids <ids>` | 否 | 要添加的标签 ID。系统标签可传 `unread` / `important` / `other` / `flagged` / `read_receipt_request` |
| `--remove-label-ids <ids>` | 否 | 要移除的标签 ID。不能与 `--add-label-ids` 传入重复标签 |
| `--add-folder <id>` | 否 | 要移动到的文件夹。系统文件夹可传 `inbox` / `sent` / `spam` / `archive` / `archived`；自定义文件夹传文件夹 ID |

`--add-label-ids`、`--remove-label-ids`、`--add-folder` 至少传一个。

`TRASH` 不允许通过本 shortcut 作为目标文件夹传入。需要软删除会话时，使用 [`mail +thread-trash`](./lark-mail-thread-trash.md)，并在用户确认后加 `--yes` 执行。

## 注意事项

- `thread_id` 必须来自 `+triage`、`+message`、`+thread`、会话列表或搜索等真实查询结果；不要用数字主键或占位符。
- 命令在本地解析逗号分隔和重复 flag，按首次出现顺序去重，并按 20 个一批提交。
- 单个 batch 请求失败时，该批次的所有 `thread_id` 都记录为同一个失败原因；后续批次继续执行。

## 返回值

返回示例：

```json
{
  "success_thread_ids": ["thread_id1"],
  "failed_thread_ids": [
    {"thread_id": "thread_id2", "reason": "api error"}
  ]
}
```

## 原生 API 适用场景

只有在需要精确复现后端/API 行为做诊断，或需要 shortcut 未暴露的请求结构时，才直接调用 `mail user_mailbox.threads batch_modify`。普通会话整理优先使用本 shortcut，因为它内置了 ID 校验、分批、批量输出和 dry-run 预览。

## 相关命令

- `lark-cli mail +triage` — 浏览邮件摘要，获取 `thread_id`
- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +message-modify` — 按 `message_id` 修改具体邮件
- `lark-cli mail +thread-trash` — 按 `thread_id` 软删除会话
