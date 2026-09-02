# 发件人白/黑名单

管理当前用户邮箱的信任发件人白名单和屏蔽发件人黑名单。写操作前必须向用户确认名单类型和条目数量。

## 查看和搜索

`list` 同时承担列表和搜索；传 `keyword` 时按发件人地址或域名前缀搜索。

```bash
# 白名单
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":20}'

# 黑名单搜索
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"example","page_size":20}'
```

输出沿用 OpenAPI JSON envelope，`data` 下包含：

- `items[]`：每项包含 `sender` 和 `create_time`
- `has_more`：是否还有下一页
- `page_token`：下一页游标

## 加入名单

```bash
# sender_type: 1=邮箱地址，2=域名
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"trusted@example.com","sender_type":1},{"sender":"example.org","sender_type":2}]}'

lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"spam@example.com","sender_type":1}]}'
```

`batch_create` 返回 `failed_items[]`。不要在 CLI 侧根据请求数量推导成功数量；如需确认最终状态，创建后再次调用 `list` 或带 `keyword` 查询。

## 删除名单

```bash
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["trusted@example.com","example.org"]}'

lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["spam@example.com"]}'
```

`batch_remove` 返回 `deleted_count`。删除不存在地址按服务端语义处理，不要额外造“未找到”。

## 注意事项

- `user_mailbox_id` 通常传 `"me"`，代表当前登录用户邮箱。
- 白名单和黑名单互斥：加入一侧时，服务端会删除另一侧相同 sender 记录。
- 读操作 scope 为 `mail:user_mailbox.message:readonly` 或 `mail:user_mailbox.message:modify`；写入和删除 scope 为 `mail:user_mailbox.message:modify`。
- 参数不确定时先运行 `lark-cli mail user_mailbox.allow_senders -h` / `lark-cli mail user_mailbox.blocked_senders -h`，再用 `lark-cli schema mail.<resource>.<method>` 查看 `--params` 与 `--data` 结构。
