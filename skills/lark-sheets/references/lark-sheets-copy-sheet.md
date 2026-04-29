
# sheets +copy-sheet（复制工作表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +copy-sheet`。

在同一个电子表格内复制指定工作表。

> 底层封装的是官方“操作工作表（operate-sheets）”接口。如果传 `--index`，CLI 会先复制，再追加一次位置更新，把副本移动到目标索引。

> [!CAUTION]
> 这是**写入操作**。可以先用 `--dry-run` 预览。

## 命令

```bash
# 按默认位置复制
lark-cli sheets +copy-sheet --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>"

# 指定副本名称和位置
lark-cli sheets +copy-sheet --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --title "销售副本" --index 2
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 源工作表 ID |
| `--title` | 否 | 新工作表标题，最长 100 字符，不能包含 `/ \ ? * [ ] :` |
| `--index` | 否 | 新工作表位置（从 0 开始） |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 输出

JSON，包含：

- `spreadsheet_token`
- `sheet.sheet_id`
- `sheet.title`
- `sheet.index`

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 获取源 `sheet_id`
- [lark-sheets-create-sheet](lark-sheets-create-sheet.md) — 创建空工作表
- [lark-sheets-delete-sheet](lark-sheets-delete-sheet.md) — 删除复制出的工作表
