
# sheets +delete-dimension（删除行列）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +delete-dimension`。

删除指定范围的行或列，已有数据向上或向左移动。

> [!CAUTION]
> 这是**破坏性写入操作** —— 删除后数据不可恢复。执行前必须确认用户意图，建议先用 `--dry-run` 预览。

## 命令

```bash
# 删除第 3-7 行（1-indexed，闭区间）
lark-cli sheets +delete-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 3 --end-index 7

# 删除第 5-8 列
lark-cli sheets +delete-dimension --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension COLUMNS --start-index 5 --end-index 8

# 仅预览参数
lark-cli sheets +delete-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 3 --end-index 7 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--sheet-id <id>` | 是 | 工作表 ID |
| `--dimension <ROWS\|COLUMNS>` | 是 | 操作维度：`ROWS` 或 `COLUMNS` |
| `--start-index <n>` | 是 | 起始位置（**1-indexed**，含） |
| `--end-index <n>` | 是 | 结束位置（**1-indexed**，含） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含：

- `delCount`：实际删除的行/列数
- `majorDimension`：`ROWS` 或 `COLUMNS`

## 参考

- [lark-sheets-add-dimension](lark-sheets-add-dimension.md) — 增加行列
- [lark-sheets-insert-dimension](lark-sheets-insert-dimension.md) — 插入行列
- [lark-sheets-info](lark-sheets-info.md) — 查看当前行列数
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
