
# sheets +create-filter-view（创建筛选视图）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli sheets +create-filter-view`。

在工作表中创建筛选视图，每个工作表最多 150 个筛选视图。

> [!CAUTION]
> 这是**写入操作** —— 执行前必须确认用户意图。可以先用 `--dry-run` 预览。

## 命令

```bash
lark-cli sheets +create-filter-view --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "<sheetId>!A1:H14"

# 指定名称
lark-cli sheets +create-filter-view --spreadsheet-token "shtxxxxxxxx" \
  --sheet-id "<sheetId>" --range "<sheetId>!A1:H14" --filter-view-name "我的筛选"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 否 | 电子表格 URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 否 | 表格 token |
| `--sheet-id` | 是 | 工作表 ID |
| `--range` | 是 | 筛选范围（如 `sheetId!A1:H14`） |
| `--filter-view-name` | 否 | 显示名称（最多 100 字符） |
| `--filter-view-id` | 否 | 自定义 10 位字母数字 ID（不传则自动生成） |

## 输出

JSON，包含 `filter_view`（filter_view_id, filter_view_name, range）。

## 参考

- [lark-sheets-list-filter-views](lark-sheets-list-filter-views.md) — 查询所有筛选视图
- [lark-sheets-create-filter-view-condition](lark-sheets-create-filter-view-condition.md) — 添加筛选条件
