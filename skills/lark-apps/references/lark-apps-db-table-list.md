# apps +db-table-list

列出妙搭应用某个数据库环境的数据表。运行时命令事实以 `lark-cli apps +db-table-list --help` 为准。

## 何时用

用于先摸清应用数据库里有哪些表，或在用户只给业务对象名时定位可能的表名。已知表名且要字段/索引时直接用 `+db-table-get`。

## 命令骨架

- 必填：`--app-id`。
- `--env` 枚举：`dev` / `online`，默认 `online`。
- 分页：`--page-size` 默认 20，`--page-token` 使用上一页 cursor。
- pretty 输出列包含 `name`、`description`、`estimated_row_count`、`size`、`columns`（列数）。

## 示例

```bash
lark-cli apps +db-table-list --app-id app_xxx
lark-cli apps +db-table-list --app-id app_xxx --env dev --page-size 50
```

## 输出契约

- 成功读取 `data.items[]`；每项字段是 `name`、`description`、`estimated_row_count`、`size_bytes`、`column_count`（列数）。CLI 默认不透出每表完整 `columns[]`（与 `+db-table-get` 重复且放大 token），只给 `column_count`；要完整列定义/索引/约束用 `+db-table-get`。
- pretty 输出是 5 列扫描表：`name`、`description`、`estimated_row_count`、`size`、`columns`（即列数）。
- 若响应带 `has_more=true`，用返回的 `page_token` / `next_page_token` 翻页。

## Agent 规则

用户说“本地/开发库/调试库”时优先 `--env dev`；线上问题排查用 `--env online`。如果 dev 返回服务端错误提示未初始化，多环境入口是 [`+db-env-create`](lark-apps-db-env-create.md)。
