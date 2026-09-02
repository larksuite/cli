# apps db 域命令

管理妙搭应用数据库：看表与结构、初始化与发布多环境、数据搬运、变更治理、时间点恢复、用量。逐条跑 SQL（SELECT/DML/DDL）走 [`+db-execute`](lark-apps-db-execute.md)（单独一篇）。运行时命令事实以 `lark-cli apps +<cmd> --help` 为准；认证、`--as user`、exit 码、`_notice` 等通用处理见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 与本域 [`SKILL.md`](../SKILL.md)。

## 何时用

用户要看应用里有哪些表 / 某张表的结构、把单库应用拆成 dev/online 多环境、把数据导进导出表、查谁在什么时候改了表结构或表数据、开关行级审计、把开发环境的库结构发布到线上、把库恢复到过去某个时间点、或看数据库用量时。逐条执行 SQL 走 [`+db-execute`](lark-apps-db-execute.md)；文件存储（上传/下载文件）走 [`lark-apps-file.md`](lark-apps-file.md)。**建表 / 改表 / 写 SQL 的平台内容规范**（审计列、RLS、`user_profile`、禁用 SQL、PG 陷阱）见 [`lark-apps-db-execute.md`](lark-apps-db-execute.md) 的「平台 SQL 规范」。

## 命令一览

| 命令 | 做什么 | 关键参数 |
|---|---|---|
| `+db-table-list` | 列出某环境的数据表 | `--environment`、`--page-size`/`--page-token` |
| `+db-table-get` | 看单张表的结构（字段/索引/约束/DDL） | `--table`、`--environment`、`--format` |
| `+db-env-create` | 把单库应用初始化为 dev/online 多环境（高危） | `--environment`、`--sync-data`、`--yes` |
| `+db-data-export` | 把一张表的数据导出到本地文件 | `--table`、`--output`、`--limit`、`--environment` |
| `+db-data-import` | 把本地 csv/json 文件导进一张表（高危） | `--file`、`--table`、`--environment`、`--yes` |
| `+db-sync-create` | 预览或创建 Base 到应用数据库的同步任务（高危） | `--config`、`--preview`、`--output`、`--environment`、`--yes` |
| `+db-sync-list` | 列出 Base 同步任务 | `--mode`、`--status`、`--table`、`--page-size`/`--page-token`、`--environment` |
| `+db-sync-get` | 查看同步任务配置、状态、统计和 warnings | `--task-id` |
| `+db-sync-enable` | 启用 streaming 同步任务 | `--task-id` |
| `+db-sync-disable` | 停用 streaming 同步任务 | `--task-id` |
| `+db-sync-update` | 修改 streaming 同步任务映射配置（高危） | `--task-id`、`--config`、`--yes` |
| `+db-sync-delete` | 删除 streaming 同步任务，保留目标数据（高危） | `--task-id`、`--yes` |
| `+db-changelog-list` | 查表结构变更（DDL）历史 | `--table`、`--change-id`、`--since`/`--until`、`--environment` |
| `+db-audit-status` | 看哪些表开了行级审计、保留期 | `--table`、`--environment` |
| `+db-audit-enable` | 给某表开启行级变更审计 | `--table`、`--retention`、`--environment` |
| `+db-audit-disable` | 关闭某表的行级审计 | `--table`、`--environment` |
| `+db-audit-list` | 列出表的行级变更事件（增删改追溯） | `--table`（可重复）、`--since`/`--until`、`--environment` |
| `+db-env-diff` | 预览开发环境待发布到线上的结构变更 | `--app-id` |
| `+db-env-migrate` | 把开发环境的结构变更发布到线上（高危） | `--app-id`、`--yes` |
| `+db-recovery-diff` | 预览把库恢复到某时间点会带来的变更 | `--target` |
| `+db-recovery-apply` | 把库恢复到某个时间点、覆盖当前数据（高危） | `--target`、`--yes` |
| `+db-quota-get` | 查数据库存储用量 | `--environment` |

## 约定（先读）

