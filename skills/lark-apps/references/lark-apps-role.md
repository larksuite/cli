# apps role 域命令（应用角色）

管理妙搭应用内的平台角色、角色成员，以及查询某个用户命中的角色。运行时命令事实以 `lark-cli apps +<cmd> --help` 为准；身份、授权和高风险确认遵循本域 [`SKILL.md`](../SKILL.md)。

## 何时用

用户要列出、查看、创建、更新或删除某个妙搭应用内的平台角色，管理角色的用户、部门或群成员，或查询某个用户在应用中命中的角色时使用。多维表格 / Base 的角色与权限走 `lark-base`；设置谁能访问应用走 `+access-scope-*`，不要路由到本命令域。

## 命令一览

| 命令 | 做什么 | 关键参数 |
|---|---|---|
| `+role-list` | 分页列出角色，或按名称筛选角色 | `--app-id`、`--name`、`--page-size`/`--page-token` |
| `+role-get` | 根据真实 `role_id` 读取角色详情 | `--app-id`、`--role-id` |
| `+role-match-list` | 查询指定用户命中的角色 | `--app-id`、`--user-id` |
| `+role-create` | 创建角色 | `--app-id`、`--name`、`--description`、`--role-id` |
| `+role-update` | 更新角色名称或描述 | `--app-id`、`--role-id`、`--name`/`--description` |
| `+role-delete` | 永久删除角色 | `--app-id`、`--role-id`、`--yes` |
| `+role-member-list` | 查询角色的用户、部门和群成员 | `--app-id`、`--role-id`、`--member-type` |
| `+role-member-add` | 向角色添加用户、部门或群成员 | `--app-id`、`--role-id`、`--users`/`--departments`/`--chats` |
| `+role-member-remove` | 定向移除或清空角色成员 | `--app-id`、`--role-id`、成员参数或 `--all`、`--yes` |

## 约定（先读）

- `app_...` 标识的是妙搭应用，其角色和成员只使用 `apps +role-*` / `apps +role-member-*`；不要改走 Base 角色命令或裸 bitable API。
- 角色名称不是 `role_id`。只有名称时优先用 `+role-list --name` 精确解析；若已取得完整分页列表，也可从中证明精确名称唯一命中。0 条如实报告，多条让用户消歧，唯一命中后才使用返回的真实 ID。
- `+role-list` 返回 `has_more=true` 时，用本页 `page_token` 继续查询，直到 `has_more=false`；不要根据 `total` 补造条目。
- `+role-list`、`+role-get`、`+role-match-list` 的角色数据分别位于 `data.items`、`data.role`、`data.roles`，不要混用。
- 同一角色的写入及依赖该写入结果的操作必须串行。不同角色的独立操作只有在每次写入可单独追溯、失败不影响其它目标且分别验收时才可并行；否则保持串行。互不依赖的名称解析或只读查询可并行。

## 各命令

### 查询角色

```bash
lark-cli apps +role-list --app-id <app_id> --page-size 100
lark-cli apps +role-list --app-id <app_id> --name '<exact_name>'
lark-cli apps +role-get --app-id <app_id> --role-id <role_id>
lark-cli apps +role-match-list --app-id <app_id> --user-id <ou_x>
```

整理角色列表时保留 `role_id`、`name` 和 `description`。不要猜测未知 `role_id`，也不要从同名候选中静默选择。
`items=[]` 时直接报告当前没有角色；不要为表格补造“无”或 `N/A` 占位行。
`+role-match-list --user-id` 只接受 `ou_...`；用户给的是姓名、邮箱或手机号时，先解析唯一 open ID，再查询命中角色。

### 创建与更新

```bash
lark-cli apps +role-create --app-id <app_id> --name '<name>' \
  --description '<description>'

# 只修改名称
lark-cli apps +role-update --app-id <app_id> --role-id <role_id> \
  --name '<new_name>' --as user --format json

# 只修改描述
lark-cli apps +role-update --app-id <app_id> --role-id <role_id> \
  --description '<new_description>' --as user --format json
```

- `--description` 和创建时的 `--role-id` 可选；仅在确实需要稳定 ID 时传 `--role-id`，创建后不能修改。
- 更新时只传用户明确要求变更的字段。
- 成功响应中的角色位于 `data.role`。只有用户要求独立验证，或结果将用于后续高风险操作时，才额外执行 `+role-get`。

