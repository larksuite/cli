# 主题资料收集工作流：执行

由状态 `CONFIRM_EXECUTION`、`EXECUTE`、`VERIFY`、`RESTORE` 加载。

本文档负责最终写操作确认、目标创建、资源移动、验证、恢复行为、`RollbackSnapshotItem` 和执行日志。不得修改搜索、召回、分类规则或计划 schema。

本文档只服务 `topic_move_collector`。进入本文档时，`workflow_id` 必须是 `topic_move_collector`；不得把当前任务改路由到其他 workflow。

## 必读上下文

执行本文档规则前：

1. 按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 处理写操作确认、高风险操作、身份、认证和权限。
2. 按 [`lark-drive-create-folder.md`](lark-drive-create-folder.md) 创建 Drive 文件夹。
3. 按 [`lark-drive-move.md`](lark-drive-move.md) 执行 Drive 移动。
4. 按 [`../../lark-wiki/references/lark-wiki-node-create.md`](../../lark-wiki/references/lark-wiki-node-create.md) 创建 Wiki 节点。
5. 按 [`../../lark-wiki/references/lark-wiki-move.md`](../../lark-wiki/references/lark-wiki-move.md) 执行 Wiki 移动和 Drive 文档移动到 Wiki。
6. 按 [`../../lark-wiki/references/lark-wiki-move-to-drive.md`](../../lark-wiki/references/lark-wiki-move-to-drive.md) 将 Wiki 节点移出到 Drive 文件夹。
7. 按 [`lark-drive-delete.md`](lark-drive-delete.md) 删除本次 workflow 新建的 Drive 文件夹。
8. 按 [`../../lark-wiki/references/lark-wiki-node-delete.md`](../../lark-wiki/references/lark-wiki-node-delete.md) 删除本次 workflow 新建的 Wiki 节点。
9. 需要轮询异步任务时，按 [`lark-drive-task-result.md`](lark-drive-task-result.md) 执行。
10. `MovePlanItem` schema 由 [`lark-drive-workflow-topic-move-collector-review-plan.md`](lark-drive-workflow-topic-move-collector-review-plan.md) 定义，本文件只消费已确认计划。

## 状态：`CONFIRM_EXECUTION`

进入条件：移动计划已准备，且用户要求执行。

必须：

1. 执行前展示所有写操作类别。
2. 将目标创建和资源移动分开展示。
3. 展示默认纳入的高相关资源。
4. 如有用户选择的中相关资源，也要展示。
5. 展示跳过分组和原因。
6. 明确展示跨容器移动。
7. 展示无移动权限和移动权限未知的资源数量。
8. 请求用户明确确认。
9. 确认前校验每个 `move_resource` 项都包含完整 `command_family`、`command_args`、权限快照和 `rollback_input`；缺失时必须返回 `PLAN_MOVE` 重新生成计划，不得在执行阶段补猜。
10. 只有 `move_permission_state=movable` 且 `target_write_state=confirmed` 的计划项可以列入“将移动”。
11. 对每个 `rollback_supported=false` 的计划项逐项展示标题、当前位置、目标位置、不可恢复原因和影响，不得只展示数量。

### 确认 UI

```text
请确认是否执行以下写操作：

本次搜索范围：<当前用户 owner / 负责的资源 | 所有当前身份可见资源>

将创建：
- 目标名称｜父级位置｜目标类型

将移动：
- 标题｜类型｜当前位置｜目标位置｜原因

不会移动：
- 中相关未选择：N 项
- 低相关：N 项
- 无权限：N 项
- 无移动权限：N 项
- 移动权限未知：N 项
- 无法验证：N 项
- 不支持移动：N 项

风险提示：
- 不可自动恢复：N 项
- 标题｜当前位置｜目标位置｜不可恢复原因｜影响：移动成功后 workflow 无法自动搬回原位置，需要手动处理
- 如果搜索范围是所有当前身份可见资源，移动权限未知项不会移动。

确认后才会创建目标和移动资源。

如果不存在不可自动恢复项，请回复“确认执行”开始写操作。
如果存在不可自动恢复项，请回复“确认执行，包括不可自动恢复项”；普通“确认执行”不满足本次风险确认。
也可以回复“调整计划”返回选择资源，或回复“取消”结束流程。
```

如果用户修改选择或相关性分组，废弃当前 `move_plan_items` 并返回 `PLAN_MOVE` 重新生成计划；不得在 `CONFIRM_EXECUTION` 直接局部改写计划。

## 状态：`EXECUTE`

进入条件：用户明确确认写操作；存在 `rollback_supported=false` 的计划项时，用户已明确确认包括不可自动恢复项。

必须：

