
# sheets +list-filter-view-conditions（查询筛选条件）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +list-filter-view-conditions`。

查询筛选视图的所有筛选条件。

## 命令

```bash
lark-cli sheets +list-filter-view-conditions --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--filter-view-id` | 是 | 筛选视图 ID |

## 输出

JSON，包含 `items[]`（condition_id, filter_type, compare_type, expected）。
