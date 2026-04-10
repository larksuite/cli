
# sheets +set-style（设置单元格样式）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +set-style`。

对指定范围的单元格设置样式（字体、颜色、对齐、边框等）。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
# 设置加粗 + 红色背景
lark-cli sheets +set-style --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:C3" \
  --style '{"font":{"bold":true},"backColor":"#ff0000"}'

# 配合 --sheet-id + 居中对齐
lark-cli sheets +set-style --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "A1:D1" \
  --style '{"hAlign":1,"vAlign":1,"font":{"bold":true,"font_size":"12pt/1.5"}}'

# 清除格式
lark-cli sheets +set-style --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:Z100" --style '{"clean":true}'

# 仅预览
lark-cli sheets +set-style --spreadsheet-token "shtxxxxxxxx" \
  --range "<sheetId>!A1:B2" --style '{"foreColor":"#0000ff"}' --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url <url>` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token <token>` | 否 | 表格 token（与 `--url` 二选一） |
| `--range <range>` | 是 | 单元格范围（`<sheetId>!A1:B2`，或配合 `--sheet-id`） |
| `--sheet-id <id>` | 否 | 工作表 ID（用于相对范围） |
| `--style <json>` | 是 | 样式 JSON 对象 |
| `--dry-run` | 否 | 仅打印参数，不执行请求 |

### style JSON 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `font.bold` | bool | 加粗 |
| `font.italic` | bool | 斜体 |
| `font.font_size` | string | 字号，如 `"12pt/1.5"` |
| `font.clean` | bool | 清除字体格式 |
| `textDecoration` | int | 0=无, 1=下划线, 2=删除线, 3=两者 |
| `formatter` | string | 数字格式 |
| `hAlign` | int | 水平对齐：0=左, 1=居中, 2=右 |
| `vAlign` | int | 垂直对齐：0=上, 1=居中, 2=下 |
| `foreColor` | string | 字体颜色（hex，如 `"#000000"`） |
| `backColor` | string | 背景色（hex） |
| `borderType` | string | 边框：FULL_BORDER, OUTER_BORDER, INNER_BORDER, NO_BORDER 等 |
| `borderColor` | string | 边框颜色（hex） |
| `clean` | bool | 清除所有格式 |

## 输出

JSON，包含 `updates`（updatedRange, updatedRows, updatedColumns, updatedCells, revision）。

## 参考

- [lark-sheets-batch-set-style](lark-sheets-batch-set-style.md) — 批量设置样式
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
