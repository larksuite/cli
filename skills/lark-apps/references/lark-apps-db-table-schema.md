# apps +db-table-schema

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

查看单张数据表的结构定义。输出形态由 `--format` 决定：

- 默认（`json` / `table` / `ndjson` / `csv`）：返回结构化的列、索引、约束与统计信息。
- `--format pretty`：直接输出建表 DDL 文本（`CREATE TABLE` 语句）。

## 命令

```bash
# 结构化 schema（JSON 默认 envelope）
lark-cli apps +db-table-schema --app-id app_xxx --table orders

# DDL 文本
lark-cli apps +db-table-schema --app-id app_xxx --table orders --format pretty

# 抽取列名列表
lark-cli apps +db-table-schema --app-id app_xxx --table orders -q '.data.columns[].name'

# dev 环境的表结构
lark-cli apps +db-table-schema --app-id app_xxx --table orders --env dev
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--app-id <str>` | ✅ | — | 妙搭应用 ID |
| `--table <str>` | ✅ | — | 表名（缺失时退出码 `1`） |
| `--env <enum>` | ❌ | `online` | 数据库环境：`online`（线上）/ `dev`（开发） |

## 返回值

**成功（JSON 默认 envelope）：**

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "name": "orders",
    "description": "订单表",
    "columns": [
      { "name": "id", "data_type": "int4", "is_primary_key": true, "is_unique": false, "is_allow_null": false, "is_array": false, "is_auto_increment": false, "default_value": "", "description": "主键" }
    ],
    "indexes": [ { "name": "orders_pkey", "columns": ["id"], "type": "btree", "definition": "CREATE UNIQUE INDEX ..." } ],
    "constraints": [ { "type": "primary_key", "name": "orders_pkey", "columns": ["id"] } ],
    "estimated_row_count": 1200,
    "size_bytes": 81920
  }
}
```

**成功（`--format pretty`，建表 DDL 文本）：**

```sql
CREATE TABLE orders (
  id integer NOT NULL,
  ...
  CONSTRAINT orders_pkey PRIMARY KEY (id)
);
CREATE INDEX idx_orders_status ON orders USING btree (status);
COMMENT ON TABLE orders IS '订单表';
```

**失败（表不存在）：** 返回 `type: api_error`，`code` 与 `message` 由服务端给出：

```json
{
  "ok": false,
  "identity": "user",
  "error": { "type": "api_error", "code": 500002763, "message": "数据表格不存在" }
}
```

## 字段语义

- `columns[].data_type` 为数据库原生类型名（如 `int4` / `timestamptz`），并带 `is_array` / `is_auto_increment` 等布尔位；按需取用即可。
- `indexes` / `constraints` / `estimated_row_count` / `size_bytes` 为可选字段，存在性视表而定。
- `--format pretty` 输出的是可直接执行的建表 DDL 文本（含索引、注释），原样使用即可。
- 失败时优先转述 `error.hint`（为空则 `error.message`），不要原样复述 envelope JSON。

## 协同命令

| 场景 | 命令 |
|---|---|
| 先看有哪些表 | `apps +db-table-list --app-id <id>` |
| 修改表结构 | `apps +db-sql --app-id <id> --query "ALTER TABLE ..."` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
