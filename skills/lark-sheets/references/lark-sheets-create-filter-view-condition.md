
# sheets +create-filter-view-condition（创建筛选条件）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +create-filter-view-condition`。

为筛选视图的指定列创建筛选条件。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。

## 命令

```bash
# 数值筛选：E 列 < 6
lark-cli sheets +create-filter-view-condition --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>" \
  --condition-id "E" --filter-type "number" --compare-type "less" --expected '["6"]'

# 文本筛选：G 列以 a 开头
lark-cli sheets +create-filter-view-condition --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>" \
  --condition-id "G" --filter-type "text" --compare-type "beginsWith" --expected '["a"]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--filter-view-id` | 是 | 筛选视图 ID |
| `--condition-id` | 是 | 列字母（如 `E`） |
| `--filter-type` | 是 | 筛选类型：`hiddenValue`、`number`、`text`、`color` |
| `--compare-type` | 否 | 比较运算符（如 `less`、`beginsWith`、`between`） |
| `--expected` | 是 | 筛选值 JSON 数组（如 `["6"]` 或 `["2","10"]`） |

## 输出

JSON，包含 `condition`（condition_id, filter_type, compare_type, expected）。
