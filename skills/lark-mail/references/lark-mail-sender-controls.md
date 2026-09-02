# 允许 / 屏蔽发件人管理

用户级允许与屏蔽名单由原生动态 API 命令提供，不需要也不存在业务 `+` shortcut：

- `user_mailbox.allow_senders`: `list`、`batch_create`、`batch_remove`
- `user_mailbox.blocked_senders`: `list`、`batch_create`、`batch_remove`

这两组资源只管理用户邮箱名单，不要与租户级 allow/block API 混用。示例使用
`user_mailbox_id=me`，因此必须显式指定 `--as user`。

## 授权与输出

- `list` 接受 `mail:user_mailbox.message:readonly` **或**
  `mail:user_mailbox.message:modify`。
- `batch_create` 与 `batch_remove` 必须有
  `mail:user_mailbox.message:modify`。
- scope 在 `auth login` 时申请；业务命令没有 `--scope` flag。执行完整闭环时，申请
  `modify` 即可同时满足读写操作。
- 动态命令继续使用 CLI 标准 JSON envelope。成功数据写到 stdout；非 2xx 错误沿用
  公共结构化错误输出，保留服务端 `code`、`msg` 与 `request_id`，不要二次改写 sender。

```bash
lark-cli auth login --domain mail \
  --scope 'mail:user_mailbox.message:modify'
```

首次调用前先检查命令与 schema：

```bash
lark-cli mail user_mailbox.allow_senders -h
lark-cli schema mail.user_mailbox.allow_senders.batch_create
lark-cli schema mail.user_mailbox.allow_senders.list
lark-cli schema mail.user_mailbox.allow_senders.batch_remove
```

## 白名单闭环

下面的地址不要求预先存在；第一步创建，最后一步清理。验证环境中请给地址 local-part
加本次运行的唯一后缀，避免并行任务互相覆盖。

```bash
SENDER='allow-check-<unique>@example.com'

# 1. body: items；sender_type 1=邮箱地址，2=域名
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data "{\"items\":[{\"sender\":\"${SENDER}\",\"sender_type\":1}]}"

# 2. path + query: 用 keyword 搜索刚创建的记录
lark-cli mail user_mailbox.allow_senders list --as user \
  --params "{\"user_mailbox_id\":\"me\",\"keyword\":\"${SENDER}\",\"page_size\":50}"

# 3. body: senders；无论第 2 步是否找到记录都执行清理
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data "{\"senders\":[\"${SENDER}\"]}"
```

`list` 续页时，把上一页返回的 `page_token` 放回 `--params`；`page_size` 范围为
1–100。

## 黑名单闭环

```bash
SENDER='block-check-<unique>@example.com'

lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data "{\"items\":[{\"sender\":\"${SENDER}\",\"sender_type\":1}]}"

lark-cli mail user_mailbox.blocked_senders list --as user \
  --params "{\"user_mailbox_id\":\"me\",\"keyword\":\"${SENDER}\",\"page_size\":50}"

lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data "{\"senders\":[\"${SENDER}\"]}"
```

添加某一侧时会按服务端规则清理另一侧的同值记录；不要依赖一个 sender 同时存在于
两个名单中。

## 缓存准备中的重试

带非空 `keyword` 的搜索可能返回 `15180304`，表示名单索引仍在准备。这是可重试错误：

1. 记录响应中的 `request_id`。
2. 稍后重试同一个 `list` 请求；不要无限自动重试。
3. 即使搜索暂时不可用，也要执行 `batch_remove` 清理本次闭环创建的 sender。
4. 多次失败时连同 `code`、`msg`、`request_id` 一起报告。
