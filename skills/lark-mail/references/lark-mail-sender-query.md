# mail +sender-query

按关键词查询当前用户邮箱的用户级发件人白名单/黑名单。搜索缓存未就绪时，接口可能返回 456；此时按错误提示稍后重试。

本 skill 对应 shortcut：`lark-cli mail +sender-query`。

## 命令

```bash
# 查询白名单和黑名单
lark-cli mail +sender-query --as user --query example.com --type all

# 只查询黑名单，并过滤成大小写不敏感的精确地址匹配
lark-cli mail +sender-query --as user --type block --query spam@example.com --exact

# 查看将要调用的接口
lark-cli mail +sender-query --as user --query example.com --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 邮箱地址，默认 `me`。`--as bot` 不能与 `--mailbox me` 一起使用。 |
| `--type allow\|block\|all` | 否 | 名单类型，默认 `all`。 |
| `--query <keyword>` | 是 | 查询关键词，最长 255 个字符。 |
| `--page-size <n>` | 否 | 分页大小，1-100，默认 50。 |
| `--page-token <token>` | 否 | 上一次返回的分页 token。 |
| `--exact` | 否 | 仅保留与 `--query` 大小写不敏感完全相等的邮箱地址。 |

## 输出

输出形态同 `+sender-list`。`--exact` 是 CLI 侧过滤，仍会先调用服务端关键词查询。
