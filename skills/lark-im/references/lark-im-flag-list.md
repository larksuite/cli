# im +flag-list（列出标记）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli im +flag-list`。底层 `GET /open-apis/im/v1/flags`。

## 排序规则（重要）

API 返回的数据按 `update_time` **升序**排列，即**最旧的在最前面，最新的在最后面**。当 `has_more=true` 时，不能直接取第一页的条目作为最新标记，必须翻完所有页，取最后一页的最后一个条目才是最新的。

推荐使用 `--page-all` 自动翻页获取完整列表，再用 `-q '.data.flag_items[-1]'` 取最新条目。

## 命令

```bash
# 拉第一页（默认 page-size=50）
lark-cli im +flag-list --as user

# 自动翻页获取所有标记（推荐）
lark-cli im +flag-list --as user --page-all

# 自动翻页 + 取最新的一条标记
lark-cli im +flag-list --as user --page-all -q '.data.flag_items[-1]'

# 自动翻页 + 只要 item_id 列表
lark-cli im +flag-list --as user --page-all -q '.data.flag_items[].item_id'

# 指定页大小（上限 50）
lark-cli im +flag-list --as user --page-size 30

# 手动翻页
lark-cli im +flag-list --as user --page-token <上一页返回的 page_token>

# 关闭自动补消息内容（默认开启）
lark-cli im +flag-list --as user --page-all --enrich-feed-thread=false

# 限制最大翻页数（默认 20，上限 40）
lark-cli im +flag-list --as user --page-all --page-limit 10
```

## 参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `--page-size <n>` | 50 | 范围 1-50（服务端上限 50） |
| `--page-token <token>` | 空 | 上一页返回的翻页 token；空字符串也必须带 |
| `--page-all` | false | 自动翻页获取所有页的数据并合并 |
| `--page-limit <n>` | 20 | `--page-all` 模式下最大翻页数（上限 40） |
| `--enrich-feed-thread` | true | 对 feed 层 thread 条目自动补消息内容（调 `im.messages.mget`） |

## 返回结构

返回以 `data` 为主体，字段说明如下：

| 字段 | 类型 | 说明 |
|------|------|------|
| `flag_items` | array | 当前仍然存在（未取消）的标记列表，按 `update_time` 升序 |
| `delete_flag_items` | array | 曾经取消过的标记列表，按 `update_time` 升序 |
| `messages` | array | 服务端对 `(default, message)` 的消息类型标记直接内联的消息内容 |
| `has_more` | boolean | 是否还有下一页 |
| `page_token` | string | 下一页的翻页 token |

补充：`(thread, feed)` / `(msg_thread, feed)` 条目由 shortcut 自动调 `mget` 补齐，并写回到对应条目的 `message` 字段。

## 返回样例（脱敏）

```json
{
  "data": {
    "delete_flag_items": [
      {
        "create_time": "xxx",
        "flag_type": "xxx",
        "item_id": "xxx",
        "item_type": "xxx",
        "update_time": "xxx"
      }
    ],
    "flag_items": [
      {
        "create_time": "xxx",
        "flag_type": "xxx",
        "item_id": "xxx",
        "item_type": "xxx",
        "update_time": "xxx"
      }
    ],
    "has_more": false,
    "messages": [],
    "page_token": "xxx"
  }
}
```

## 权限

- Scope：`im:feed.flag:read`（列表）+ `im:message:readonly`（补消息内容）
