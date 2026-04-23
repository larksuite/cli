
# sheets +update-filter-view-condition（更新筛选条件）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-filter-view-condition`。

## 命令

```bash
lark-cli sheets +update-filter-view-condition --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>" --condition-id "E" \
  --filter-type "number" --compare-type "between" --expected '["2","10"]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--filter-view-id` | 是 | 筛选视图 ID |
| `--condition-id` | 是 | 列字母（如 `E`） |
| `--filter-type` | 否 | 筛选类型 |
| `--compare-type` | 否 | 比较运算符 |
| `--expected` | 否 | 筛选值 JSON 数组 |