- **环境 `--environment dev|online`（可省略）**：看表、看结构、数据导入导出、变更追溯、审计、配额都按环境区分。省略 `--environment` 时 CLI 不带该参数、由服务端按应用形态自动选分支——多环境应用走 `dev`、未开多环境的走 `online`；要固定环境就显式传。唯一会报错的组合：对未开多环境的应用显式传 `--environment dev`（无 `dev` 分支）。写操作建议先在 `dev` 验（仅多环境应用有 `dev`）。旧名 `--env` 已**移除**：传入会报 validation 错（提示改用 `--environment`），一律用 `--environment`。`+db-env-diff`/`+db-env-migrate` 是「dev→online 发布」语义，**没有** `--environment`。
- **本地文件 / `--output` 用工作目录内相对路径**：导入 `--file ./orders.csv`、导出 `--output ./out.csv`；绝对路径、或经 `..`/符号链接越出工作目录的 `--output` 会被拒（validation / exit 2）。路径在别处先 `cd` 过去或改成相对路径。
- **高危操作必须带 `--yes`**：`+db-env-create`、`+db-data-import`、`+db-env-migrate`、`+db-recovery-apply`、`+db-sync-create`、`+db-sync-update`、`+db-sync-delete` 缺省会被确认关卡拦下；动手前先用对应的预览命令或 `--dry-run` 看清影响。`+db-sync-create --preview` 只解析/校验配置、不落库，免确认、不需 `--yes`；真正建任务（不带 `--preview`）才需要 `--yes`。
- **Base 同步不是整库任务**：`+db-sync-create` 一次只处理一张 Base 表到一张目标表。用户说“整库”“客户、订单、回款三张表都同步”时，先明确告诉用户会拆成三套独立配置、三次 preview、用户确认后三次 create；不要暗示一个同步任务能覆盖整个 Base。
- **batch 任务不能重新启用**：用户说“批量任务重新启用”“operation-not-allowed”时，先给结论：batch/import 是一次性任务，不能 enable。不要先陷入授权排障而漏掉这个结论；授权缺失时也要说明授权完成后应 `+db-sync-get` 查状态/结果，持续同步要新建 streaming。
- **时间参数按口语自然传**（`--since`/`--until`/`--target`），格式见末尾。

## 各命令

### 表与结构

**`+db-table-list`**：列出某环境的数据表。分页 `--page-size`（默认 20）/ `--page-token`（上一页 cursor）。每项给表名、描述、估算行数、大小、列数；要完整列定义 / 索引 / 约束用 `+db-table-get`。只知道业务对象名时，先用它定位可能的表名。

```bash
lark-cli apps +db-table-list --app-id app_xxx
lark-cli apps +db-table-list --app-id app_xxx --environment dev --page-size 50
```

**`+db-table-get`**：看单张表的结构。默认 JSON 给结构化的字段 / 索引 / 约束 / 估算行数 / 大小；`--format pretty` 直接输出建表 DDL 文本（给用户看建表语句或做迁移参照时用）。

```bash
lark-cli apps +db-table-get --app-id app_xxx --table orders
lark-cli apps +db-table-get --app-id app_xxx --table orders --environment dev --format pretty
```

### 多环境数据库（初始化 + 发布）

**`+db-env-create`（高危）**：把存量单库应用初始化为 dev/online 两套库，不可逆，必须带 `--yes`。`--environment` 目前只支持 `dev`（默认 `dev`）；`--sync-data` 把现有 online 数据复制到新环境（不传则不复制）。注意：`+create --app-type full_stack` 新建的应用通常已自带多环境，重复初始化会返回冲突错误（应用已是多环境）——按 `error.hint` 转述状态即可，别重复初始化。

```bash
lark-cli apps +db-env-create --app-id app_xxx --environment dev --dry-run
lark-cli apps +db-env-create --app-id app_xxx --environment dev --sync-data --yes
```

**`+db-env-diff`**：预览开发环境里待发布到线上的表结构变更，不落地。发布前先看这个。无待发布变更时明确返回「无变更」。

**`+db-env-migrate`（高危）**：把开发环境的结构变更正式发布到线上，不可逆，必须带 `--yes`，返回实际发布的变更条数。发布是异步的，命令会等到完成再返回结果。

> 预览与发布同一端点，故 `+db-env-diff` 也需 `spark:app:write` scope（不是纯只读权限）。

```bash
lark-cli apps +db-env-diff --app-id app_xxx
lark-cli apps +db-env-migrate --app-id app_xxx --yes
```

