# 主题资料收集工作流：审核与计划

由状态 `RELEVANCE_CLASSIFY`、`PLAN_MOVE` 加载。

本文档负责相关性分级、审核 UI、移动计划生成和 `MovePlanItem`。不得重新执行资源解析或内容验证，也不得创建目标、移动资源或执行恢复操作。

本文档只服务 `topic_move_collector`。进入本文档时，`workflow_id` 必须是 `topic_move_collector`；不得把当前任务改路由到其他 workflow。

## 输入契约

进入本文档前必须已有：

1. `resource_items`，且每个 `ResourceItem` 已包含稳定 `resource_id`、资源类型、移动所需 token、结构化当前父级、权限状态、内容验证状态和证据。
2. `content_verify_completed=true`。
3. 每个资源都有内容证据、搜索证据复用说明或明确跳过原因。

`ResourceItem` schema 和字段生成规则由 [`lark-drive-workflow-topic-move-collector-resolve-verify.md`](lark-drive-workflow-topic-move-collector-resolve-verify.md) 负责。只要上述输入契约完整，本状态不得为重复读取 schema 而重新加载或执行前一阶段文档。

如果输入字段缺失、资源需要重新解析或用户要求重新读取证据，废弃受影响的相关性和计划结果，返回 `RESOURCE_RESOLVE` 或 `CONTENT_VERIFY`，并加载资源解析与内容验证文档；不得在本状态补猜。

## 状态：`RELEVANCE_CLASSIFY`

进入条件：`CONTENT_VERIFY` 已完成，`content_verify_completed=true`，且每个 `ResourceItem` 都已有验证状态或跳过验证原因。

禁止条件：

1. 只有 `candidate_items`，没有 `resource_items`。
2. 资源未经过 `RESOURCE_RESOLVE`。
3. 资源没有 `RESOURCE_RESOLVE` 写入的移动资格状态。
4. 资源没有 `CONTENT_VERIFY` 写入的验证状态或跳过验证原因。
5. 上一完成状态是 `RESOURCE_RESOLVE`，或 `content_verify_completed` 不为 `true`。

必须将每个资源归入且只归入一个分组：

| 分组 | 说明 | 默认移动 |
|-------|------|--------------|
| `high` | 可移动资源，且主题或内容直接命中，有明确标题 / 正文 / 表格 / 评论证据。 | 是 |
| `medium` | 可移动资源，可能相关，但证据不足或只命中弱相关片段。 | 否，需用户选择 |
| `low` | 可移动资源，弱相关或噪声，保留展示但不建议移动。 | 否 |
| `permission_denied` | 当前身份无权读取或解析，不能验证内容。 | 否 |
| `no_move_permission` | 已确认当前身份不具备移动资格。 | 否 |
| `move_permission_unknown` | 无法确认当前身份是否具备移动资格。 | 否 |
| `unverifiable` | 类型或工具限制导致无法验证内容。 | 否 |
| `unsupported_move_target` | 目标方向或资源类型不支持移动。 | 否 |

`high`、`medium` 和 `low` 只能包含 `move_permission_state=movable` 且 `target_write_state=confirmed` 的资源。

判为高相关至少需要一个强证据：

1. 标题或内容中出现精确主题短语。
2. 多个主题词在相关上下文中同时出现。
3. Sheet / 表格单元格明确匹配用户主题。
4. 用户明确提供的文档名或项目别名命中。

中相关示例：

1. 标题包含一个主题词，但内容无法确认。
2. 搜索摘要看起来相关，但无法完整读取。
3. 别名命中合理但证据不够强。

## 审核 UI

必须展示每个分组中的资源名称。

默认展示规则：

1. 展开 `high` 和 `medium`。
2. 折叠 `low`、`permission_denied`、`no_move_permission`、`move_permission_unknown`、`unverifiable` 和 `unsupported_move_target`，但展示数量并允许展开。
3. 每个可见资源展示标题、类型、当前位置、证据和默认动作。
4. 除非用户要求技术细节，否则不展示原始 token。

示例：

```text
筛选结果：

搜索范围：<当前用户 owner / 负责的资源 | 所有当前身份可见资源>

高相关（默认移动）：
- 标题｜类型｜证据｜当前位置

中相关（需你勾选后才移动）：
- 标题｜类型｜证据｜当前位置

未默认移动：
- 低相关：N 项
- 无权限：N 项
- 无移动权限：N 项
- 移动权限未知：N 项
- 无法验证：N 项
- 不支持移动：N 项

你可以选择：
1. 确认按默认规则生成移动计划。
2. 勾选要加入计划的中相关资源。
3. 要求把某些资源移到其他分组或从计划中移除。
4. 展开低相关 / 无权限 / 无移动权限 / 移动权限未知 / 无法验证 / 不支持移动分组查看名称。
```

