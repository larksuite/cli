
# sheets +add-dimension（增加行列）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +add-dimension`。

在工作表末尾追加空行或空列，不影响已有数据。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 在末尾追加 10 行
lark-cli sheets +add-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --length 10

# 在末尾追加 3 列
lark-cli sheets +add-dimension --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension COLUMNS --length 3

# 仅预览参数（不发请求）
lark-cli sheets +add-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --length 5 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--sheet-id <id>` | 是 | 工作表 ID |
| `--dimension <ROWS\|COLUMNS>` | 是 | 操作维度：`ROWS` 或 `COLUMNS` |
| `--length <n>` | 是 | 追加数量（1-5000） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON，包含：

- `addCount`：实际追加的行/列数
- `majorDimension`：`ROWS` 或 `COLUMNS`

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 查看当前行列数
- [lark-sheets-delete-dimension](lark-sheets-delete-dimension.md) — 删除行列
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