### 数据导入导出

**`+db-data-export`**：把一张表导出到本地文件。导出格式**只由 `--output` 的扩展名决定**——`.csv` / `.json` / `.sql`，缺省按 `<表名>.csv` 落在当前目录。注意：全局 `--format json|pretty` 只控制**命令自身输出**（成功摘要 / 错误信封）的渲染，**不影响导出文件的格式**；`--output` 后缀必须是 `.csv/.json/.sql` 之一，否则报 validation 错误（exit 2），且不支持导出到 stdout。两道体量约束：

- `--limit`（1..5000，默认 5000）是**行数上限守卫**：表的行数超过它会被整体拒掉（不是「只导前 N 行」）；
- 导出产物 >1 MB 也会被拒。

超大表别硬导：先用 `+db-execute` 加 `WHERE` / `LIMIT` 缩小范围、分批导。

```bash
lark-cli apps +db-data-export --app-id app_xxx --table orders --output ./orders.csv
lark-cli apps +db-data-export --app-id app_xxx --table orders --output ./orders.json --environment dev
```

**`+db-data-import`（高危）**：把本地 csv/json 文件的数据导进表。文件需是 `.csv`/`.json`、≤1 MB，必须带 `--yes`。目标表缺省取文件名去掉**最后一个**扩展名（如 `orders.csv`→`orders`，`orders.2026.csv`→`orders.2026`）；文件名带点号时建议显式传 `--table` 以免落到意外的表名。

```bash
lark-cli apps +db-data-import --app-id app_xxx --table orders --file ./orders.csv --environment dev --yes
```

**导入/导出限额**：体积 ≤ **1 MB**、行数 ≤ **5000**，导入导出都一样，超限会被拒。超限就分批——导入拆成 ≤1 MB / ≤5000 行的多个文件，导出用 `WHERE` / `LIMIT` 缩小范围。

### Base 数据同步

Base 数据同步走 `+db-sync-*`，和本地文件导入不同：`+db-data-import` 只处理本地 `.csv/.json` 文件；Base 链接、Base 表、字段映射、持续同步任务都走 `+db-sync-create` / `+db-sync-update`。

**任务类型**：
- `mode=batch`：一次性任务。`schema_only=true` 只建目标表；`schema_only=false` 建表或写入已有表并导入当前 Base 数据。完成后不能 enable/disable/update/delete。
- `mode=streaming`：持续同步任务。首次同步后持续处理 Base 变化，可 enable/disable/update/delete。

**环境（重要）**：`+db-sync-*` 命令省略 `--environment` 时默认落 **online**（不同于 `+db-table-*`/`+db-audit-*` 等「多环境自动选 dev、单环境选 online」的规则——db-sync 家族不走自动选分支）。**多环境应用建表**（`target.table.action=create`）**必须显式 `--environment dev`**：不填或填 `online` 会被 online 分支的 DDL 禁令拒（`k_dl_4000001：forbid ddl/dcl operation in online env`），因为 online 分支产品上不允许直接建表，建表要落到 dev 分支。共享库 / 单环境应用只有 online、在 online 建表正常成功（不会报 `k_dl_4000001`），省略 `--environment` 或填 `online` 均可。

**配置格式**：只通过 `--config` 传完整 JSON，支持内联 JSON、`@file`、`-` stdin。配置 key 使用复数：`field_maps`、`option_mappings`、`syncable_source_fields`。不要写单数 `field_map` / `option_mapping`，CLI 会直接报 validation 错。正式 create 时 `field_maps` **可省略或传空数组**：服务端会使用与 preview 相同的逻辑自动匹配字段并直接创建任务；若显式传了映射，则至少要有一项未写成 `"enabled": false`，写了却全部关闭会被 CLI 拒绝。`+db-sync-update` 仍要求至少一个启用的 `field_maps`，因为 update 的语义是修改既有映射。`target.table.action` 只能是 `create` 或 `use_existing`：建表时 `pg_field` 需要完整字段定义；写已有表时通常只需目标列名。`source.base_url`（源 Base 表完整 URL）在 `+db-sync-create` 必填、由服务端强制；`+db-sync-update` 可选——省略时服务端复用原任务的源 URL，仅在换源 / 替换成另一张 Base 表时才需要传新的 `base_url`。`source.table.name` 是要同步的 Base 表名。`base_url` 形如 `https://.../base/<token>?table=<tableId>`：`token` 定位 Base，`table=` 参数（tableId）定位表。填了 `source.table.name` 就以 name 为准——服务端用 `token + name` 反查 tableId（覆盖 url 里的 `table=` 参数）；不填才用 url 的 `table=` 参数定位。所以用户自然语言里说「同步 xxx 表」「把 xxx 表同步过去」时，一定要把「xxx」填进 `source.table.name`，不要只给 `base_url`——尤其当 `base_url` 不带 `table=` 参数（指向不带具体表的 Base）时，漏了 name 服务端无从定位表。

