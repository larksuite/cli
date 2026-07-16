# apps +db-execute

经妙搭服务端在应用数据库执行 SQL。运行时命令事实以 `lark-cli apps +db-execute --help` 为准。

> **写 SQL 前先看文末「平台 SQL 规范」**：妙搭底层是 PostgreSQL + 一层平台约束，SQL 内容不符合会被服务端直接拒或建出行为不对的表。最容易踩的三条：① 建业务表必须带 4 个审计列（`_created_at`/`_updated_at`/`_created_by`/`_updated_by`）+ 启用 RLS + 4 条 policy，一次调用里写全；② 人员字段用内置复合类型 `user_profile`（写入 `ROW('<user_id>')::user_profile`，查询解引用 `(field).user_id`）；③ `CREATE/DROP DATABASE·SCHEMA·USER·ROLE`、非白名单 `CREATE EXTENSION`、平台保留表 `auth`/`users` 会被硬拒，`online` 环境禁 DDL。

## 何时用

用于通过妙搭服务端执行应用数据库 SQL。不要从环境变量里取连接串裸连数据库；本地调试也走这个 shortcut。写什么样的 SQL（平台约束、建表模板、`user_profile`、审计列、禁用 SQL、PG 陷阱）见文末「平台 SQL 规范」。

## 命令骨架

- 必填：`--app-id`，以及 `--sql` / `--file` 二选一（互斥）。
- `--sql`：内联 SQL 文本；传 `-` 时从 stdin 读。绝对路径文件经 stdin 传入：`--sql - < <absolute-path>`（shell 解析路径，CLI 仅接收内容）。
- `--file`：`.sql` 文件路径，需为工作目录内的相对路径（如 `--file ./migration.sql`）；绝对路径、或经 `..`/符号链接越出工作目录的路径会被拒绝。文件不在工作目录内时，改用 `--sql - < <文件路径>` 经 stdin 传入。
- `--environment` 枚举：`dev` / `online`，**不传则由服务端按应用是否开启多环境自动选择（多环境→`dev`，未开启多环境→`online`）**；要固定环境就显式传 `--environment dev|online`。**未开启多环境的应用显式传 `--environment dev` 会报错（无 dev 分支）——这类应用不传 `--environment`（走 `online`）或显式 `--environment online`**。旧名 `--env` 已**移除**：传入会报 validation 错（提示改用 `--environment`），一律用 `--environment`。
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

---

# 平台 SQL 规范

上面讲命令怎么调，这里讲**该写出什么样的 SQL**：妙搭底层是 PostgreSQL + 一层平台约束（RLS、审计列、`user_profile` 复合类型、禁用 SQL 白名单），不符合会被服务端直接拒或建出行为不对的表。看表 / 看结构用 [`+db-table-list`/`+db-table-get`](lark-apps-db.md)，别手写系统表查询模拟。

## 平台禁用 SQL（硬拒绝）

以下命中会被服务端拒，`error`（`type:"api"`）的 message/hint 会说明原因——先按 hint 修再重试，不要反复重试同一句。

| 类别 | 禁止 |
|---|---|
| 数据库级 | `CREATE / DROP / ALTER DATABASE` |
| Schema 级 | `CREATE / DROP SCHEMA` |
| 用户 / 角色级 | `CREATE / DROP USER`、`CREATE / DROP / ALTER ROLE` |
| Owner 切换 | `REASSIGN OWNED` / `DROP OWNED` |

## 建表规范（CREATE TABLE）

新建业务表必须：4 个审计列 + 启用 RLS + 4 条默认 policy，**放在同一次 `+db-execute` 调用里**（RLS / policy / COMMENT / INDEX 一起）。裸表名，不写 `public.` 或 schema 前缀。

```sql
CREATE TABLE IF NOT EXISTS <table> (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  -- ... 业务列 ...
  name varchar(100) NOT NULL,
  _created_at TIMESTAMP(3) WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  _created_by user_profile DEFAULT (
    CASE
      WHEN current_setting('app.user_id', TRUE) = '' THEN NULL
      ELSE concat('(', current_setting('app.user_id', TRUE), ')')::user_profile
    END
  ),
  _updated_at TIMESTAMP(3) WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  _updated_by user_profile DEFAULT (
    CASE
      WHEN current_setting('app.user_id', TRUE) = '' THEN NULL
      ELSE concat('(', current_setting('app.user_id', TRUE), ')')::user_profile
    END
  )
);

ALTER TABLE <table> ENABLE ROW LEVEL SECURITY;

CREATE POLICY service_role_bypass_policy ON <table>
  TO service_role USING (true);

CREATE POLICY "修改全部数据" ON <table>
  AS PERMISSIVE FOR ALL TO authenticated USING (true);

CREATE POLICY "查看全部数据" ON <table>
  AS PERMISSIVE FOR SELECT TO authenticated, anon USING (true);

CREATE POLICY "修改本人数据" ON <table>
  AS PERMISSIVE FOR ALL TO authenticated USING (
    (current_setting('app.user_id'::text) = ANY (ARRAY[]::text[]))
    AND (current_setting('app.user_id'::text) = ((_created_by).user_id)::text)
  );
```