### 用户调整规则

如果用户不同意相关性结果，必须基于用户要求更新 `relevance_groups`，再重新展示分组结果并重新生成后续移动计划。

典型调整包括：

1. 从 `high` 中移除某个资源。
2. 将 `medium` 中某个资源提升为 `high`。
3. 将某个资源标为 `low` 或不移动。
4. 要求重新读取证据或重新判断一批资源。
5. 要求重新确认某些资源的移动权限。

用户调整后：

1. 旧的 `move_plan_items` 立即失效。
2. 必须先输出“调整后相关性结果”，展示被调整项、各分组数量和高 / 中相关资源名称。
3. 不得只回复“已调整”，也不得直接跳到 `CONFIRM_EXECUTION`。
4. 必须基于新的 `relevance_groups` 重新执行 `PLAN_MOVE`。
5. 不得把 `no_move_permission` 或 `move_permission_unknown` 资源直接提升到 `high` / `medium`；必须先回到 `RESOURCE_RESOLVE`，加载 [`lark-drive-workflow-topic-move-collector-resolve-verify.md`](lark-drive-workflow-topic-move-collector-resolve-verify.md) 取得可移动证据。

### 调整后结果 UI

```text
已按你的要求调整相关性结果：
- <标题>：<原分组> -> <新分组>

调整后分组：

搜索范围：<当前用户 owner / 负责的资源 | 所有当前身份可见资源>

高相关（默认移动）：N 项
- 标题｜类型｜证据｜当前位置

中相关（需你勾选后才移动）：N 项
- 标题｜类型｜证据｜当前位置

未默认移动：
- 低相关：N 项
- 无权限：N 项
- 无移动权限：N 项
- 移动权限未知：N 项
- 无法验证：N 项
- 不支持移动：N 项

接下来会基于这个调整后的结果重新生成移动计划；你也可以继续调整。
```

## 状态：`PLAN_MOVE`

进入条件：相关性分组已准备。

必须：

1. 当 `target_location.create_required=true` 时，纳入目标创建计划。
2. 生成移动计划前，比较规范化的当前父级与目标父级；已在目标位置的资源生成 `skip_resource`，设置 `skip_reason=already_at_target`，不得生成移动命令。
3. 默认纳入全部 `high`、`move_permission_state=movable` 且 `target_write_state=confirmed` 的资源。
4. 只有用户明确选择时，才纳入 `medium`、`move_permission_state=movable` 且 `target_write_state=confirmed` 的资源。
5. 默认排除 `low`、`permission_denied`、`no_move_permission`、`move_permission_unknown`、`unverifiable` 和 `unsupported_move_target`。
6. 为每个跳过项生成 `skip_reason`。
7. 为每个计划项生成稳定 `plan_id`，并使用 `resource_id` 连接对应资源；不得按标题或临时 token 猜测关联。
8. 按 `command_family` 保存完整、不可变的 `command_args`；不得把 Wiki 底层对象 token 当作 Wiki 节点移动 token。
9. 为每个 `move_resource` 项复制执行前恢复所需的完整 `rollback_input`，使确认计划不依赖运行时回查 `ResourceItem`。
10. 当前父级无法结构化解析或属于 Drive / Wiki 跨容器移动时，设置 `rollback_supported=false` 和明确 `rollback_blocker`；该单项仍可进入确认，但必须逐项展示不可恢复风险，不得阻塞其他独立项。
11. 停止并等待用户选择或执行意图。
12. 不得为 `move_permission_state!=movable` 或 `target_write_state!=confirmed` 的资源生成 `move_resource` 计划项。

### 已在目标位置判定

1. `drive_move` 比较 `current_parent_kind` 和目标 Drive 父级，并比较规范化后的 `current_parent_token` / root 标识。
2. `wiki_move_node` 比较 `current_parent_space_id`、`current_parent_kind` 和 `current_parent_token`；Wiki 空间根节点使用明确的 root 标识，不得用空字符串和未知状态混淆。
3. 只有父级类型、space ID（适用时）和 token 都已解析且相等时，才能设置 `skip_reason=already_at_target`；父级未知时不得猜测为相等。

### 移动 token 选择