**不知道要同步哪张表**：若 `base_url` 只有域名+token、不带 `?table=` 参数，又不确定表名，别硬猜。先用 `lark-cli base +table-list --base-token <token>` 列出该 Base 的所有表（`<token>` 就是 `base_url` 里 `/base/` 后面那段），把表名给用户选定，再填进 `source.table.name`（或改用带 `?table=<table_id>` 的完整 URL）。`+db-sync-create` 会在本地就拦下「`base_url` 无 `?table=` 且 `source.table.name` 空」的配置（提交前即报 validation 错，不送到服务端）。

**单数 key 恢复**：如果用户说配置里 `field_map` 是单数、`option_mapping` 是单数、或字段映射可能不生效，不要把原配置直接提交。先找到用户这份同步配置，做这三步：

1. 只把已知 key 改成复数：`field_map` -> `field_maps`，`option_mapping` -> `option_mappings`；不要发明 `fieldMappings` / `mapping` 之类字段名。
2. 检查 `field_maps` 是数组，且至少有一项 `enabled` 缺省或为 `true`。如果全是 `"enabled": false`，先让用户确认要启用哪几项，再继续。
3. 修好后先重新 preview，或复用最近一次 preview `--output` 产出的 `data.config`，再继续 create / update。

```bash
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @sync.json --preview --output ./resolved-sync.json
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @resolved-sync.json --yes
```

如果本地找不到配置文件，不要只停在“请提供文件”。先说明恢复来源：让用户贴失败时传入的 JSON，或查找最近 preview 的 `--output` 文件；如果是已有任务的修改，先用 `+db-sync-get` 取回当前任务配置，再基于它修正后 update：

```bash
lark-cli apps +db-sync-get --app-id app_xxx --task-id streaming_123 -q '.data | {mode, source, target, field_maps}' > sync.json
lark-cli apps +db-sync-update --app-id app_xxx --task-id streaming_123 --environment dev --config @sync.json --yes
```

**推荐流程（最佳实践，不是强制）**：优先先 preview，再让用户确认映射，最后用 preview 输出的完整 config 正式创建；这样最稳，也避免手写复杂 `field_maps`。若用户明确要求直接执行、不需要 preview，也可以在 create config 中省略 `field_maps`（或传空数组），由服务端自动匹配并直接创建任务；CLI 不应为了拿 mapping 强制用户先 preview。

```bash
lark-cli apps +db-sync-create \
  --app-id app_xxx \
  --environment dev \
  --config - \
  --preview \
  --output ./resolved-sync.json <<'JSON'
{
  "mode": "streaming",
  "source": {
    "type": "base",
    "base_url": "https://example.feishu.cn/base/xxx",
    "table": {"name": "客户"}
  },
  "target": {
    "type": "postgresql",
    "table": {"name": "customers", "action": "use_existing"}
  }
}
JSON
```

preview 返回 `data.config`、`syncable_source_fields` 和 `summary`。`--output` 只把 `data.config` 写入文件，文件可直接作为正式输入：

```bash
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @resolved-sync.json --yes
lark-cli apps +db-sync-get --app-id app_xxx --task-id streaming_123
```

**多表 Base**：本命令一次只处理一张表。用户要同步整个 Base 时，先把计划说清楚：不是一个“整库同步任务”，而是按表拆成 N 个单表任务。每张表各有一份配置文件、一次 `+db-sync-create --preview`、一次用户确认后的 `+db-sync-create --yes`，并记录各自 `task_id`。

