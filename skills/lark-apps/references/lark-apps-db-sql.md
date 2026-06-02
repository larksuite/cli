# apps +db-sql

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

在妙搭应用数据库上执行 SQL，支持 SELECT / DML（INSERT / UPDATE / DELETE）/ DDL（CREATE / ALTER / DROP）。

多语句**不自动包裹事务**：每条语句独立提交，前面成功的语句会保留落地。需要整批原子性时，在 SQL 内显式写 `BEGIN; ...; COMMIT;`。

## 命令

```bash
# 内联 SQL（JSON envelope）
lark-cli apps +db-sql --app-id app_xxx --query "SELECT * FROM orders LIMIT 10"

# pretty 输出表格
lark-cli apps +db-sql --app-id app_xxx --query "SELECT id, total_cents FROM orders LIMIT 2" --format pretty

# 从文件 / stdin 读 SQL
lark-cli apps +db-sql --app-id app_xxx --query @./migrate.sql --format pretty
cat schema.sql | lark-cli apps +db-sql --app-id app_xxx --query -

# 在 dev 环境执行 DDL
lark-cli apps +db-sql --app-id app_xxx --env dev --query "CREATE TABLE foo (id int);" --format pretty
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--app-id <str>` | ✅ | — | 妙搭应用 ID |
| `--query <str / @path / ->` | ✅ | — | SQL 文本；`@path` 从文件读，`-` 从 stdin 读 |
| `--env <enum>` | ❌ | `online` | 数据库环境：`online`（线上）/ `dev`（开发） |

## 返回值

**成功（JSON 默认 envelope）：** `data.results[]` 每个元素对应一条语句，含 `sql_type` 与结果：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "results": [
      { "sql_type": "SELECT", "data": "[{\"id\":101,\"total_cents\":2500}]", "record_count": 1 }
    ]
  }
}
```

**成功（`--format pretty`，按语句形态自适应）：**

```
# 单条 SELECT → 表格        # 空结果      # 单条 DML                    # 单条 DDL
id   total_cents            (0 rows)      ✓ 1 row updated                ✓ DDL executed
101  2500

# 多语句全部成功：逐条 + 末尾汇总
Statement 1: ✓ 1 row inserted

Statement 2: SELECT (1 row)
id   total_cents
999  101

✓ 2 statements executed
```

**失败（语句执行出错）：** 返回 `type: api_error`（退出码 `1`），`code` / `message` 由服务端给出。`detail` 给出失败定位与已落地的语句，便于精确重试：

```json
{
  "ok": false,
  "identity": "user",
  "error": {
    "type": "api_error",
    "code": 1300002,
    "message": "duplicate key value violates unique constraint (at statement 2 of 2)",
    "hint": "statements 1-1 were already applied; fix statement 2 and re-run only the remaining statements.",
    "detail": {
      "statement_index": 1,
      "completed": [ { "sql_type": "INSERT", "affected_rows": 1 } ],
      "rolled_back": false
    }
  }
}
```

pretty 模式下，失败前的逐条结果照常打印，并提示哪些语句已生效：

```
Statement 1: ✓ 1 row inserted

Statement 2: ✗ duplicate key value violates unique constraint [1300002]

(statement 2 failed; 1 statement before it already applied)
```

**失败（参数校验）：**

```json
{ "ok": false, "error": { "type": "validation", "message": "--query is empty (no inline SQL, file, or stdin content)" } }
```

## 字段语义

- **失败不回滚已成功的语句**：`rolled_back: false` 表示失败语句**之前**的语句已落地，`detail.completed` 列出这些语句。收到此类错误时**不要整批重跑**——否则前序语句会被重复执行；按 `detail.statement_index`（0-based）只重跑剩余语句。要整批原子性请在 SQL 内显式 `BEGIN; ...; COMMIT;`。
- `data.results[].sql_type`：`SELECT` 携带 `data`（行数组 JSON 字符串）+ `record_count`；DML 携带 `affected_rows`；DDL（建表 / 改表等）执行成功后 pretty 统一显示 `✓ DDL executed`。
- 失败时优先转述 `error.hint`（为空则 `error.message`），不要原样复述 envelope JSON。

## 协同命令

| 场景 | 命令 |
|---|---|
| 先看有哪些表 / 表结构 | `apps +db-table-list` / `apps +db-table-schema` |
| 尚未拆分 dev / online 双环境 | `apps +db-dev-init`（不可逆，需确认） |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
