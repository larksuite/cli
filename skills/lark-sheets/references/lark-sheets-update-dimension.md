
# sheets +update-dimension（更新行列属性）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +update-dimension`。

更新指定范围行/列的属性，支持设置显隐状态和行高/列宽。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 隐藏第 1-3 行
lark-cli sheets +update-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 1 --end-index 3 \
  --visible=false

# 设置第 1-5 列列宽为 120px
lark-cli sheets +update-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension COLUMNS --start-index 1 --end-index 5 \
  --fixed-size 120

# 同时设置显示 + 行高
lark-cli sheets +update-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 1 --end-index 10 \
  --visible=true --fixed-size 50

# 仅预览参数
lark-cli sheets +update-dimension --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --dimension ROWS --start-index 1 --end-index 3 \
  --visible=true --dry-run
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
| `--visible <true\|false>` | 否 | `true` 显示 / `false` 隐藏（须与 `--fixed-size` 至少传一个） |
| `--fixed-size <px>` | 否 | 行高或列宽（像素）（须与 `--visible` 至少传一个） |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

> **注意**：`--visible` 是 bool flag，传值时使用 `--visible=true` 或 `--visible=false` 格式。

## 输出

JSON（成功时 `data` 为空对象 `{}`）。

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 查看当前行列属性
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
