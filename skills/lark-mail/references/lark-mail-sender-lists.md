# 用户级信任 / 屏蔽发件人名单

使用原生 Meta API 管理当前用户邮箱的信任发件人（allow list）和屏蔽发件人（block list）。这是**用户级**设置，与管理员维护的租户级 `allowed_senders` / `blocked_senders` 不是同一套数据；不要用其中一套代替另一套。

## 契约与安全约定

- 资源名以当前命令树为准：`user_mailbox.allow_senders`、`user_mailbox.blocked_senders`。两者都只支持用户身份，调用时显式传 `--as user`。
- `user_mailbox_id`、搜索关键字和分页参数放在 `--params`；批量条目放在 `--data`。首次调用前先运行对应资源的 `-h` 和 method 级 `schema`。
- `sender_type=1` 表示完整邮箱地址，`sender_type=2` 表示邮箱域名。`batch_create` 是“加入名单”，不是全量替换。
- `batch_create` / `batch_remove` 是写操作，只能按用户明确指定的名单和条目执行。当前生成命令的风险级别是 `write`，不接受 `--yes`；不要为示例添加命令树不存在的 flag。
- 批量响应只有 `failed_items`。空数组表示本批次没有逐项失败，**不代表 CLI 获得了真实新增数或删除数**；需要确认最终状态时，写入后用 `list --keyword` 回读。
- 成功输出保持标准 JSON envelope；OpenAPI 错误保持结构化 stderr envelope 和非零退出码。上游返回 trace 标识时保留在 `error.log_id`，包括搜索缓存未就绪（HTTP 456）等错误；不要把错误响应当成空列表。

## 信任发件人：加入 → 查询 → 删除

```bash
# 先确认资源、方法和输入结构
lark-cli mail user_mailbox.allow_senders -h
lark-cli schema mail.user_mailbox.allow_senders.batch_create

# 加入一个邮箱地址和一个域名
lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"trusted@example.com","sender_type":1},{"sender":"partner.example","sender_type":2}]}'

# 用 keyword 前缀搜索验证；检查 items、has_more 和 page_token
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"trusted@","page_size":20}'

# 删除刚才加入的值
lark-cli mail user_mailbox.allow_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["trusted@example.com","partner.example"]}'
```

## 屏蔽发件人：加入 → 查询 → 删除

```bash
# 先确认资源、方法和输入结构
lark-cli mail user_mailbox.blocked_senders -h
lark-cli schema mail.user_mailbox.blocked_senders.batch_create

# 加入一个邮箱地址和一个域名
lark-cli mail user_mailbox.blocked_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"spam@example.com","sender_type":1},{"sender":"noise.example","sender_type":2}]}'

# 用 keyword 前缀搜索验证
lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","keyword":"spam@","page_size":20}'

# 删除刚才加入的值
lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["spam@example.com","noise.example"]}'
```

## 分页与互斥

`list` 返回 `items`、`has_more` 和 opaque `page_token`。单页精确控制时，把上一页的 `page_token` 原样放回 `--params`；遍历全部结果时可用全局 `--page-all`：

```bash
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":100,"page_token":"<opaque_page_token>"}'

lark-cli mail user_mailbox.blocked_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":100}' \
  --page-all
```

同一个 sender 的用户级信任名单和屏蔽名单互斥：加入一侧可能移除另一侧的对应记录。若用户要求切换状态，写入目标名单后应分别查询两侧，确认目标侧存在且对侧不再命中。
