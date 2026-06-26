# mail +sender-list

列出当前用户邮箱的用户级发件人白名单/黑名单。

本 skill 对应 shortcut：`lark-cli mail +sender-list`。

## 命令

```bash
# 列出白名单和黑名单
lark-cli mail +sender-list --as user --type all

# 只列出白名单
lark-cli mail +sender-list --as user --type allow --page-size 50

# 查看将要调用的接口
lark-cli mail +sender-list --as user --type block --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 邮箱地址，默认 `me`。`--as bot` 不能与 `--mailbox me` 一起使用。 |
| `--type allow\|block\|all` | 否 | 名单类型，默认 `all`。 |
| `--page-size <n>` | 否 | 分页大小，1-100，默认 50。 |
| `--page-token <token>` | 否 | 上一次返回的分页 token。 |

## 输出

输出字段包含 `items[]`、`list_type`、`total`、`has_more`、`next_page_token` 或 `next_page_tokens`。每个 `items[]` 包含：

| 字段 | 说明 |
|------|------|
| `address` | 发件人邮箱地址 |
| `timestamp` | 记录更新时间戳 |
| `list_type` | `allow` 或 `block` |
