# 邮箱允许/拒收发件人 Shortcut

管理当前用户或指定用户邮箱的 allow/block sender 列表。优先使用 `mail +allow-senders-*` 和 `mail +blocked-senders-*` shortcut；只有需要复现 API 原始行为时，才回退到 `mail user_mailbox.allow_senders` 或 `mail user_mailbox.blocked_senders` 原子命令。

## 常用 shortcut

```bash
# 列出允许发件人，默认邮箱为 me
lark-cli mail +allow-senders-list --as user --format json

# 搜索拒收发件人
lark-cli mail +blocked-senders-search --as user \
  --mailbox shared@example.com \
  --keyword spam \
  --page-size 50 \
  --format json

# 添加邮箱地址或域名，逗号分隔或重复 --sender 均可
lark-cli mail +allow-senders-set --as user \
  --mailbox me \
  --sender alice@example.com,example.org

# 删除拒收发件人；传入值会原样发送，便于删除历史混合大小写记录
lark-cli mail +blocked-senders-delete --as user \
  --sender Alice@Example.COM \
  --sender example.org
```

## 参数

| 参数 | 适用命令 | 默认 | 说明 |
|---|---|---|---|
| `--mailbox` | 全部 | `me` | `user_mailbox_id`，可传 `me`、邮箱地址或可访问邮箱 ID |
| `--keyword` | `*-list` / `*-search` | 空 | 按发件人邮箱或域名搜索 |
| `--page-size` | `*-list` / `*-search` | `20` | 分页大小，范围 1-100 |
| `--page-token` | `*-list` / `*-search` | 空 | 上一页返回的分页 token |
| `--sender` | `*-set` / `*-delete` / `*-add` / `*-remove` | 必填 | 邮箱地址或域名；支持逗号分隔或重复传参 |

## Scope 和身份

- 列表查询需要 `mail:user_mailbox.message:readonly`。
- 添加和删除需要 `mail:user_mailbox.message:modify`。
- shortcut 仅支持 `--as user`。需要先通过 `lark-cli auth login --domain mail` 完成用户授权。
- `--mailbox me` 表示当前用户邮箱；操作共享邮箱或其他可访问邮箱时显式传 `--mailbox <email_or_id>`。

## 行为说明

- allow 和 block 是用户邮箱级列表，不要使用租户级 allow/block sender API 代替。
- set/add 会对输入做 trim、lowercase、大小写不敏感去重，并为每项推断 `sender_type`：包含 `@` 的值按邮箱地址提交，否则按域名提交。
- delete/remove 会 trim 并按大小写不敏感去重，但保留原始大小写值提交给服务端，兼容历史混合大小写记录。
- set/add 返回的 `failed_items[]` 是后端逐项失败结果；这类部分失败不会伪装成全部成功，调用方应展示给用户。
- `*-add` 与 `*-remove` 是兼容入口；新流程优先使用 `*-set` 与 `*-delete`。

## 原生 API fallback

```bash
lark-cli mail user_mailbox.allow_senders list --as user \
  --params '{"user_mailbox_id":"me","page_size":20}'

lark-cli mail user_mailbox.allow_senders batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"alice@example.com","sender_type":1},{"sender":"example.org","sender_type":2}]}'

lark-cli mail user_mailbox.blocked_senders batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["Alice@Example.COM","example.org"]}'
```
