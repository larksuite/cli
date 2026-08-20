# mail +thread-trash

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

已有 `thread_id` 且要按会话维度软删除邮件时，优先使用 `mail +thread-trash`。执行前必须先拿到真实 `thread_id`，并让用户确认删除预览。

如果操作对象是具体邮件 `message_id`，不是整个会话，使用 [`mail +message-trash`](./lark-mail-message-trash.md)。

## 命令

```bash
# 软删除多个会话
lark-cli mail +thread-trash --thread-ids <thread_id1>,<thread_id2> --yes

# 指定公共邮箱或共享邮箱
lark-cli mail +thread-trash --mailbox shared@example.com --thread-ids <thread_id> --yes

# Dry Run：只预览请求，不执行
lark-cli mail +thread-trash --thread-ids <thread_id1> --thread-ids <thread_id2> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 会话所属邮箱，默认 `me` |
| `--thread-ids <ids>` | 是 | 会话 ID 列表，支持逗号分隔和重复传参；超过 20 个时自动分批提交 |
| `--yes` | 执行时必填 | 高风险写操作确认。只有用户确认删除预览后才加 |

## 注意事项

- `thread_id` 必须来自 `+triage`、`+message`、`+thread`、会话列表或搜索等真实查询结果；不要用数字主键或占位符。
- 软删除属于高风险写操作。先用真实查询结果展示删除预览，包括受影响会话数量和关键邮件摘要；用户确认后再执行并加 `--yes`。
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

只有在需要精确复现后端/API 行为做诊断时，才直接调用 `mail user_mailbox.threads batch_trash`。普通会话软删除优先使用本 shortcut，因为它内置了 ID 校验、分批、批量输出、dry-run 预览和 `--yes` 确认。

## 相关命令

- `lark-cli mail +triage` — 浏览邮件摘要，获取 `thread_id`
- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +message-trash` — 按 `message_id` 软删除具体邮件
- `lark-cli mail +thread-modify` — 按 `thread_id` 修改会话标签或移动文件夹
