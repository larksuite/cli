
# sheets +delete-dropdown（删除下拉列表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +delete-dropdown`。

删除指定范围的下拉列表配置。支持一次删除多个范围。

> [!CAUTION]
> 这是**删除操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 删除单个范围
lark-cli sheets +delete-dropdown --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --ranges '["<sheetId>!A2:A100"]'

# 删除多个范围
lark-cli sheets +delete-dropdown --spreadsheet-token "shtxxxxxxxx" \
  --ranges '["<sheetId>!A2:A100", "<sheetId>!C1:C50"]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--ranges` | 是 | 范围 JSON 数组（如 `'["sheetId!A2:A100"]'`），单个范围最多 5000 格，单次最多 100 个范围 |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含：

- `rangeResults[].range` — 对应的范围
- `rangeResults[].success` — 是否成功
- `rangeResults[].updatedCells` — 影响的单元格数量

## 参考

- [lark-sheets-set-dropdown](lark-sheets-set-dropdown.md) — 设置下拉列表
- [lark-sheets-update-dropdown](lark-sheets-update-dropdown.md) — 更新下拉列表
- [lark-sheets-get-dropdown](lark-sheets-get-dropdown.md) — 查询下拉列表
