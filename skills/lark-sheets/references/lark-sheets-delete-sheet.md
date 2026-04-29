
# sheets +delete-sheet（删除工作表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +delete-sheet`。

删除电子表格中的一个工作表。

> 底层封装的是官方“操作工作表（operate-sheets）”接口。

> [!CAUTION]
> 这是**高风险删除操作**。CLI 会要求显式确认；可以先用 `--dry-run` 预览。

## 命令

```bash
lark-cli sheets +delete-sheet --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 要删除的工作表 ID |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 输出

JSON，包含：

- `deleted`
- `spreadsheet_token`
- `sheet_id`

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 删除前先确认 `sheet_id`
- [lark-sheets-copy-sheet](lark-sheets-copy-sheet.md) — 先复制再删，避免误操作
