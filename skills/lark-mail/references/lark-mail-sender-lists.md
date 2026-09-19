# mail sender lists

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

使用 sender-list shortcuts 管理当前用户邮箱的允许发件人和阻止发件人名单。所有命令仅支持 user 身份，并分别使用 `mail:user_mailbox.message:readonly` 或 `mail:user_mailbox.message:modify` scope。

## 查询

```bash
# 列出允许发件人
lark-cli mail +sender-list --kind allow

# 列出阻止发件人并翻页
lark-cli mail +sender-list --kind block --page-size 100 --page-token <token>

# 通过服务端 keyword 查询地址或域名
lark-cli mail +sender-search --kind allow --query example.com
```

`--kind` 只能是 `allow` 或 `block`。列表响应保留服务端的 `items`、`has_more` 和 `page_token`，并补充当前页 `total`。

## 添加

```bash
# 添加一个邮件地址
lark-cli mail +sender-set --kind allow --senders newsletter@example.com

# 添加多个地址或域名；可逗号分隔或重复传参
lark-cli mail +sender-set --kind block \
  --senders sender@example.com,example.net \
  --senders another@example.com

# 只预览请求
lark-cli mail +sender-set --kind allow --senders example.com --dry-run
```

CLI 会按首次出现顺序去重。包含 `@` 的值按地址提交，其余值按域名提交。接口可能通过 `failed_items` 返回逐项失败原因，调用方应检查该字段。

## 删除

删除会改变邮箱的反垃圾行为，执行前必须展示名单类型、具体 sender 和受影响数量，并取得用户确认。

```bash
# 先预览
lark-cli mail +sender-delete --kind block --senders sender@example.com --dry-run

# 用户确认后执行
lark-cli mail +sender-delete --kind block --senders sender@example.com --yes
```

## 参数

| 参数 | 命令 | 说明 |
|------|------|------|
| `--mailbox` | 全部 | 邮箱地址，默认 `me` |
| `--kind` | 全部 | `allow` 或 `block` |
| `--page-size` | list/search | 当前页数量，1–100 |
| `--page-token` | list/search | 上一页返回的分页 token |
| `--query` | search | 服务端 keyword 查询，不能为空 |
| `--senders` | set/delete | 地址或域名列表，支持逗号分隔和重复传参 |
| `--yes` | delete | 用户确认后执行删除 |

读取操作需要 `mail:user_mailbox.message:readonly`；添加和删除需要 `mail:user_mailbox.message:modify`。
