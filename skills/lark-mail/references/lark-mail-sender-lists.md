# 邮箱允许名单与阻止名单

使用原生 Mail API 管理当前用户邮箱的允许名单或阻止名单：

- 允许名单资源：`user_mailbox.allow_senders`
- 阻止名单资源：`user_mailbox.blocked_senders`
- `sender_type=1` 表示完整邮箱地址，`sender_type=2` 表示域名
- `list` 只请求 `mail:user_mailbox.message:readonly`
- `batch_create` 和 `batch_remove` 只请求 `mail:user_mailbox.message:modify`

写操作前向用户展示名单类型、目标邮箱和待新增或删除的项目；仅处理用户明确指定的项目。服务端可能在 `failed_items` 中逐项返回 `INVALID`、`SELF_ADDRESS`、`SELF_DOMAIN`、`CONFLICT_BLOCK` 或 `QUOTA_EXCEEDED`，不能只根据请求本身成功就声称每一项都已生效。

## 查看和筛选

先用 `-h` 或 method 级 schema 确认当前命令形态：

```bash
lark-cli mail user_mailbox.allow_senders -h
lark-cli schema mail.user_mailbox.allow_senders.list
```

列出允许名单，并按地址或域名筛选：

```bash
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"example.com","page_size":20}'
```

阻止名单使用相同参数：

```bash
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"spam.example.com","page_size":20}'
```

当响应中 `has_more=true` 时，把返回的 `page_token` 传给下一次 `list`，或使用 `--page-all`。

## 新增和删除

新增项目时，`items` 必须同时包含 `sender` 和 `sender_type`：

```bash
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"verify.example.com","sender_type":2}]}'

lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"blocked@example.com","sender_type":1}]}'
```

删除时传 `senders`，无需再传 `sender_type`：

```bash
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["verify.example.com"]}'

lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["blocked@example.com"]}'
```

## 可重复的闭环验证

验证不依赖预置名单数据。为避免覆盖已有项目，选一个本次验证专用且可识别的地址或域名，然后依次执行：

1. `batch_create` 新增测试项，并确认 `failed_items` 为空。
2. `list` 携带同一值作为 `keyword`，确认 `items[].sender` 中存在测试项。
3. `batch_remove` 删除测试项，并确认 `failed_items` 为空。
4. 再次 `list`，确认 `items[].sender` 中不存在测试项。

允许名单和阻止名单分别执行这四步。列表记录中的 `create_time` 以及删除响应中的 `deleted_count` 是兼容保留字段，调用方不应依赖它们判断单项成败；以 `items` 查询结果和 `failed_items` 为准。
