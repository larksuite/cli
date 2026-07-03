# 用户级发件人白名单 / 黑名单

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

管理当前邮箱用户自己的允许发件人和屏蔽发件人名单。该能力不同于租户级 `allowed_senders` / `blocked_senders`，不要混用。

## 命令形态

CLI 使用复数资源名：

- `user_mailbox.allow_senders`
- `user_mailbox.blocked_senders`

方法名来自已发布 Meta API：

- `list`
- `batch_create`
- `batch_remove`

没有 `batch_set` / `batch_delete` 命令；不要用 Meta 单数资源名 `user_mailbox.allow_sender` 或 `user_mailbox.blocked_sender` 作为 CLI resource。

## Scope 与风险

| 命令 | Scope | 风险 |
|---|---|---|
| `user_mailbox.allow_senders list` | `mail:user_mailbox:readonly` | 只读 |
| `user_mailbox.blocked_senders list` | `mail:user_mailbox:readonly` | 只读 |
| `user_mailbox.allow_senders batch_create` | `mail:user_mailbox` | 写入，执行前必须确认 |
| `user_mailbox.allow_senders batch_remove` | `mail:user_mailbox` | 写入，执行前必须确认 |
| `user_mailbox.blocked_senders batch_create` | `mail:user_mailbox` | 写入，执行前必须确认 |
| `user_mailbox.blocked_senders batch_remove` | `mail:user_mailbox` | 写入，执行前必须确认 |

写操作会改变用户收信策略。执行前必须向用户展示名单类型、动作和发件人数量，并取得确认。

## 示例

```bash
# 列出白名单
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":20}'

# 查询黑名单
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"example.com","page_size":20}'

# 加入白名单：sender_type 1=邮箱地址，2=域名
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"a@example.com","sender_type":1}]}'

# 从黑名单删除
lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["bad@example.com"]}'
```

## Dry-run

变更前可先用 `--dry-run` 检查请求形态：

```bash
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me"}' \
  --dry-run

lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"bad@example.com","sender_type":1}]}' \
  --dry-run
```

## 参数提示

- `user_mailbox_id` 通常传 `me`，表示当前登录用户邮箱。
- `sender_type=1` 表示邮箱地址，`sender_type=2` 表示域名。
- `keyword` 只用于 `list` 搜索；空值或 `*` 表示列出。
- `page_size` 默认 20，最大 100。
