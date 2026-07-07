# 用户级发件人白名单 / 黑名单

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

管理当前邮箱用户自己的允许发件人和屏蔽发件人名单。该能力不同于租户级 `allowed_senders` / `blocked_senders`，不要混用。

## API 形态

该能力依赖服务端新 OpenAPI。命令只有在本地 CLI registry 已同步这些资源后才可调用；调用前必须用 `lark-cli mail -h` 和下级 `-h` 确认可用资源。

资源名：

- `user_mailbox.allow_senders`
- `user_mailbox.blocked_senders`

方法名来自已发布 Meta API：

- `list`
- `batch_create`
- `batch_remove`

没有 `batch_set` / `batch_delete` 方法；不要用 Meta 单数资源名 `user_mailbox.allow_sender` 或 `user_mailbox.blocked_sender` 作为 CLI resource。

## Scope 与风险

| API | Scope | 风险 |
|---|---|---|
| allow_senders list | `mail:user_mailbox:readonly` | 只读 |
| blocked_senders list | `mail:user_mailbox:readonly` | 只读 |
| allow_senders batch_create | `mail:user_mailbox` | 写入，执行前必须确认 |
| allow_senders batch_remove | `mail:user_mailbox` | 写入，执行前必须确认 |
| blocked_senders batch_create | `mail:user_mailbox` | 写入，执行前必须确认 |
| blocked_senders batch_remove | `mail:user_mailbox` | 写入，执行前必须确认 |

写操作会改变用户收信策略。执行前必须向用户展示名单类型、动作和发件人数量，并取得确认。

## 请求形态

读取名单时通过 params 传入 `user_mailbox_id`、`keyword`、`page_size` 和 `page_token`。

加入名单时通过 params 传入 `user_mailbox_id`，通过 data 传入 `items`；`items[].sender_type` 中 1 表示邮箱地址，2 表示域名。

删除名单时通过 params 传入 `user_mailbox_id`，通过 data 传入 `senders` 字符串数组。

变更前可先用 `--dry-run` 检查请求形态；写操作必须在执行前向用户展示名单类型、动作和发件人数量。

## 参数提示

- `user_mailbox_id` 通常传 `me`，表示当前登录用户邮箱。
- `sender_type=1` 表示邮箱地址，`sender_type=2` 表示域名。
- `keyword` 只用于 `list` 搜索；空值或 `*` 表示列出。
- `page_size` 默认 20，最大 100。
