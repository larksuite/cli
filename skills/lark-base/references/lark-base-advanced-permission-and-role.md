# Base Advanced Permission 与 Role

This is the module entry point for Base advanced permissions and roles. Use it to choose commands and understand safety boundaries. For the permission JSON itself, use [Role Permission Schema](lark-base-role-config.md).

## Command selection

| Goal | Command | Notes |
|------|---------|-------|
| Check advanced permission status | `+base-get` | Read `data.base.is_advanced`. There is no `+advperm-get` command. |
| Enable advanced permissions | `+advperm-enable` | Required before creating or updating roles. Caller must be a Base admin. |
| Disable advanced permissions | `+advperm-disable` | High-risk write. Disabling invalidates existing custom roles. |
| Locate roles | `+role-list` | Returns role summaries. Use `+role-get` for full config. |
| Inspect one role | `+role-get` | Use before updating a role or deciding whether a role can be deleted. |
| Create a custom role | `+role-create` | Supports `custom_role` only. Read [Role Permission Schema](lark-base-role-config.md) before constructing `--json`. |
| Update a role | `+role-update` | Delta merge. Read current config first, then send only intended changes. |
| Delete a role | `+role-delete` | Custom roles only. System roles cannot be deleted. |

## Required order

At the start of a role workflow, before the first `+role-list`, `+role-get`, `+role-create`, `+role-update`, or `+role-delete` call:

1. Run `lark-cli base +base-get --base-token <base_token>` and inspect `data.base.is_advanced`.
2. If `is_advanced` is `false`, run `+advperm-enable` before the role command. If the user did not authorize enabling advanced permissions, stop and explain the required precondition.
3. Run the requested role commands only after `is_advanced` is `true` or `+advperm-enable` succeeds. Reuse that confirmed status for later role calls in the same workflow.

Do not probe with `+advperm-get`: that command is not supported. Do not use an empty `+role-list` response to infer the advanced permission status; a disabled Base can also return an empty list.

## Safety boundaries

- Role operations require advanced permissions to be enabled and the caller to be a Base admin.
- `+role-create` creates custom roles only.
- `+role-delete` is only for custom roles. System roles such as editor/reader can be configured within supported limits, but cannot be deleted.
- `+role-update` uses delta merge: omitted fields remain unchanged, but identity fields such as `role_name` and `role_type` should match the current target role.
- `+advperm-disable` invalidates existing custom roles; confirm the target Base and user intent before passing `--yes`.

## 全角色安全验收

启用或修改高级权限后，写响应成功不代表权限任务完成。以下流程只由明确的高级权限 / 角色配置要求，或同时包含**权限主体、权限能力和资源范围**的句子触发；Workflow 的“只要……就……”，以及普通字段、表单、视图操作中的“只 / 仅 / 所有”，不得触发角色枚举或权限收窄。

### 1. 写入前生成权限约束账本

先将当前权限步骤逐句展开，不能直接按角色名或字段名生成 JSON：

| 允许主体 | 排除主体 | 资源 | 能力 | 允许状态 | 禁止状态 |
|---|---|---|---|---|---|

- 资源可以是 Base、数据表、字段、记录范围、视图或 Dashboard。
- 能力必须拆成读取、编辑已有值、仅新增时填写、新增记录、删除记录、复制、下载和管理；“可编辑”不能代替其他能力。
- `只让 / 仅让 / 只有 / 仅限 + 角色、人群或可回指人称 + 权限动作` 先生成角色轴排他约束；句中的字段、记录条件只确定资源范围，不能吞掉角色轴。
- 角色轴、记录轴和字段轴可以同时命中：同一句还点名字段或记录操作时，同时生成对应约束，不得二选一。

### 2. 编译角色目标态

**先分清新建还是修改，判据是目标角色本身，不是 `+advperm-enable`。**