1. 只执行已确认 `MovePlanItem.command_family` 和 `command_args`；不得回查 `ResourceItem` 补齐或改写命令参数。
2. 当存在 `action_type=create_target` 的 `MovePlanItem` 时，先创建目标。
3. 目标创建后记录返回 token；只允许把 `created_by_plan:<create_target plan_id>` 引用解析为该 token，并把解析后的实际参数写入 `execution_journal`。不得重新搜索或猜测目标。
4. 目标 token 引用解析成功后再移动依赖该目标的资源；解析失败时停止依赖该创建目标的移动并记录 blocker，不得替换为其他目标。
5. 执行任何写操作前，基于每个已确认计划项的 `rollback_input` 生成 `rollback_snapshot`。`rollback_supported=false` 且已有明确 `rollback_blocker` 的快照视为完整风险快照，不阻塞其他项。
6. 执行任何写操作前，初始化 `execution_journal`。
7. 每次写操作尝试后记录 `execution_journal`。
8. 单项失败后可继续执行相互独立的移动；目标创建失败时必须停止。
9. 不得移动 `permission_denied`、`no_move_permission`、`move_permission_unknown`、`unverifiable`、`low` 或 `unsupported_move_target` 项。
10. 不得移动 `move_permission_state!=movable` 或 `target_write_state!=confirmed` 的资源。
11. 如果移动命令返回权限错误，记录失败原因，不自动申请权限，不自动重试同一移动。
12. 如果 `rollback_supported=true` 但 `rollback_input` 缺少恢复所需字段，将该计划项标记为 `failed` / `plan_snapshot_incomplete` 并跳过；不得在未重新确认风险的情况下把它静默降级为不可恢复项，也不得阻塞其他独立项。

### 移动方式选择

| 来源 -> 目标 | 移动方式 |
|------------------|-------------|
| Drive resource -> Drive folder | `drive +move` |
| Drive document-like resource -> Wiki target | `wiki +move` 的 docs-to-wiki 模式；默认不可自动恢复 |
| Wiki node -> Wiki target | `wiki +move --node-token` |
| Wiki node -> Drive folder | `wiki +move-to-drive` |

### 执行顺序

1. 如有 `create_target` 项，先执行。
2. 按确认计划顺序执行 `move_resource` 项。
3. 如果命令返回 task ID，执行异步任务轮询。
4. 输出写操作执行摘要。

### 进度 UI

批量较大时，按计数汇报进度：

```text
执行进度：已完成 <done_count>/<total_count>，成功 <success_count>，失败 <failed_count>。
当前操作：<title>
继续执行中，不需要你操作；如遇到需要确认的失败会单独提示。
```

## 状态：`VERIFY`

进入条件：执行完成。

必须：

1. 如果创建了目标，验证目标存在。
2. 能力支持时，验证已移动资源在目标位置可见。
3. 对比实际位置和 `move_plan_items`。
4. 为每一项标记验证状态。
5. 只有当已有移动成功且存在严重不一致或失败时，才提供恢复选项。
6. 输出验证结果时，必须说明用户下一步可以结束流程、查看失败项，或在可恢复时选择恢复。
7. 如果出现 `async_pending`，先使用 `drive +task_result` 轮询确认；超过轮询限制后再报告 pending blocker。

### 验证结果

| 状态值 | 说明 |
|--------|------|
| `verified` | 资源已在目标位置可见。 |
| `not_found` | 目标位置未找到资源。 |
| `permission_unknown` | 当前身份无法确认结果。 |
| `async_pending` | 异步任务尚未完成，需要继续轮询。 |
| `failed` | 移动命令失败或结果不符合计划。 |

## 状态：`RESTORE`

进入条件：失败、不一致或用户明确要求恢复。

必须：

1. 只基于 `rollback_snapshot` 和 `execution_journal` 生成恢复计划。
2. 展示可恢复项和不可恢复项。
3. 执行恢复写操作前请求明确确认；确认内容必须包含反向移动和删除本次 workflow 新建目标。
4. 只恢复本次 workflow 移动过的资源。
5. 只恢复 `rollback_supported=true` 且 `rollback_eligible=true` 的移动项。
6. Drive / Wiki 跨容器移动、原父级 token 缺失等 `rollback_supported=false` 的项不得反向移动，也不得删除迁入后的文档。
7. 本次 workflow 成功创建的目标文件夹或 Wiki 节点必须纳入清理计划。
8. 删除 workflow 新建的 Wiki 目标节点时，必须使用 `wiki +node-delete --include-children=false --yes`，让已迁入的直接子文档保留到该节点父级层级。
9. 删除 workflow 新建的 Drive 文件夹前，必须先恢复或移出其中由本次 workflow 放入的资源；如果无法确认文件夹已安全可删，报告清理阻塞，不得用删除文件夹来删除用户资源。

### 恢复顺序

1. 先恢复 `rollback_supported=true` 且 `rollback_eligible=true` 的移动项。
2. 对全部 `rollback_supported=false` 的项，只记录“保留在当前目标位置，不回迁、不删除”和对应 blocker。
3. 再清理 `created_by_workflow=true` 的目标容器。
4. Wiki 新建目标清理使用 `--include-children=false`；Drive 新建目标清理只在不会删除用户资源时执行。

