
# sheets +batch-set-style（批量设置单元格样式）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +batch-set-style`。

对多个范围批量设置不同的单元格样式，一次请求可包含多组范围和样式。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 对两组范围分别设置样式
lark-cli sheets +batch-set-style --spreadsheet-token "shtxxxxxxxx" \
  --data '[{"ranges":["<sheetId>!A1:C3"],"style":{"font":{"bold":true},"backColor":"#21d11f"}},{"ranges":["<sheetId>!D1:F3"],"style":{"foreColor":"#ff0000"}}]'

# 同一样式应用到多个范围
lark-cli sheets +batch-set-style --spreadsheet-token "shtxxxxxxxx" \
  --data '[{"ranges":["<sheetId>!A1:B2","<sheetId>!D4:E5"],"style":{"hAlign":1,"font":{"bold":true}}}]'

# 仅预览
lark-cli sheets +batch-set-style --spreadsheet-token "shtxxxxxxxx" \
  --data '[{"ranges":["<sheetId>!A1:B2"],"style":{"backColor":"#0000ff"}}]' --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--data <json>` | 是 | JSON 数组，每项包含 `ranges`（字符串数组）和 `style`（样式对象） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

### style 对象字段

与 `+set-style` 相同，参见 [lark-sheets-set-style](lark-sheets-set-style.md)。

## 输出

JSON，包含：

- `totalUpdatedRows/totalUpdatedColumns/totalUpdatedCells`：汇总更新量
- `revision`：工作表版本号
- `responses[]`：每个范围的更新详情

## 参考

- [lark-sheets-set-style](lark-sheets-set-style.md) — 单范围设置样式
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