```bash
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @customers-sync.json --preview --output ./customers-resolved.json
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @orders-sync.json --preview --output ./orders-resolved.json
lark-cli apps +db-sync-create --app-id app_xxx --environment dev --config @payments-sync.json --preview --output ./payments-resolved.json
```

配置也必须是单表粒度：每份 JSON 只有一个 `source.table` 和一个 `target.table`，字段名保持 `field_maps`、`option_mappings`、`syncable_source_fields` 这些复数 key。

**修改 streaming 映射**：先用 get 导出当前配置，编辑 `field_maps` 后 update。update 是高危操作，必须经用户确认再加 `--yes`。

`+db-sync-get` 返回的 `source` **不含 `base_url`**（只有 token / tableId，服务端没有 domain 拼不出完整 URL），这是正常的。原表 update 直接省略 `base_url` 即可；只有要换成另一张 Base 表时，才在 config 里显式补一个新的 `base_url`。不要为了"补全" `base_url` 而编造 domain 或拼接 URL——拿不到就省略，让服务端复用原任务的源 URL。

`+db-sync-update` 也遵循 db-sync 家族「省略 `--environment` 落 online」的规则，所以改 dev 上的任务必须显式带该任务所在环境的 `--environment`（多环境应用的 streaming 任务通常在 `dev`），否则会错落 online、找不到任务或改错分支。

```bash
lark-cli apps +db-sync-get --app-id app_xxx --task-id streaming_123 -q '.data | {mode, source, target, field_maps}' > sync.json
lark-cli apps +db-sync-update --app-id app_xxx --task-id streaming_123 --environment dev --config @sync.json --yes
```

**列表与生命周期**：

```bash
lark-cli apps +db-sync-list --app-id app_xxx --mode streaming --table customers
lark-cli apps +db-sync-disable --app-id app_xxx --task-id streaming_123
lark-cli apps +db-sync-enable --app-id app_xxx --task-id streaming_123
lark-cli apps +db-sync-delete --app-id app_xxx --task-id streaming_123 --yes
```

`+db-sync-enable`、`+db-sync-disable`、`+db-sync-update`、`+db-sync-delete` 只适用于 `streaming_...` task。对 `batch_...` 执行这些操作会返回 failed-precondition。

**batch 任务 operation-not-allowed 恢复**：用户说“批量任务重新启用”“导入历史订单表的任务重新 enable”“系统说操作不允许”时，先给生命周期结论：batch / import 类任务是一次性任务，完成或失败后不能重新启用，也不要反复调用 `+db-sync-enable`。下一步改为查状态和结果：

```bash
lark-cli apps +db-sync-get --app-id app_xxx --task-id batch_123
```

把 `status`、`result`、`warnings` 和目标表写入情况告诉用户。若用户要的是后续持续同步，不是“重启这个 batch”，应新建 `mode=streaming` 任务：先 `+db-sync-create --preview` 给用户确认映射和影响，再带 `--yes` 创建；不要强行 enable 已完成的 batch 任务。若此时 CLI 还缺授权，仍要先解释这个生命周期边界，再提示授权完成后用 `+db-sync-get` 查结果。

**失败恢复**：看到 `warnings` 不要直接说同步成功。按 warning 或 error 的 `hint` 继续排查，恢复路径按任务 mode 分支：

- **streaming 任务**：常见路径是 `+log-list --keyword <target_table>` / `+log-get` 查日志，然后用 `+db-execute` 修目标表结构，或用 `+db-sync-update`（带该任务所在环境的 `--environment`）修字段映射，最后对同一 `task_id` 再 `+db-sync-get` 复查。
- **batch 任务**：batch 是一次性任务、**不能 update**（见上文生命周期）。修完目标表结构（`+db-execute`）后不要 update 原 batch，而是重新 `+db-sync-create --preview` 建新任务；只想看这个 batch 的结果就直接 `+db-sync-get`。

若此时 CLI 还缺授权、查不到 warning 详情，也不要只给泛化的字段核对建议：先说明被授权卡住，再把对应 mode 的固定命令链作为授权完成后的下一步明确交代给用户。

