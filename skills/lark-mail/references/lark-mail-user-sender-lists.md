# 用户级发件人白名单 / 黑名单

管理当前用户自己的 allow/block sender 列表。用于让某些发件人或域名总是允许或阻止进入当前用户邮箱。

## 资源区别

| 资源 | 作用范围 | 用途 |
|------|----------|------|
| `user_mailbox.allow_senders` | 当前用户邮箱 | 列出、搜索、添加、移除个人白名单发件人 |
| `user_mailbox.blocked_senders` | 当前用户邮箱 | 列出、搜索、添加、移除个人黑名单发件人 |
| `allowed_senders` / `blocked_senders` | 租户级邮箱目录设置 | 企业管理场景；不要用来处理个人收信偏好 |

用户级资源都需要 `user_mailbox_id`，通常传 `"me"`。写操作前先展示目标名单、条目和数量并获得用户确认。

## 列出或搜索

```bash
# 白名单列表；keyword 非空时按关键词搜索
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":50,"keyword":"example.com"}'

# 黑名单列表
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":50}'
```

支持 `page_size`、`page_token` 和 `keyword`。需要遍历全部结果时可用 `--page-all`。

## 添加条目

```bash
# 添加邮箱地址到白名单
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"trusted@example.com","sender_type":1}]}'

# 添加域名到黑名单
lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"example.net","sender_type":2}]}'
```

`items` 是数组，每个元素包含发件人或域名。`sender_type` 为 `1` 表示邮箱地址，`2` 表示域名。运行前可用 schema 确认当前版本字段：

```bash
lark-cli schema mail.user_mailbox.allow_senders.batch_create
lark-cli schema mail.user_mailbox.blocked_senders.batch_create
```

## 移除条目

```bash
# 从白名单移除一个或多个条目
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["trusted@example.com","example.net"]}'

# 从黑名单移除条目
lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["spam@example.com"]}'
```

## 权限

| 操作 | Scope |
|------|-------|
| `allow_senders.list` | `mail:allowed_sender:read` |
| `allow_senders.batch_create` / `batch_remove` | `mail:allowed_sender:write` |
| `blocked_senders.list` | `mail:blocked_sender:read` |
| `blocked_senders.batch_create` / `batch_remove` | `mail:blocked_sender:write` |

缺少权限时，引导用户重新执行 `lark-cli auth login --domain mail --scope <scope>`，不要在业务命令上添加 `--scope` flag。
