
# sheets +move-dimension（移动行列）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +move-dimension`。

将指定范围的行/列移动到目标位置。被移动到目标位置后，原本在目标位置的行/列会对应右移或下移。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 将第 0-1 行移动到第 4 行位置
lark-cli sheets +move-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS \
  --start-index 0 --end-index 1 --destination-index 4

# 将第 2 列移动到第 0 列位置
lark-cli sheets +move-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension COLUMNS \
  --start-index 2 --end-index 2 --destination-index 0

# 仅预览参数
lark-cli sheets +move-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS \
  --start-index 0 --end-index 1 --destination-index 4 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--sheet-id <id>` | 是 | 工作表 ID |
| `--dimension <ROWS\|COLUMNS>` | 是 | 操作维度：`ROWS` 或 `COLUMNS` |
| `--start-index <n>` | 是 | 源起始位置（0-indexed） |
| `--end-index <n>` | 是 | 源结束位置（0-indexed，含） |
| `--destination-index <n>` | 是 | 目标位置（0-indexed） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON（成功时 `data` 为空对象 `{}`）。

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 查看当前行列数
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