建表流程：先 `+db-table-list` / `+db-table-get` 确认表不存在或看现有结构 → 生成 DDL → 向用户展示影响并取得授权 → `+db-execute ... --yes` 执行。

## 审计列

- 平台自动维护的四列固定叫 `_created_at` / `_updated_at` / `_created_by` / `_updated_by`（**下划线开头**）。查询 / 排序 / 过滤一律用这些名字，别写 `created_at`。
- `_created_at` / `_updated_at` 在 INSERT 时可省略（有默认值）；需要业务归属时显式写 `_created_by` / `_updated_by`。
- UPDATE 业务字段时建议同步 `_updated_at = CURRENT_TIMESTAMP` 和 `_updated_by`。

## `user_profile` 复合类型

平台内置类型 `(user_id varchar, name varchar, email varchar, avatar text, status integer)`，无需创建。**业务 SQL 只允许访问 `(field).user_id`**，不要依赖 `name` / `email` / `avatar` / `status`（可能为空或过期）。

```sql
-- 写入 / 更新：用 ROW()::user_profile，更新时替换整个字段，不改单个属性
INSERT INTO teacher (teacher_profile, class_id)
VALUES (ROW('<user_id>')::user_profile, gen_random_uuid());

UPDATE teacher SET teacher_profile = ROW('<user_id>')::user_profile
WHERE (teacher_profile).user_id = '<old_user_id>';

-- 查询 / 过滤：解引用取 user_id；raw SQL 返回给前端前必须解引用，别直接返回复合类型
SELECT (teacher_profile).user_id AS teacher_profile, class_id FROM teacher;

-- 索引 / 唯一性：表达式列用三重括号；表达式唯一性用 CREATE UNIQUE INDEX，
-- 不能用 ALTER TABLE ADD CONSTRAINT UNIQUE（不支持表达式列）
CREATE INDEX idx_teacher_user_id ON teacher (((teacher_profile).user_id));
CREATE UNIQUE INDEX uk_teacher_user_id ON teacher (((teacher_profile).user_id));
```

## DDL 规则

| 场景 | 做法 |
|---|---|
| 加列 | `ALTER TABLE <t> ADD COLUMN IF NOT EXISTS <col> <type>`，相关 `COMMENT ON` 同次执行 |
| 加索引 | `CREATE INDEX IF NOT EXISTS idx_<t>_<cols> ON <t>(...)` |
| JSONB 类型声明 | 必须 `COMMENT ON COLUMN <t>.<col> IS '@type { ... }'` 声明 TypeScript 类型，和 CREATE / ALTER 同次调用 |
| 加 NOT NULL 列 | 必须带 `DEFAULT` 让存量行自动填：`ADD COLUMN <col> <type> NOT NULL DEFAULT <值>` |
| 删表 / 删列 | 有业务数据默认禁止；必须用户明确授权后才执行，并说明数据丢失风险 |
| 强约束 | `UNIQUE` / `FOREIGN KEY` / `NOT NULL` 默认谨慎，不确定不加 |

**多环境库加约束前先查 online 存量**：`dev` 干净不代表 `online` 干净，约束发布到 online 会撞线上存量数据而失败。发布前一律先用 `--environment online` 查清楚，按约束类型分三种：

- **加唯一约束（`UNIQUE` / 唯一索引）**：线上不能有重复值。先查重复，有则先清理再加：

  ```bash
  lark-cli apps +db-execute --app-id app_xxx --environment online --sql \
    "SELECT <cols>, count(*) FROM t GROUP BY <cols> HAVING count(*) > 1" --yes
  ```

- **已有列改 `NOT NULL`（收紧约束）**：线上该列不能有 NULL。先查 NULL 行数，有就先回填（`UPDATE t SET <col> = <默认值> WHERE <col> IS NULL`）再加约束：

  ```bash
  lark-cli apps +db-execute --app-id app_xxx --environment online --sql \
    "SELECT count(*) FROM t WHERE <col> IS NULL" --yes
  ```

