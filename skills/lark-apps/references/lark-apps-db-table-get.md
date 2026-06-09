# apps +db-table-get

查看妙搭应用数据库某张表的结构。运行时命令事实以 `lark-cli apps +db-table-get --help` 为准。

## 何时用

用于查看已知表的字段、索引、约束，或给 SQL/迁移生成提供依据。只想知道有哪些表时先 `+db-table-list`。

## 命令骨架

- 必填：`--app-id`、`--table`。
- `--env` 枚举：`dev` / `online`，默认 `online`。
- `--format pretty` 会向服务端请求 DDL，并直接输出 DDL 文本；默认 JSON 返回结构化 columns/indexes/constraints/stats。

## 示例

```bash
lark-cli apps +db-table-get --app-id app_xxx --table orders
lark-cli apps +db-table-get --app-id app_xxx --table orders --env dev --format pretty
```

## 输出契约

- 默认 JSON 读取 `data.name`、`columns`、`indexes`、`constraints`、`estimated_row_count`、`size_bytes`。
- `--format pretty` stdout 是服务端返回的 DDL 文本，不是 JSON envelope；需要建表语句时可原样给用户。

## Agent 规则

需要给用户看建表语句或迁移参照时用 `--format pretty`；需要程序化分析字段/索引/约束时保留默认 JSON。
