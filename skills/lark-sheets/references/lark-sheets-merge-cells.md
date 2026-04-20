
# sheets +merge-cells（合并单元格）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +merge-cells`。

合并指定范围的单元格，支持全合并、按行合并、按列合并三种模式。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 全合并 A1:B2
lark-cli sheets +merge-cells --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:B2" --merge-type MERGE_ALL

# 按行合并，配合 --sheet-id
lark-cli sheets +merge-cells --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "A1:D4" --merge-type MERGE_ROWS

# 仅预览
lark-cli sheets +merge-cells --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:B2" --merge-type MERGE_ALL --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--range <range>` | 是 | 单元格范围（`<sheetId>!A1:B2`，或配合 `--sheet-id` 使用 `A1:B2`） |
| `--sheet-id <id>` | 否 | 工作表 ID（用于相对范围） |
| `--merge-type <type>` | 是 | 合并方式：`MERGE_ALL`（全合并）、`MERGE_ROWS`（按行）、`MERGE_COLUMNS`（按列） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `spreadsheetToken`。

## 参考

- [lark-sheets-unmerge-cells](lark-sheets-unmerge-cells.md) — 拆分单元格
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
