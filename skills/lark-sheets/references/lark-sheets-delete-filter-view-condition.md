
# sheets +delete-filter-view-condition（删除筛选条件）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +delete-filter-view-condition`。

> [!CAUTION]
> 这是**破坏性写入操作** —— 删除后不可恢复。

## 命令

```bash
lark-cli sheets +delete-filter-view-condition --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>" --condition-id "E"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--filter-view-id` | 是 | 筛选视图 ID |
| `--condition-id` | 是 | 列字母（如 `E`） |
