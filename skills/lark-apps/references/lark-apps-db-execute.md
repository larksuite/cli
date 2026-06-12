# apps +db-execute

经妙搭服务端在应用数据库执行 SQL。运行时命令事实以 `lark-cli apps +db-execute --help` 为准。

## 何时用

用于通过妙搭服务端执行应用数据库 SQL。不要从环境变量里取连接串裸连数据库；本地调试也走这个 shortcut。

## 命令骨架

- 必填：`--app-id`，以及 `--sql` / `--file` 二选一（互斥）。
- `--sql`：内联 SQL 文本；传 `-` 时从 stdin 读。绝对路径文件经 stdin 传入：`--sql - < <absolute-path>`（shell 解析路径，CLI 仅接收内容）。
- `--file`：`.sql` 文件路径，需为工作目录内的相对路径（如 `--file ./migration.sql`）；绝对路径、或经 `..`/符号链接越出工作目录的路径会被拒绝。文件不在工作目录内时，改用 `--sql - < <文件路径>` 经 stdin 传入。
- `--env` 枚举：`dev` / `online`，**默认 `dev`**；需要操作线上环境数据库时，显式指定 `--env online`。
- risk 是 `high-risk-write`（SQL 可含 DML/DDL）：任何执行都需 `--yes`，否则返回 `confirmation_required` / exit 10。`--dry-run` 预览不需要 `--yes`。
- CLI 永远传 `transactional=false`；不默认包事务。

## 示例

```bash
lark-cli apps +db-execute --app-id app_xxx --env dev --sql "select * from orders limit 5" --yes
lark-cli apps +db-execute --app-id app_xxx --env dev --file ./migration.sql --dry-run
# 绝对路径文件 / cwd 不固定：经 stdin 传入
lark-cli apps +db-execute --app-id app_xxx --env dev --sql - --yes < /Users/.../migrations/0001_init.sql
```

## 输出契约

- 成功默认 JSON 读取 `data.results[]`；每个元素对应一条 SQL，常见字段有 `sql_type`、`data`、`record_count`、`affected_rows`。
- pretty 会按 SELECT/DML/DDL 自适应渲染；多语句会逐条显示 Statement 摘要。
- 失败可能仍有前序语句已执行；此时 stdout 输出 `ok:false` 的 envelope（exit 非 0），从 `data` 读 `results[]`（全部逐条结果，失败语句 `sql_type` 为 `ERROR`）、`statement_index`、`error_code`、`error_message`、`rolled_back` 和 `note`，决定从哪条继续。

## Agent 规则

- 该命令为 high-risk-write，执行一律需 `--yes`；无 `--yes` 会返回 `confirmation_required` / exit 10。
  - **只读查询、以及不删除/不丢失既有数据且可撤回的语句**：已授权时可直接带 `--yes` 执行。
  - **会删除或丢失既有数据、或难以撤回的语句**：先 `--dry-run` 预览（无需 `--yes`），向用户确认后再带 `--yes` 执行；不要在用户不知情时自动补 `--yes`。
- 多语句失败时，失败前的语句可能已经 auto-commit。不要整批重跑；按错误 detail/hint 修失败语句，并从剩余语句继续。
- 如果需要原子性，让用户在 SQL 内显式写 `BEGIN` / `COMMIT`，不要假设 CLI 会包事务。
- 不要把数据库连接串从 env 中取出来裸连。
