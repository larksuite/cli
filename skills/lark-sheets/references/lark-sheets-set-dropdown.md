
# sheets +set-dropdown（设置下拉列表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +set-dropdown`。

为指定范围的单元格配置下拉列表选项。**这是使用 `multipleValue` 格式写入数据的前置步骤**——未配置下拉选项的单元格，`multipleValue` 写入会变成纯文本。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 基础：设置单选下拉
lark-cli sheets +set-dropdown --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --range "<sheetId>!A2:A100" --condition-values '["选项1", "选项2", "选项3"]'

# 多选 + 颜色高亮
lark-cli sheets +set-dropdown --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A2:A100" --condition-values '["选项1", "选项2", "选项3"]' \
  --multiple --highlight --colors '["#1FB6C1", "#F006C2", "#FB16C3"]'

# 仅预览参数（不发请求）
lark-cli sheets +set-dropdown --url "https://..." --range "..." --condition-values '...' --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--range` | 是 | 范围（如 `<sheetId>!A2:A100`），单次最多 5000 行 x 100 列 |
| `--condition-values` | 是 | 下拉选项，JSON 数组（如 `'["选项1","选项2"]'`），最多 500 个，每个 ≤100 字符，不能包含逗号 |
| `--multiple` | 否 | 是否多选，默认 false |
| `--highlight` | 否 | 是否着色，默认 false |
| `--colors` | 否 | RGB 十六进制颜色 JSON 数组，需与 `--condition-values` 一一对应（`--highlight` 时必填） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含 `code`（0=成功）和 `msg`。

## 典型流程

```bash
# 1. 先配置下拉选项
lark-cli sheets +set-dropdown --url "<url>" \
  --range "<sheetId>!J2:J100" --condition-values '["选项1","选项2"]' --multiple

# 2. 再用 multipleValue 写入
lark-cli sheets +write --url "<url>" --sheet-id "<sheetId>" --range "J2" \
  --values '[[{"type":"multipleValue","values":["选项1","选项2"]}]]'
```

## 参考

- [lark-sheets-update-dropdown](lark-sheets-update-dropdown.md) — 更新下拉列表
- [lark-sheets-get-dropdown](lark-sheets-get-dropdown.md) — 查询下拉列表
- [lark-sheets-delete-dropdown](lark-sheets-delete-dropdown.md) — 删除下拉列表
