
# sheets +unmerge-cells（拆分单元格）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +unmerge-cells`。

拆分指定范围内的合并单元格。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 拆分 A1:B2 范围的合并单元格
lark-cli sheets +unmerge-cells --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:B2"

# 配合 --sheet-id
lark-cli sheets +unmerge-cells --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "A1:B2"

# 仅预览
lark-cli sheets +unmerge-cells --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:B2" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--range <range>` | 是 | 单元格范围（`<sheetId>!A1:B2`，或配合 `--sheet-id` 使用 `A1:B2`） |
| `--sheet-id <id>` | 否 | 工作表 ID（用于相对范围） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `spreadsheetToken`。

## 参考

- [lark-sheets-merge-cells](lark-sheets-merge-cells.md) — 合并单元格
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