### 恢复 UI

```text
可以尝试恢复本次已移动的资源：

可恢复：
- 标题｜当前位置｜原位置

不可自动恢复：
- 标题｜当前位置｜原位置｜原因｜影响：需要手动恢复

将清理本次新建目标：
- 名称｜类型｜清理方式

将保留在当前目标位置的跨容器迁入文档：
- 标题｜当前位置｜保留结果

是否执行恢复？
```

## RollbackSnapshotItem

```json
{
  "snapshot_id": "稳定快照行 ID",
  "plan_id": "对应 MovePlanItem.plan_id",
  "resource_id": "对应 MovePlanItem.resource_id",
  "source_kind": "drive|wiki",
  "title": "资源标题",
  "resource_type": "Drive 恢复命令需要的资源类型",
  "original_token": "原始 Drive token",
  "original_node_token": "原始 Wiki node token",
  "original_parent_kind": "drive_folder|drive_root|wiki_node|wiki_space_root|unknown",
  "original_parent_token": "原始父级 token",
  "original_space_id": "原始 Wiki space_id",
  "original_path": "执行前路径",
  "planned_target_parent_token": "计划目标父级 token",
  "rollback_supported": "是否支持自动恢复",
  "rollback_blocker": "不可自动恢复原因"
}
```

| 字段 | 说明 |
|-------|------|
| `snapshot_id` | 稳定快照行 ID。 |
| `plan_id` | 对应 `MovePlanItem.plan_id`，用于连接计划、快照和执行日志。 |
| `resource_id` | 对应稳定资源 ID，用于审计计划来源。 |
| `resource_type` | `drive +move` 恢复时必须传入的 `--type`；非 Drive 恢复也保留原始资源类型。 |
| `original_token` / `original_node_token` | 执行前源资源身份。 |
| `original_parent_kind` / `original_parent_token` | 执行前父级位置。 |
| `rollback_supported` | 是否支持自动恢复。 |
| `rollback_blocker` | 不可自动恢复原因。 |

## 执行日志

每次写操作尝试都必须追加一条内部日志：

```json
{
  "journal_id": "稳定日志行 ID",
  "plan_id": "对应 MovePlanItem 的 plan_id",
  "time": "ISO-8601",
  "action_type": "create_target|move_resource|restore_resource|cleanup_target",
  "operation": "create_folder|create_node|move_drive|move_wiki_node|move_wiki_to_drive|restore_drive|restore_wiki_node|delete_folder|delete_wiki_node",
  "command_family": "drive +move|wiki +move|wiki +move-to-drive|drive +create-folder|wiki +node-create|drive +delete|wiki +node-delete",
  "resolved_command_args": {"<arg>": "实际发送的参数"},
  "title": "资源或目标名称",
  "resource_type": "资源类型",
  "input_token": "命令输入 token",
  "input_node_token": "命令输入 Wiki node token",
  "input_parent_token": "已知源父级 token",
  "target_parent_token": "目标父级 token",
  "returned_token": "命令返回 token",
  "returned_node_token": "命令返回 Wiki node token",
  "returned_parent_token": "返回父级 token",
  "task_id": "异步任务 ID",
  "next_command": "异步继续命令",
  "created_by_workflow": "是否由本次 workflow 创建",
  "rollback_eligible": "是否可进入自动恢复计划",
  "status": "success|failed|pending",
  "error": "失败原因"
}
```

字段说明：

| 字段 | 说明 |
|------|------|
| `journal_id` | 稳定日志行 ID。 |
| `plan_id` | 对应 `MovePlanItem`，用于把日志项匹配回原计划。 |
| `operation` | 细分操作类型，用于区分创建、移动和恢复。 |
| `resolved_command_args` | 从确认计划解析出的实际发送参数；用于审计 `created_by_plan:<plan_id>` 的唯一运行时替换。 |
| `resource_type` | 实际移动 / 恢复使用的资源类型。 |
| `input_token` / `input_node_token` | 命令实际输入的资源 token。 |
| `input_parent_token` | 执行前已知源父级 token。 |
| `target_parent_token` | 命令输入的目标父级 token。 |
| `returned_token` / `returned_node_token` | 命令返回的资源 token，恢复时作为当前源。 |
| `returned_parent_token` | 命令返回的当前父级 token。 |
| `task_id` / `next_command` | 异步任务跟踪信息。 |
| `created_by_workflow` | 是否由本次 workflow 创建，用于后续清理判断。 |
| `rollback_eligible` | 是否可进入自动恢复计划。 |
| `status` | 写操作状态，异步未完成时为 `pending`。 |

除非用户要求查看技术调试细节，否则不要展示完整原始命令输出。
