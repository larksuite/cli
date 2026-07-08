# user mailbox sender lists

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

用户级发件人名单分为两类资源：

| 名单 | 资源 | 说明 |
|---|---|---|
| 信任发件人白名单 | `user_mailbox.allow_senders` | 加入后该发件人地址或域名被当前用户邮箱信任 |
| 屏蔽发件人黑名单 | `user_mailbox.blocked_senders` | 加入后该发件人地址或域名被当前用户邮箱屏蔽 |

这些是原生 Meta API 命令，不是 `+` shortcut。参数不确定时先运行 `-h` 和 schema，不要猜测字段名：

```bash
lark-cli mail user_mailbox.allow_senders -h
lark-cli mail user_mailbox.blocked_senders -h
lark-cli schema mail.user_mailbox.allow_senders.batch_create
```

命令名以 `-h` 为准：使用 `allow_senders` / `blocked_senders` 和 `batch_create` / `batch_remove`，不要使用旧方案里的 `sender_lists`、`batch_set` 或 `batch_delete`。

## 权限与确认

- `list` 需要 `mail:user_mailbox.message:readonly`。
- `batch_create` / `batch_remove` 需要 `mail:user_mailbox.message:modify`，属于写操作。执行前必须向用户展示名单类型、发件人列表和数量并取得确认。
- `user_mailbox_id` 通常传 `"me"`。使用 tenant access token 时必须显式传邮箱地址或 `open_id`，不要传 `"me"`。
- 添加接口单次最多 100 项；单用户黑白名单合计最多 2000 项。白名单和黑名单互斥，加入一侧会移除另一侧对应记录。

## 列出或搜索名单

```bash
# 列出白名单
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":20}'

# 搜索黑名单中的地址或域名前缀
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"example.com","page_size":20}'
```

返回字段包括 `items[].sender`、`items[].create_time`、`has_more`、`page_token`。需要继续翻页时把上一次返回的 `page_token` 放回 `--params`。

## 加入白名单或黑名单

```bash
# 加入白名单：sender_type=1 表示完整邮箱地址，sender_type=2 表示域名
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"trusted@example.com","sender_type":1},{"sender":"example.org","sender_type":2}]}'

# 加入黑名单
lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"spam@example.com","sender_type":1}]}'
```

响应中的 `failed_items` 为空表示全部成功；非空时逐项查看 `reason_code`，常见值包括 `INVALID`、`SELF_ADDRESS`、`SELF_DOMAIN`、`CONFLICT_BLOCK`、`QUOTA_EXCEEDED`。

## 删除名单条目

```bash
# 从白名单删除
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["trusted@example.com","example.org"]}'

# 从黑名单删除
lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["spam@example.com"]}'
```

响应中的 `deleted_count` 可能小于请求的 `senders` 数量；不存在的条目会被静默跳过。
