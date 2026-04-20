
# sheets +update-dropdown（更新下拉列表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-dropdown`。

更新已有下拉列表的选项、颜色等配置。可同时更新多个范围。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
lark-cli sheets +update-dropdown --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" \
  --ranges '["<sheetId>!A1:A100", "<sheetId>!C1:C100"]' \
  --condition-values '["新选项1", "新选项2", "新选项3"]'

# 更新为多选 + 着色
lark-cli sheets +update-dropdown --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" \
  --ranges '["<sheetId>!A1:A100"]' \
  --condition-values '["选项A", "选项B"]' \
  --multiple --highlight --colors '["#1FB6C1", "#F006C2"]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--ranges` | 是 | 范围 JSON 数组（如 `'["sheetId!A1:A100"]'`） |
| `--condition-values` | 是 | 新的下拉选项，JSON 数组 |
| `--multiple` | 否 | 是否多选，默认 false |
| `--highlight` | 否 | 是否着色，默认 false |
| `--colors` | 否 | RGB 颜色 JSON 数组，需与 `--condition-values` 一一对应 |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `spreadsheetToken`、`sheetId`、`dataValidation`（选项值和颜色映射）。

## 参考

- [lark-sheets-set-dropdown](lark-sheets-set-dropdown.md) — 设置下拉列表
- [lark-sheets-get-dropdown](lark-sheets-get-dropdown.md) — 查询下拉列表
- [lark-sheets-delete-dropdown](lark-sheets-delete-dropdown.md) — 删除下拉列表
