# mail sender allowlist/blocklist

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

管理用户邮箱级信任发件人名单和屏蔽发件人名单。

本 skill 对应 shortcut：

- `lark-cli mail +sender-allowlist`
- `lark-cli mail +sender-blocklist`
- `lark-cli mail +sender-allowlist-modify`
- `lark-cli mail +sender-blocklist-modify`

## 查询

`--page-size` 默认 20，允许范围是 1 到 100。

搜索/列表底层按游标扫描，可能出现 `items=[]` 但 `has_more=true` 的空页；这不代表结果已结束，应继续使用返回的 `page_token` 翻页。

```bash
# 查询信任发件人
lark-cli mail +sender-allowlist --page-size 20

# 按前缀搜索信任发件人
lark-cli mail +sender-allowlist --query fixture --page-size 20

# 查询屏蔽发件人
lark-cli mail +sender-blocklist --page-size 20

# 翻页
lark-cli mail +sender-blocklist --query fixture --page-size 20 --page-token '<page_token>'
```

## 修改

```bash
# 添加邮箱地址或域名到信任名单
lark-cli mail +sender-allowlist-modify --add alice@example.com --add example.com

# 添加邮箱地址或域名到屏蔽名单
lark-cli mail +sender-blocklist-modify --add spam@example.com,bad.example

# 从信任名单删除邮箱地址或域名
lark-cli mail +sender-allowlist-modify --remove alice@example.com --remove example.com

# 从屏蔽名单删除邮箱地址或域名
lark-cli mail +sender-blocklist-modify --remove spam@example.com,bad.example
```

写操作前必须先向用户确认目标名单和发件人数量。`--add` / `--remove` 可重复，也支持逗号分隔。CLI 会自动识别邮箱地址和域名：包含 `@` 的值按邮箱地址写入，否则按域名写入。

## 参数

| 参数 | 适用 shortcut | 说明 |
|------|---------------|------|
| `--mailbox <id>` | 全部 | 用户邮箱 ID、邮箱地址或 `me`，默认 `me` |
| `--query <keyword>` | `+sender-allowlist` / `+sender-blocklist` | 前缀搜索关键词 |
| `--page-size <n>` | `+sender-allowlist` / `+sender-blocklist` | 分页大小，范围 1 到 100 |
| `--page-token <token>` | `+sender-allowlist` / `+sender-blocklist` | 下一页 token |
| `--add <sender>` | `+sender-allowlist-modify` / `+sender-blocklist-modify` | 添加邮箱地址或域名 |
| `--remove <sender>` | `+sender-allowlist-modify` / `+sender-blocklist-modify` | 删除邮箱地址或域名 |

## 返回值

查询返回 `items`、`page_token`、`has_more` 等分页信息。添加和删除返回 `failed_items`，必须原样反馈失败项。
