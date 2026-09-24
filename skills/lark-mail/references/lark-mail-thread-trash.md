# mail +thread-trash

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

`mail +thread-trash` 批量软删除邮件会话。它影响整个会话中的邮件，不等同于删除单封邮件；只需操作具体邮件时使用 [`mail +message-trash`](./lark-mail-message-trash.md)。

## 示例

```bash
# 先预览单次批量请求
lark-cli mail +thread-trash --thread-id <thread_id_1>,<thread_id_2> --dry-run

# 用户确认后执行
lark-cli mail +thread-trash \
  --thread-id <thread_id_1> --thread-id <thread_id_2> \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|---|---:|---|
| `--mailbox <email>` | 否 | 会话所属邮箱，默认 `me`；bot 身份必须传显式邮箱地址 |
| `--thread-id <id>` | 是 | 会话 ID；可重复或逗号分隔，输入会 trim 并按首次出现顺序去重 |
| `--yes` | 执行时必填 | 确认高风险写操作；dry-run 不需要 |

公共参数 `--as`、`--dry-run`、`--format`、`--field` 和 `--jq` 继续由 Shortcut 框架提供。

## 行为与返回值

- 所有会话 ID 通过一次 `threads.batch_trash` 调用提交，请求体只包含 `thread_ids`。
- 成功输出直接透传底层接口的 `data`；不会根据输入数量合成 `trashed_count`、成功数或逐项结果。
- 写请求超时后不会自动重放。先查询会话状态，再决定是否重新执行。

## 相关命令

- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +thread-modify` — 修改整个会话
- `lark-cli mail +message-trash` — 软删除单封邮件
