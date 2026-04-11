
# sheets +replace（替换单元格）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +replace`。

在指定范围内查找并替换单元格内容，支持正则、大小写敏感、全单元格匹配等选项。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 简单替换
lark-cli sheets +replace --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --find "hello" --replacement "world"

# 指定范围 + 大小写敏感
lark-cli sheets +replace --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "A1:C5" \
  --find "Hello" --replacement "World" --match-case

# 正则替换
lark-cli sheets +replace --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --find "\\d{4}-\\d{2}-\\d{2}" \
  --replacement "DATE" --search-by-regex

# 仅预览
lark-cli sheets +replace --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --find "old" --replacement "new" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--sheet-id <id>` | 是 | 工作表 ID |
| `--find <text>` | 是 | 搜索文本（启用 `--search-by-regex` 时为正则表达式） |
| `--replacement <text>` | 是 | 替换文本 |
| `--range <range>` | 否 | 搜索范围（不传则搜索整个工作表） |
| `--match-case` | 否 | 区分大小写 |
| `--match-entire-cell` | 否 | 匹配整个单元格 |
| `--search-by-regex` | 否 | 使用正则表达式搜索 |
| `--include-formulas` | 否 | 在公式中搜索 |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `replace_result`：

- `matched_cells`：匹配的非公式单元格列表
- `matched_formula_cells`：匹配的公式单元格列表
- `rows_count`：包含匹配的行数

## 参考

- [lark-sheets-find](lark-sheets-find.md) — 查找单元格（只查不改）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