**online 禁 DDL（`k_dl_4000001`）恢复**：`+db-sync-create` 建表报 `k_dl_4000001：forbid ddl/dcl operation in online env` 时，这必然是多环境应用（共享库在 online 建表不会报此码）。online 分支**本就不允许**直接建表，这是多环境应用的产品设计、不是可绕过的限制。改用 `--environment dev` 重跑 `+db-sync-create`，把表建到 dev 分支；不要试图「在 online 想办法重试建表」，没有这个选项。

**缺 Base 表记录 ID 映射列（`400002477`）恢复**：streaming 自动同步要求目标表有一个映射给「Base 表记录 ID」的 **text + 单值 + unique** 列。用 `action=use_existing` 写已有表时，若该表没有这样的列，会报 `400002477`（Field mapping must include 'Base 表记录 ID'）。先用 `+db-execute` 给表加一个，如 `ALTER TABLE <表> ADD COLUMN base_record_id varchar UNIQUE`，再把它映射给「Base 表记录 ID」、重跑 `+db-sync-create --preview`。注意这是**加列**、不是建表，不需要审计列 / RLS 那套建表规范。

### 变更追溯与审计

**`+db-changelog-list`**：查表结构变更（DDL）历史——谁、什么时候、改了哪张表、做了什么。可按 `--table` 过滤、按 `--change-id` 精确定位某条、用 `--since`/`--until` 圈时间区间，分页 `--page-size`/`--page-token`。

```bash
lark-cli apps +db-changelog-list --app-id app_xxx --table orders --since 7d
```

**`+db-audit-status`**：看审计开关状态。给 `--table` 看单表，不给则列出所有已配置的表（开没开、保留期）。

**`+db-audit-enable` / `+db-audit-disable`**：开 / 关某张表的行级变更审计。`--retention` 设保留期，取值 `7d`/`30d`/`180d`/`360d`/`forever`（默认 `7d`）。不要对已经开启审计的表重复 enable——不确定就先用 `+db-audit-status` 查。

```bash
lark-cli apps +db-audit-enable --app-id app_xxx --table orders --retention 30d
lark-cli apps +db-audit-disable --app-id app_xxx --table orders
```

**`+db-audit-list`**：列出表的行级变更事件（INSERT/UPDATE/DELETE 的前后值与操作人）。`--table` 必填、可重复传多张表；`--since`/`--until` 圈时间。
- **多表查询**：会先帮用户把不存在、或没开审计的表过滤掉再查，被过滤的表及原因列在结果的 `skipped` 里——据此告诉用户哪些表没纳入及为什么。
- **单表查询**：不预过滤，表不存在 / 未开审计会直接报错（按 `error.hint` 转述给用户，引导先 `+db-audit-enable`）。

```bash
lark-cli apps +db-audit-list --app-id app_xxx --table orders --since 24h
lark-cli apps +db-audit-list --app-id app_xxx --table orders --table users
```

### 时间点恢复（PITR）

**`+db-recovery-diff`**：预览把库恢复到 `--target` 时间点会带来哪些变更（受影响的表、行数、预计耗时），不落地。同样需 `spark:app:write` scope。

**`+db-recovery-apply`（高危）**：把库恢复到某个时间点，**会覆盖当前数据**，不可逆，必须带 `--yes`。

- 可恢复窗口最长 **7 天**，且不早于**最近一次 `+db-env-migrate`**；超出窗口的目标会被拒。
- 目标时间点与当前库一致时返回 `no_changes`（空操作），不算失败。
- 动手前务必先 `+db-recovery-diff` 给用户确认。

```bash
lark-cli apps +db-recovery-diff --app-id app_xxx --target 2h
lark-cli apps +db-recovery-apply --app-id app_xxx --target 2026-04-15T10:00:00Z --yes
```

### 配额

**`+db-quota-get`**：查数据库存储用量（已用量、表数、视图数；配额接入后还会给总配额与使用率）。

```bash
lark-cli apps +db-quota-get --app-id app_xxx --environment dev
```

## 时间格式（`--since` / `--until` / `--target`）

按用户口语自然传入即可，支持：
- 相对时间 `7d` / `2h` / `30s`（从现在往前推）
- 日期 `2026-04-15`
- 日期时间 `2026-04-15T10:00:00`
- 带时区的 ISO 8601 `2026-04-15T10:00:00Z` / `2026-04-15T10:00:00+08:00`

