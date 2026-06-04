# apps +db-sql

经妙搭服务端在应用数据库执行 SQL。运行时命令事实以 `lark-cli apps +db-sql --help` 为准。

## 何时用

用于通过妙搭服务端执行应用数据库 SQL。不要从环境变量里取连接串裸连数据库；本地调试也走这个 shortcut。

## 命令骨架

- 必填：`--app-id`、`--query`。
- `--query` 支持内联 SQL、`@path` 读文件、`-` 读 stdin。
- `--env` 枚举：`dev` / `online`，默认 `online`。
- risk 是 `write`，因为支持 DML/DDL。
- CLI 永远传 `transactional=false`；不默认包事务。

## 示例

```bash
lark-cli apps +db-sql --app-id app_xxx --env dev --query "select * from orders limit 5"
lark-cli apps +db-sql --app-id app_xxx --env dev --query @./migration.sql --dry-run
```

## 输出契约

- 成功默认 JSON 读取 `data.results[]`；每个元素对应一条 SQL，常见字段有 `sql_type`、`data`、`record_count`、`affected_rows`。
- pretty 会按 SELECT/DML/DDL 自适应渲染；多语句会逐条显示 Statement 摘要。
- 失败可能仍有前序语句已执行；看 `error.detail.statement_index`、`completed`、`rolled_back` 和 `hint` 决定从哪条继续。

## Agent 规则

- 读查询可直接执行；DML/DDL 先 `--dry-run` 给用户确认更稳妥。
- 多语句失败时，失败前的语句可能已经 auto-commit。不要整批重跑；按错误 detail/hint 修失败语句，并从剩余语句继续。
- 如果需要原子性，让用户在 SQL 内显式写 `BEGIN` / `COMMIT`，不要假设 CLI 会包事务。
- 不要把数据库连接串从 env 中取出来裸连。