| `command_family` | `command_args` 必须包含 |
|------------------|---------------------------|
| `drive +move` | `file_token`、`type`、`folder_token`；移动到 Drive root 时显式记录 `folder_token` 为空且目标类型为 root。 |
| `wiki +move`（node） | `node_token`，以及 `target_space_id` 或 `target_parent_token`；可选 `source_space_id`。不得使用 `wiki_obj_token` 代替 `node_token`。 |
| `wiki +move`（docs-to-wiki） | `obj_type`、`obj_token`、`target_space_id`、可选 `target_parent_token`，并显式保存 `apply=false`。 |
| `wiki +move-to-drive` | `node_token`、`folder_token`；移动到 Drive root 时显式记录 `folder_token` 为空。 |
| `drive +create-folder` | `name`、父级 `folder_token`；创建在 Drive root 时显式记录父级为空。 |
| `wiki +node-create` | `space_id`、`title`、`obj_type`、可选 `parent_node_token`。 |
| `none` | 不执行命令，保留 `skip_reason`。 |

目标由本次 workflow 创建时，对应目标参数保存 `created_by_plan:<create_target plan_id>` 引用。`EXECUTE` 只允许把该引用替换为对应创建计划返回的 token；不得重新搜索或猜测目标。

### 计划 UI

```text
移动计划已生成：
- 默认将移动高相关：N 项
- 你已选择中相关：N 项
- 其中不可自动恢复：N 项
- 已在目标位置：N 项
- 不会移动：N 项
- 无移动权限：N 项
- 移动权限未知：N 项

你可以回复“确认执行”，也可以继续调整分组、增减中相关资源，或取消本次移动。
```

## MovePlanItem

```json
{
  "plan_id": "稳定计划项 ID",
  "resource_id": "对应 ResourceItem.resource_id；create_target 为空",
  "action_type": "create_target|move_resource|skip_resource|unsupported",
  "title": "资源或目标名称",
  "resource_type": "源资源类型",
  "move_method": "drive_move|wiki_move_node|wiki_move_docs_to_wiki|wiki_move_to_drive|none",
  "command_family": "具体 shortcut 命令或 none",
  "command_args": {
    "<arg>": "按 command_family 参数表保存的完整、类型明确的参数"
  },
  "source_path": "用户确认时展示的源位置",
  "target_path": "用户确认时展示的目标位置",
  "move_permission_state": "movable|denied|unknown|not_required",
  "target_write_state": "confirmed|unknown|denied",
  "reason": "纳入或跳过原因",
  "skip_reason": "already_at_target 或其他跳过原因",
  "rollback_input": {
    "source_kind": "drive|wiki",
    "original_token": "原始 Drive / obj token",
    "original_node_token": "原始 Wiki node token",
    "resource_type": "恢复命令需要的资源类型",
    "original_parent_kind": "drive_folder|drive_root|wiki_node|wiki_space_root|unknown",
    "original_parent_token": "原始父级 token",
    "original_space_id": "原始 Wiki space_id",
    "original_path": "执行前路径"
  },
  "rollback_supported": "是否支持自动恢复",
  "rollback_blocker": "不可自动恢复原因",
  "execution_status": "pending|success|failed|skipped"
}
```

| 字段 | 说明 |
|-------|------|
| `plan_id` | 稳定计划项 ID，用于连接计划、快照和执行日志。 |
| `resource_id` | 稳定资源 ID，用于连接确认计划和解析结果；`create_target` 为空。执行阶段不得依赖该关联回查可变参数。 |
| `action_type` | 计划动作类型。 |
| `move_method` | 实际使用的移动方式。 |
| `command_family` / `command_args` | 用户确认的完整写命令及参数快照；确认后保持不可变。目标待创建时只允许使用 `created_by_plan:<plan_id>` 引用。 |
| `move_permission_state` / `target_write_state` | 用户确认时的权限门禁快照；`move_resource` 必须分别为 `movable` / `confirmed`。`create_target` 的移动权限为 `not_required`，但父级写入权限仍必须为 `confirmed`。 |
| `rollback_input` | 从 `ResourceItem` 复制出的完整恢复输入；仅 `move_resource` 必填，生成确认计划后不得再回查或猜测。 |
| `rollback_supported` | 是否支持自动恢复。 |
| `rollback_blocker` | 不可自动恢复原因；跨容器移动使用 `cross_container_permission_model_not_losslessly_restorable`，原父级 token 缺失使用 `original_parent_token_unavailable`。 |
| `execution_status` | 执行状态。 |