> **时区**：不带时区的 `日期` / `日期时间` 按**运行机器的本地时区**解析（再归一化到 UTC）。CI（UTC）与本地（如 UTC+8）跑同一条命令，时间边界会差几小时；要精确锁定时区时显式写 ISO 8601 带偏移（如 `...+08:00` / `...Z`）。`--target`（PITR 恢复）尤其建议带时区，避免恢复到非预期时间点。

## Agent 规则

- 用户说「本地 / 开发库 / 调试库」优先 `--environment dev`，线上排查用 `--environment online`；数据面写操作（导入 / 审计开关）建议先在 `dev` 验再动 `online`。**注意省略 `--environment` 时写操作会落到服务端选中的分支——单环境应用即 `online`（生产）**：不确定应用是否多环境时，写操作显式传 `--environment`；显式 `dev` 在单环境应用上会安全报错（无 dev 分支），正好当「是否多环境」的探针用。
- 看表用 `+db-table-list`，看结构用 `+db-table-get`（要建表语句加 `--format pretty`）；`+db-env-create` 仅用于存量单库拆多环境，新建的 full_stack 应用一般不需要。
- 高危命令（`+db-env-create`、`+db-data-import`、`+db-env-migrate`、`+db-recovery-apply`、`+db-sync-create`、`+db-sync-update`、`+db-sync-delete`）动手前先看清影响再带 `--yes`：发布 / 恢复先跑对应预览 `+db-env-diff` / `+db-recovery-diff`，Base 同步先跑 `+db-sync-create --preview`，导入无预览命令、可先 `--dry-run` 看请求或先在 `--environment dev` 验；不要静默追加 `--yes`，遇 confirmation_required（exit 10）按 lark-shared 协议向用户确认不可逆风险后再补 `--yes` 重试。
- 导入 / 导出的本地路径用工作目录内相对路径；超大表导出会被行数 / 体积上限拒，改用 `+db-execute` 分批。
- Base 同步优先走 preview → 用户确认 → create，这是最稳的最佳实践、不是强制。用户明确要求直接 create 时，可省略 `field_maps`（或传空数组）让服务端自动匹配并创建；不要为了拿 mapping 强制用户先 preview。显式写映射时使用 `field_maps` / `option_mappings` 复数 key。
- 修复 Base 同步配置时，只把 `field_map` / `option_mapping` 改成 `field_maps` / `option_mappings`。若显式给了 `field_maps`，检查至少一个映射启用；全是 `"enabled": false` 时先让用户确认要启用哪项。create 也可删掉/置空 `field_maps` 交给服务端自动匹配，但 update 仍必须提供启用的映射。
- `+db-sync-update` 省略 `source.base_url` 是合法的（服务端复用原任务源 URL）；`+db-sync-get` 不返回 `base_url` 属正常，不要因此编造 domain / 拼接 URL 去"补全"，只有换源 / 替换表时才传新的 `base_url`。`+db-sync-create` 的 `base_url` 必填，缺失由服务端报错。用户说「同步 xxx 表」时把「xxx」填进 `source.table.name`——填了 name 就以 name 为准（服务端用 `base_url` 的 token + name 反查 tableId，覆盖 url 的 `table=` 参数），不填才用 url 的 `table=` 参数定位；别只给 `base_url`。
- batch 同步任务不能重新 enable。遇到 operation-not-allowed 先 `+db-sync-get` 查状态和结果；要持续同步就新建 streaming 任务，走 preview -> 用户确认 -> create。
- `+db-sync-*` 省略 `--environment` 默认落 online。多环境应用建表（`action=create`）必须显式 `--environment dev`；省略或填 `online` 会撞 `k_dl_4000001`（online 禁 DDL）——那是多环境应用的产品设计，把表建到 dev 分支即可，不要在 online 重试建表。共享库应用在 online 建表正常，不受此限。
- `+db-audit-list` 多表查询时，把结果里 `skipped` 的表（不存在 / 未开审计）连同原因一并向用户说明，不要让用户以为这些表「没有变更」。
- 恢复是覆盖式且不可逆：`+db-recovery-apply` 前必须先 `+db-recovery-diff`，并明确告知用户会覆盖当前数据。
