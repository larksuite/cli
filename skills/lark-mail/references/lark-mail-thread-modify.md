# mail +thread-modify

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

`mail +thread-modify` 批量修改邮件会话的标签或所在文件夹。它影响整个会话中的邮件，不等同于修改单封邮件；只需操作具体邮件时使用 [`mail +message-modify`](./lark-mail-message-modify.md)。

## 示例

```bash
# 添加标签并移动会话
lark-cli mail +thread-modify \
  --thread-id <thread_id_1>,<thread_id_2> \
  --add-label-id <label_id> \
  --folder-id <folder_id>

# 重复 flag 与逗号分隔可以混用
lark-cli mail +thread-modify \
  --thread-id <thread_id_1> --thread-id <thread_id_2> \
  --remove-label-id <label_id>

# 只预览单次批量请求，不产生修改
lark-cli mail +thread-modify \
  --thread-id <thread_id> --add-label-id <label_id> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---:|---|
| `--mailbox <email>` | 否 | 会话所属邮箱，默认 `me`；bot 身份必须传显式邮箱地址 |
| `--thread-id <id>` | 是 | 会话 ID；可重复或逗号分隔，输入会 trim 并按首次出现顺序去重 |
| `--add-label-id <id>` | 否 | 要添加的标签 ID；可重复或逗号分隔 |
| `--remove-label-id <id>` | 否 | 要移除的标签 ID；可重复或逗号分隔，不能与新增标签重叠 |
| `--folder-id <id>` | 否 | 要移入的文件夹 ID；仅映射到底层请求的 `add_folder` 字段 |

`--add-label-id`、`--remove-label-id`、`--folder-id` 至少传一项。命令不接受任意 `--data` 请求体，也不暴露 `--add-folder` 旁路。

公共参数 `--as`、`--dry-run`、`--format`、`--field` 和 `--jq` 继续由 Shortcut 框架提供。

## 行为与返回值

- 所有会话 ID 通过一次 `threads.batch_modify` 调用提交，不拆成逐会话请求。
- 成功输出直接透传底层接口的 `data`；不会根据输入数量合成 `updated_count`、成功数或逐项结果。
- 写请求超时后不会自动重放。先查询会话状态，再决定是否重新执行。

## 相关命令

- `lark-cli mail +thread` — 读取完整会话
- `lark-cli mail +thread-trash` — 软删除整个会话
- `lark-cli mail +message-modify` — 修改单封邮件
