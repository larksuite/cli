
# sheets +get-dropdown（查询下拉列表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +get-dropdown`。

查询指定范围内已配置的下拉列表设置，包括选项值、是否多选、颜色映射等。

## 命令

```bash
lark-cli sheets +get-dropdown --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --range "<sheetId>!A2:A100"

lark-cli sheets +get-dropdown --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A2:A100"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--range` | 是 | 范围（如 `<sheetId>!A2:A100`） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含：

- `dataValidations[].conditionValues` — 下拉选项列表
- `dataValidations[].ranges` — 应用范围
- `dataValidations[].options.multipleValues` — 是否多选
- `dataValidations[].options.highlightValidData` — 是否着色
- `dataValidations[].options.colorValueMap` — 选项颜色映射

## 参考

- [lark-sheets-set-dropdown](lark-sheets-set-dropdown.md) — 设置下拉列表
- [lark-sheets-update-dropdown](lark-sheets-update-dropdown.md) — 更新下拉列表
- [lark-sheets-delete-dropdown](lark-sheets-delete-dropdown.md) — 删除下拉列表