### 删除角色

普通“删除某角色”请求只说明目标，**不等于不可逆确认**。如果用户尚未明确确认删除后果，本轮只能定位角色、读取完整成员并说明影响，最后请求确认；不得在同一轮自动追加 `--yes`。用户已明确确认不可逆删除时才继续。

只有名称时仍按上述规则唯一解析，优先使用 `+role-list --name`。目标写前已不存在时立即停止，如实说明本次是 no-op、没有执行删除，不能把“当前不存在”表述为“删除成功”。

删除前读取准确角色和完整成员范围，向用户说明 app、role、`users` / `departments` / `chats` 影响；得到不可逆删除确认后才使用 `--yes`：

```bash
lark-cli apps +role-get --app-id <app_id> --role-id <role_id>
lark-cli apps +role-member-list --app-id <app_id> --role-id <role_id>
lark-cli apps +role-delete --app-id <app_id> --role-id <role_id> --yes
```

成功响应包含匹配的 `data.role_id` 和 `data.deleted=true`。只有用户明确要求独立验证删除结果时，才再用 `+role-list --name` 检查目标 ID 已不存在。

### 成员 ID 解析

成员 flags 只接受 open ID：用户 `ou_...`、部门 `od-...`、群 `oc_...`。用户已提供对应类型的合法 open ID 时直接使用；只有名称或邮箱时才解析。
对象类型以用户语义为准，不能互换解析器：用户走通讯录用户搜索，部门走部门搜索，群走群搜索。

```bash
# 用户：每个姓名或邮箱单独查询。
lark-cli contact +search-user --query '<姓名或邮箱>' \
  --exclude-external-users --page-size 30

# 部门：拉完分页，只接受唯一的 open_department_id。
lark-cli api POST /open-apis/contact/v3/departments/search \
  --params '{"user_id_type":"open_id","department_id_type":"open_department_id","page_size":50}' \
  --data '{"query":"<部门名称>"}'

# 群：拉完分页，只接受名称精确匹配的唯一 chat_id。
lark-cli im +chat-search --query '<群名称>' --page-size 50
```

- 只接受与输入姓名、邮箱或群名精确匹配的唯一结果；部门搜索只接受完整 query 的唯一 `od-...`。0 条、多条或分页未完成时停止写入并让用户补充或消歧。
- 多个对象逐个解析。全部解析成功且总数不超过 100 后，按类型放入一次成员写入；任一对象失败时不要部分写入，也不要自动拆批。

### 成员操作

```bash
# 省略 --member-type，返回完整 users / departments / chats。
lark-cli apps +role-member-list --app-id <app_id> --role-id <role_id>

lark-cli apps +role-member-add --app-id <app_id> --role-id <role_id> \
  --users ou_x,ou_y --departments od-x --chats oc_x

lark-cli apps +role-member-remove --app-id <app_id> --role-id <role_id> \
  --users ou_x --yes

# 清空成员，不删除角色。
lark-cli apps +role-member-remove --app-id <app_id> --role-id <role_id> \
  --all --yes
```

- `+role-member-list` 不分页；`--member-type` 只返回选中类型的字段，未返回的成员字段表示“未查询”而不是空。影响确认或完整比较时必须省略它。
- 汇总 `--member-type` 结果时明确这是过滤投影，不得据此断言角色没有其它类型成员。
- 用户要求 CLI 原生 table 时，直接执行 `+role-member-list --format table`；可原样转发或做事实摘要，不要先取 JSON 再手工重建一张替代表格。
- 写入和依赖其结果的回读不得放进同一个并发批次；必须等待写入完整返回成功后，再单独发起回读。误并发时只能以写入完成后的新回读作为结果证据。
- 添加前仅在用户要求独立证明或确认其他成员类型未变化时读取完整基线，并在写后完整回读；否则成功响应即可作为结果。
- 定向移除前确认准确成员及影响。若需要证明结果，写后完整回读；不要把过滤结果当作完整成员集合。
- `--all` 前读取完整成员范围并确认；成功后执行一次无过滤 `+role-member-list`，确认三个成员数组均为空。

## 权限

| 操作 | 所需 scope |
|---|---|
| list / get / member-list / match-list | `spark:app:read` |
| create / update / delete / member-add / member-remove | `spark:app:write` |
