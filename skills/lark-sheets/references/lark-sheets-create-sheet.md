
# sheets +create-sheet（创建工作表）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +create-sheet`。

在已存在的电子表格中新增一个工作表。

> 底层封装的是官方“操作工作表（operate-sheets）”接口。

> [!CAUTION]
> 这是**写入操作**。可以先用 `--dry-run` 预览。

## 命令

```bash
# 在表格末尾或服务端默认位置创建工作表
lark-cli sheets +create-sheet --spreadsheet-token "shtxxxxxxxx" \
  --title "明细"

# 指定插入位置（0-based）
lark-cli sheets +create-sheet --url "https://example.larksuite.com/sheets/shtxxxxxxxx" \
  --title "汇总" --index 0
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--title` | 否 | 工作表标题，最长 100 字符，不能包含 `/ \ ? * [ ] :` |
| `--index` | 否 | 工作表位置（从 0 开始） |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 输出

JSON，包含：

- `spreadsheet_token`
- `sheet.sheet_id`
- `sheet.title`
- `sheet.index`

## 参考

- [lark-sheets-info](lark-sheets-info.md) — 先查看现有工作表和 `sheet_id`
- [lark-sheets-copy-sheet](lark-sheets-copy-sheet.md) — 复制工作表
- [lark-sheets-update-sheet](lark-sheets-update-sheet.md) — 更新工作表属性
