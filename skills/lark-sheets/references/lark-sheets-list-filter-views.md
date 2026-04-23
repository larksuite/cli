
# sheets +list-filter-views（查询筛选视图）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +list-filter-views`。

查询工作表中的所有筛选视图，返回视图 ID、名称和范围。

## 命令

```bash
lark-cli sheets +list-filter-views --spreadsheet-token "shtxxxxxxxx" --sheet-id "<sheetId>"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |

## 输出

JSON，包含 `items[]`（filter_view_id, filter_view_name, range）。
