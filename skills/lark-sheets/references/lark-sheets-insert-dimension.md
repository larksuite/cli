
# sheets +insert-dimension（插入行列）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +insert-dimension`。

在指定位置插入空行或空列，已有数据向下或向右移动。支持继承相邻行/列样式。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 在第 3 行前插入 4 行空行（0-indexed，插入位置 3~7，不含 7）
lark-cli sheets +insert-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 3 --end-index 7

# 插入列，并继承前方列的样式
lark-cli sheets +insert-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension COLUMNS --start-index 2 --end-index 4 \
  --inherit-style BEFORE

# 仅预览参数
lark-cli sheets +insert-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 0 --end-index 2 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--sheet-id <id>` | 是 | 工作表 ID |
| `--dimension <ROWS\|COLUMNS>` | 是 | 操作维度：`ROWS` 或 `COLUMNS` |
| `--start-index <n>` | 是 | 起始位置（0-indexed） |
| `--end-index <n>` | 是 | 结束位置（0-indexed，不包含；插入数量 = end - start） |
| `--inherit-style <BEFORE\|AFTER>` | 否 | 样式继承方向：`BEFORE` 继承前方、`AFTER` 继承后方；不传则为空白样式 |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

## 输出

JSON（成功时 `data` 为空对象 `{}`）。

## 参考

- [lark-sheets-add-dimension](lark-sheets-add-dimension.md) — 在末尾追加行列
- [lark-sheets-delete-dimension](lark-sheets-delete-dimension.md) — 删除行列
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