- 新建角色：仅该角色以 `no_perm` 为基线，按约束逐项开放。
- 既有角色：未命中本轮约束的配置保持原状；只定向修改违反约束的能力。
- 表级 `edit`、字段编辑、记录新增和记录删除是独立能力，不互相推导。
- 新建角色被允许填写 / 录入记录，同时仅点名部分字段可继续修改时：记录操作仅加入 `add`；新建记录所需业务字段设为 `create`，点名可修改字段设为 `edit`，系统字段与不可写派生字段保持 `read`；使用 `field_perm_mode=specify`，不得退化为 `all_edit`。
- 权限字段必须映射到真实 `+field-list` 结果；无法可靠唯一映射时只处理已确认字段并澄清剩余项，不得放宽为 `all_edit`。

### 3. 定向处理排他、敏感和行级隔离

- 一般排他约束只收窄目标资源上的目标能力。例如“只让某角色新增”只移除其他角色的 `add`，不自动删除其字段编辑权。
- 表被明确标记为敏感、保密或隐私，且同一权限任务列出允许访问角色时，这些角色构成**敏感表访问白名单**：未列出的普通角色、`Editor`、`Reader` 对该表必须为 `no_perm`，复制和下载保持关闭。
- “员工互相不可见”“只能看自己的”等行级隔离必须覆盖所有可能访问该表的默认和自定义角色：每个角色要么使用同一条有效隔离，要么对该表 `no_perm`；财务、管理员等被明确授权的角色可保留全量访问。
- `view_rule.visibility.all_visible` 只描述视图可见性，不能证明记录隔离；必须检查 `record_rule`。当前成员条件只有在字段类型正确、服务端回读 `is_invalid=false` 且没有其他全表可读路径时才成立。

### 4. 每个权限阶段写后断言

每个编号权限步骤或多轮权限步骤都要逐步执行，写后立即：

1. `+role-list` 枚举全部系统角色和自定义角色。
2. 对每个角色逐个执行 `+role-get`；解析到真实角色对象，不能停在 `data.data` 或字符串层。
3. 对照权限约束账本逐行检查 `perm`、`record_operations`、`field_perm_mode`、相关 `field_perms`、`record_rule`、`copy` 和 `download`。
4. 任一允许主体未获得目标能力、排除主体仍持有被禁止能力、新建角色的部分字段授权仍为 `all_edit`，该步骤都保持 `written`。
5. 只修复违反约束的角色和能力，再执行完整的 `+role-list` 与逐角色 `+role-get`；把 `+role-list` 返回的角色总数与按 `role_id` 去重后的 `+role-get` 成功数对齐，两者必须一致。
6. 全部约束通过后才能进入下一编号步骤；后续扩权、删角色或 `+advperm-disable` 不能替代或覆盖前一阶段验收。

这些角色命令不提供用户 / 部门到角色的成员映射。逐角色回读可以验证角色能力没有越界，但不能单独证明某个排除主体未加入允许访问的角色；没有成员映射 API 或经授权的排除主体实测时，必须把主体排除标记为 `unverified` / `blocked`。

授权证据可以来自本会话的任意前序步骤；已经明确授予且未被后续约束收回的权限继续有效。

## Common Fewshots

Use these fewshots for simple role changes. For table, field, record, dashboard, docx, or filter permission details, switch to [Role Permission Schema](lark-base-role-config.md).

Create a custom role that keeps copy/download disabled:

```bash
lark-cli base +role-create \
  --base-token <base_token> \
  --json '{"role_name":"Reviewer","role_type":"custom_role","base_rule_map":{"copy":false,"download":false}}'
```

Rename a role while preserving its type:

```bash
lark-cli base +role-update \
  --base-token <base_token> \
  --role-id <role_id> \
  --json '{"role_name":"Finance Reviewer","role_type":"custom_role"}' \
  --yes
```

Grant read-only access to one table:

```bash
lark-cli base +role-update \
  --base-token <base_token> \
  --role-id <role_id> \
  --json '{"role_name":"Finance Reviewer","role_type":"custom_role","table_rule_map":{"Orders":{"perm":"read_only"}}}' \
  --yes
```

## JSON SSOT

Use [Role Permission Schema](lark-base-role-config.md) for:

- `AdvPermBaseRoleConfig` top-level structure.
- `base_rule_map`, `table_rule_map`, `dashboard_rule_map`, and `docx_rule_map`.
- Table, view, field, record, dashboard, and docx permission values.
- Filter permission JSON.
- Default permission strategy and risk rules.
