
# sheets +update-filter-view（更新筛选视图）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-filter-view`。

## 命令

```bash
lark-cli sheets +update-filter-view --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --filter-view-id "<fvId>" --range "<sheetId>!A1:J20"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--filter-view-id` | 是 | 筛选视图 ID |
| `--range` | 否 | 新的筛选范围 |
| `--filter-view-name` | 否 | 新的显示名称 |
