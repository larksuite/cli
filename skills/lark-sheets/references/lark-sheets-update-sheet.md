
# sheets +update-sheet（更新工作表属性）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-sheet`。

更新工作表标题、位置、隐藏状态、冻结行列，或保护设置。

> [!CAUTION]
> 这是**写入操作**。可以先用 `--dry-run` 预览。

## 命令

```bash
# 改名 + 调整冻结
lark-cli sheets +update-sheet --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --title "汇总表" --frozen-row-count 2 --frozen-col-count 1

# 隐藏工作表
lark-cli sheets +update-sheet --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --hidden=true

# 开启保护并授权额外编辑人
lark-cli sheets +update-sheet --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --lock LOCK --lock-info "仅财务维护" \
  --user-id-type open_id --user-ids '["ou_xxx","ou_yyy"]'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 要更新的工作表 ID |
| `--title` | 否 | 新标题，最长 100 字符，不能包含 `/ \ ? * [ ] :` |
| `--index` | 否 | 新位置（从 0 开始） |
| `--hidden` | 否 | `--hidden=true` 隐藏，`--hidden=false` 取消隐藏 |
| `--frozen-row-count` | 否 | 冻结行数，`0` 表示取消冻结 |
| `--frozen-col-count` | 否 | 冻结列数，`0` 表示取消冻结 |
| `--lock` | 否 | 保护模式：`LOCK` / `UNLOCK` |
| `--lock-info` | 否 | 保护备注；要求 `--lock LOCK` |
| `--user-id-type` | 否 | `--user-ids` 的 ID 类型：`open_id` / `union_id` / `lark_id` / `user_id` |
| `--user-ids` | 否 | 额外可编辑用户 ID 的 JSON 数组；要求 `--lock LOCK` |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 输出

JSON，包含：

- `spreadsheet_token`
- `sheet.sheet_id`
- `sheet.title`
- `sheet.hidden`
- `sheet.grid_properties.frozen_row_count`
- `sheet.grid_properties.frozen_column_count`
- `sheet.protect`

## 说明

- 这个 shortcut 按官方文档封装了“更新工作表属性”接口；底层请求仍是 `sheets_batch_update`，但只构造单个 `updateSheet` 请求，避免手写嵌套 JSON。
- 至少需要传一个更新字段；否则 CLI 会直接报校验错误。

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 先获取 `sheet_id`
- [lark-sheets-create-sheet](lark-sheets-create-sheet.md) — 创建工作表
- [lark-sheets-delete-sheet](lark-sheets-delete-sheet.md) — 删除工作表