- **新加 `NOT NULL` 字段**：必须带 `DEFAULT`，且要求线上该表**无存量数据**，否则发布报错。线上已有数据时别直接加，改走三步安全变更：先 `ADD COLUMN <col> <type>`（可空）→ 回填 `UPDATE t SET <col> = <值>` → 再 `ALTER COLUMN <col> SET NOT NULL`。先查线上行数判断走哪条：

  ```bash
  lark-cli apps +db-execute --app-id app_xxx --environment online --sql \
    "SELECT count(*) FROM t" --yes
  ```

## SELECT 规则

| 规则 | 要求                                                               |
|---|------------------------------------------------------------------|
| 行数 | 结果集有硬上限（平台限制 1000 行），超限**报错而非静默截断**；大表必须显式 `LIMIT`、聚合或游标分页       |
| 分页 | 大表优先游标分页 `WHERE id > <last_id> ORDER BY id LIMIT n`，避免大 `OFFSET` |
| user_profile | 返回给前端前解引用：`(owner).user_id AS owner`                             |
| 统计 | 总数用 `count(*)`、分组用 `GROUP BY`，别把全量拉到 agent 侧再统计                  |
| 慢查询 | 用 `EXPLAIN (ANALYZE, BUFFERS)`；大表 Seq Scan 考虑加索引                 |

## DML 规则

**INSERT**
- UUID 主键省略，交给 `DEFAULT gen_random_uuid()`；外键 UUID 用子查询取父表 id，不手写。
- NOT NULL 且无默认值的列必须给值；批量 INSERT 每行列数一致。
- 需要幂等用 `ON CONFLICT ... DO NOTHING / DO UPDATE`。
- 标量子查询必须保证单行，非唯一条件加 `ORDER BY ... LIMIT 1`。

**UPDATE**
- **必须有明确 `WHERE`，禁止无条件 UPDATE**。
- 用户说「修改 / 更新 / 改一下」数据时用 UPDATE，**禁止 DELETE + INSERT** 模式。
- 更新 `user_profile` / 复合类型时替换整个字段。
- 批量更新前影响范围不明确，先 `SELECT count(*)` 给用户确认。

**DELETE / TRUNCATE**（属会丢数据的高影响操作，按上面「Agent 规则」的确认流程走）
- 已有表 / 已有数据默认禁止；先 `SELECT count(*)` 展示命中行数、取得用户明确授权，再带 `--yes` 执行。
- `TRUNCATE` 影响整表，视同高风险删除。

```sql
UPDATE task
SET status = 'done', _updated_at = CURRENT_TIMESTAMP, _updated_by = ROW('<user_id>')::user_profile
WHERE id = (SELECT id FROM task WHERE title = '梳理需求' ORDER BY _created_at DESC LIMIT 1);
```

## 常见 PostgreSQL 陷阱

| 陷阱 | 正确做法 |
|---|---|
| 表名带 schema 前缀 | 业务表一律裸表名 `FROM orders`，别写 `public.orders` |
| 保留字作标识符 | 避免 `user` / `order` / `desc` / `offset` / `references` 等 |
| 内联 COMMENT | 禁止 `col TEXT COMMENT 'xx'`，用独立 `COMMENT ON COLUMN` |
| 手写系统表查结构 | 常规结构查询用 `+db-table-list` / `+db-table-get`，别手写 `information_schema` / `pg_indexes` 模拟 |
| 空数组类型不明 | 写 `ARRAY[]::text[]` 或 `'{}'::text[]` |
| `ROUND` 报错 | 用 `ROUND(num::numeric, n)` 或 `ROUND(num::double precision)` |
| `DISTINCT` + 窗口函数 | 分两层查询，先 DISTINCT 再窗口函数 |
| MySQL 方言 | 不用 `SHOW TABLES` / `DESCRIBE` / 内联 `COMMENT`；用 `+db-table-*` 和 `COMMENT ON` |
| 多语句以为自动回滚 | `A; B; C` 不自动包事务，B 失败时 A 已提交；要原子性显式 `BEGIN; ... COMMIT;`（见上「命令骨架」「Agent 规则」） |

## 数据类型与设计

| 项目 | 规则 |
|---|---|
| 主键 | 默认 `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` |
| 命名 | 表名单数、全小写、snake_case、无冗余后缀 |
| 枚举 / 状态 | 用 `varchar(255)`，值用小写英文 + 下划线 |
| JSONB | 必须 `COMMENT ON COLUMN ... IS '@type { ... }'` 声明类型 |
| 附件 / 图片 | URL 用 `TEXT`，命名 `xxx_url` |
| 约束 | `UNIQUE` / `FOREIGN KEY` / `NOT NULL` 默认谨慎，新增 NOT NULL 列优先带 `DEFAULT` |
