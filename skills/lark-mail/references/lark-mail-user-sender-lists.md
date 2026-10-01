# 用户级发件人黑白名单

管理当前登录用户自己的信任发件人白名单（allow）和屏蔽发件人黑名单（block）。这不是租户管理员的 `allowed_senders` / `blocked_senders` 能力；始终使用 `--as user` 和 `user_mailbox_id=me`，不要指定其他用户。

正式 Meta 资源是两个独立资源，而不是一个合并资源：

| 名单类型 | CLI 资源 | list | set | delete |
|---|---|---|---|---|
| `allow` | `user_mailbox.allow_senders` | `list` | `batch_create`（单元素） | `batch_remove`（单元素） |
| `block` | `user_mailbox.blocked_senders` | `list` | `batch_create`（单元素） | `batch_remove`（单元素） |
| `all` | 依次读取上面两个资源 | 两次 `list` | 不适用 | 不适用 |

首次调用前先查看实际命令和 method schema：

```bash
lark-cli mail user_mailbox.allow_senders -h
lark-cli mail user_mailbox.blocked_senders -h
lark-cli schema mail.user_mailbox.allow_senders.list
lark-cli schema mail.user_mailbox.allow_senders.batch_create
lark-cli schema mail.user_mailbox.allow_senders.batch_remove
```

## 列出与续页

`list` 返回结构化 JSON，其中 `items[].sender` 是地址或域名，`has_more` 表示是否还有下一页。`page_token` 是不透明值：只原样传回，不要解析、改写或假设它是时间戳。

```bash
# allow
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":100}' --format json

# block
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":100}' --format json

# 使用上一页响应中的 page_token 继续；TOKEN 必须原样传入
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":100,"page_token":"TOKEN"}' --format json
```

需要 `all` 时分别调用 allow 和 block 两个 `list`，并在结果中保留名单类型；不要把两路 token 合并成自定义 token。也可以对单个资源使用 `--page-all` 让 CLI 自动翻页。

## 精确查询（get 语义）

正式 API 没有独立 `get` method。用两个 `list` 的 `keyword` 前缀搜索后，对返回的 `items[].sender` 做大小写不敏感的完整字符串比较。前缀命中不等于精确命中；两边均无精确项才是未配置。任何 API 错误都必须作为错误处理，不能当作未命中。

```bash
# 查询 allow 候选
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"verify-sender@example.net","page_size":100}' \
  --format json

# 查询 block 候选
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"verify-sender@example.net","page_size":100}' \
  --format json
```

自动化调用方应持续翻页直到找到完整匹配或 `has_more=false`。若两个名单都出现同一完整值，应报告冲突，不能任意选择一个。

## 设置目标状态

`sender_type=1` 表示完整邮箱地址，`sender_type=2` 表示域名。设置 allow 会移除同值 block，设置 block 会移除同值 allow；重复设置同一目标状态可安全重试。写操作前先向用户展示目标地址、名单类型并取得确认。

```bash
# set allow
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"verify-sender@example.net","sender_type":1}]}' \
  --format json

# set block
lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"verify-sender@example.net","sender_type":1}]}' \
  --format json
```

成功响应中的 `failed_items` 必须为空。若非空，按 `reason_code` 报告具体失败项；不要把 HTTP 成功误判为所有条目成功。

## 删除目标状态

删除不存在的条目按目标状态成功收敛，可安全重试。只删除调用方指定的名单类型，不要顺带删除另一侧。

```bash
# delete allow
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["verify-sender@example.net"]}' \
  --format json

# delete block
lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["verify-sender@example.net"]}' \
  --format json
```

## 可验证闭环

下面的序列使用同一组正式 CLI 命令完成写入、回读和清理；每次 `list` 都要对 `items[].sender` 做完整匹配：

1. 分别 `list --keyword verify-sender@example.net`，确认 allow/block 均未精确命中。
2. `allow_senders batch_create` 设置 allow，确认 `failed_items` 为空。
3. 再次查询，确认 allow 精确命中且 block 未命中。
4. `blocked_senders batch_create` 切换为 block，确认 `failed_items` 为空。
5. 再次查询，确认 block 精确命中且 allow 未命中。
6. `blocked_senders batch_remove` 清理，确认 `failed_items` 为空。
7. 最后再次查询，确认 allow/block 均未精确命中。

## Scope 与错误

| 操作 | 必需 scope |
|---|---|
| 两个资源的 `list` | `mail:user_mailbox.message:readonly` |
| 两个资源的 `batch_create` / `batch_remove` | `mail:user_mailbox.message:modify` |

缺 scope 时会得到 403；重新登录并授权对应 scope 后再试。非法地址、自身地址/域、名单冲突和 2000 条配额限制会通过 `failed_items[].reason_code` 暴露。缓存未就绪、网关错误和下游依赖失败由 CLI 以结构化错误写到 stderr 并返回非零退出码；不得用空数组或“未命中”掩盖这些错误。
