# apps +db-table-list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

列出妙搭应用数据库的所有数据表，含预估行数与占用空间。游标分页，默认每页 20 条，用 `--page-token` 翻页。

## 命令

```bash
# 列出线上环境第一页（JSON envelope）
lark-cli apps +db-table-list --app-id app_xxx

# pretty 表格视图
lark-cli apps +db-table-list --app-id app_xxx --format pretty

# dev 环境 + 自定义页大小 + 翻页
lark-cli apps +db-table-list --app-id app_xxx --env dev --page-size 50 --page-token "eyJBcHBJ..."

# 找占用最大的表
lark-cli apps +db-table-list --app-id app_xxx -q '.data.items | sort_by(-.size_bytes)[0]'
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--app-id <str>` | ✅ | — | 妙搭应用 ID |
| `--env <enum>` | ❌ | `online` | 数据库环境：`online`（线上）/ `dev`（开发） |
| `--page-size <int>` | ❌ | `20` | 每页条数 |
| `--page-token <str>` | ❌ | — | 翻页游标，取自上次响应的 `data.page_token` |

## 返回值

**成功（JSON 默认 envelope）：**

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "items": [
      { "name": "orders", "description": "订单表", "estimated_row_count": 1200, "size_bytes": 81920, "columns": [ ... ] }
    ]
  }
}
```

**成功（`--format pretty`，5 列对齐表格）：**

```
name       description  estimated_row_count  size   columns
orders     订单表        1200                 80 KB  7
customers  —            350                  24 KB  5
```

**失败：**

```json
{ "ok": false, "error": { "type": "validation", "message": "--app-id is required" } }
```

## 字段语义

- `items[].estimated_row_count` / `size_bytes`：每张表的预估行数与字节数，随列表默认返回（统计信息缺失时可能为 `null`）。
- pretty 表头：`size` 为占用空间的友好格式（KB / MB / GB），`columns` 为列数；空描述以 `—` 占位。
- `--env` 仅接受 `online` / `dev`，其他取值会被拒绝（`type: validation`，退出码 `2`）。
- 翻页：响应顶层带 `data.has_more` 与 `data.page_token`，把 `page_token` 回传下一次调用取下一页。

## 协同命令

| 场景 | 命令 |
|---|---|
| 查看某张表的结构 / DDL | `apps +db-table-schema --app-id <id> --table <name>` |
| 在表上执行 SQL | `apps +db-sql --app-id <id> --query "..."` |
| 获取 app_id | 从妙搭应用链接 `https://miaoda.feishu.cn/app/app_xxx` 提取，或由用户直接提供（详见 `../SKILL.md`） |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
