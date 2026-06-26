# apps +db-execute

经妙搭服务端在应用数据库执行 SQL。运行时命令事实以 `lark-cli apps +db-execute --help` 为准。

## 何时用

用于通过妙搭服务端执行应用数据库 SQL。不要从环境变量里取连接串裸连数据库；本地调试也走这个 shortcut。

## 命令骨架

- 必填：`--app-id`，以及 `--sql` / `--file` 二选一（互斥）。
- `--sql`：内联 SQL 文本；传 `-` 时从 stdin 读。绝对路径文件经 stdin 传入：`--sql - < <absolute-path>`（shell 解析路径，CLI 仅接收内容）。
- `--file`：`.sql` 文件路径，需为工作目录内的相对路径（如 `--file ./migration.sql`）；绝对路径、或经 `..`/符号链接越出工作目录的路径会被拒绝。文件不在工作目录内时，改用 `--sql - < <文件路径>` 经 stdin 传入。
- `--environment` 枚举：`dev` / `online`，**默认 `dev`**；操作线上库、或**未开启多环境的应用（其数据库在 `online`，没有 dev 分支）**时显式 `--environment online`。旧名 `--env` 已**移除**：传入会报 validation 错（提示改用 `--environment`），一律用 `--environment`。
- risk 是 `high-risk-write`（SQL 可含 DML/DDL）：任何执行都需 `--yes`，否则返回 `confirmation_required` / exit 10。`--dry-run` 预览不需要 `--yes`。
- **不会自动为你包事务，事务边界需自己在 SQL 里控制**：多语句默认逐条独立提交，中间某条失败时前序语句已生效、不会回滚；若需要「要么全部成功、要么全部回滚」的原子性，请在 SQL 内显式写 `BEGIN … COMMIT`（详见下「Agent 规则」）。

## 示例

```bash
lark-cli apps +db-execute --app-id app_xxx --environment dev --sql "select * from orders limit 5" --yes
lark-cli apps +db-execute --app-id app_xxx --environment dev --file ./migration.sql --dry-run
# 绝对路径文件 / cwd 不固定：经 stdin 传入
lark-cli apps +db-execute --app-id app_xxx --environment dev --sql - --yes < /Users/.../migrations/0001_init.sql
```

## 输出契约

- 成功默认 JSON 的 `data` 按 SQL 类型自适应（不透传后端原始串）：
  - 单 SELECT → `data` 是行数组 `[{...}]`（空 → `[]`），直接 `-q '.data[].col'` 取字段。
  - 单 DML → `data = {command, rows_affected}`（如 `{"command":"INSERT","rows_affected":1}`）。
  - 单 DDL → `data = {command}`（如 `{"command":"CREATE_TABLE"}`）。
  - 多语句 → `data` 是元素数组：SELECT 为 `{command:"SELECT", rows:[...]}`，DML 为 `{command, rows_affected}`，DDL 为 `{command}`。
- pretty 会按 SELECT/DML/DDL 自适应渲染；多语句会逐条显示 Statement 摘要。
- 失败返回 typed `error`（`type:"api"`、`subtype:"server_error"`、`code`、`message`、`hint`）：失败位置在 `message` 的「(at statement N of M)」；前序是否落地 / 是否整批回滚写在 `hint`——事务内失败「Transaction rolled back; no changes persisted.」；非事务多语句前序已落地「Earlier statements were committed and not rolled back; fix statement N and re-run the remaining statements.」；首句即失败（无前序落地）「No statements were applied; fix the SQL and re-run.」。据此决定整段重跑还是只跑剩余语句。

## Agent 规则

- 该命令为 high-risk-write，执行一律需 `--yes`；无 `--yes` 会返回 `confirmation_required` / exit 10。
  - **只读查询、以及不删除/不丢失既有数据且可撤回的语句**：已授权时可直接带 `--yes` 执行。
  - **会删除或丢失既有数据、或难以撤回的语句**：先 `--dry-run` 预览（无需 `--yes`），向用户确认后再带 `--yes` 执行；不要在用户不知情时自动补 `--yes`。
- 多语句失败时，失败前的语句可能已经 commit 落地。不要整批重跑；按错误 message/hint 修失败语句，并从剩余语句继续。
- 如果需要原子性，让用户在 SQL 内显式写 `BEGIN` / `COMMIT`，不要假设 CLI 会包事务。
- 不要把数据库连接串从 env 中取出来裸连。
