# 知识整理工作流：Planning

Loaded by states: `PLAN_GENERATION`, `EXEC_CONFIRM`.

This file owns plan generation, plan revision, user-facing pagination, and execution confirmation. It MUST NOT perform write operations.

## Required Context

Before executing rules in this file:

1. `resource_items`, `classification_rules`, and `target_tree` MUST already exist.
2. Follow command syntax, scope requirements, and confirmation behavior from referenced shortcut docs.
3. Follow `Non-goals` from the main workflow entry. Do not execute excluded operations from this file.

## State: PLAN_GENERATION

Entry: `target_tree` exists after the user confirmed the organization approach.

MUST:

1. Generate complete internal `plan_items`.
2. Build `DisplayItem` only for user-facing pages.
3. Apply `Plan Generation`.
4. Apply `Plan Pagination`.
5. Set `active_plan_items` to the latest complete plan.
6. Keep complete plan internally even if only one page is displayed.
7. Output `Target Tree And Plan Overview` or requested plan page, then wait.

### Plan Generation

| Condition | Agent MUST Do |
|-----------|---------------|
| Target path appears in any plan item | Ensure the path exists in `target_tree` |
| Source parent and descendants share same target subtree | Move parent only; mark descendants `covered_by_parent_move=true` |
| A child target differs from parent target | Move divergent child before parent; order by `source_depth` from deep to shallow |
| Target directory / node does not exist | Add `create_folder` / `create_node` before move |
| Resource is root-level and target path differs from current path | Add a `move` plan item; do not leave root-level resources in place by default |
| Resource has `needs_review=true` because classification evidence is insufficient | Set `target_path` to manual confirmation target, set `action=move`, and preserve `needs_review_reason` |
| Top-level folder / Wiki node has descendants that share the same target subtree | Move the parent folder / node only; descendants are covered by parent move |
| Top-level folder / Wiki node has descendants with divergent target subtrees | Move divergent descendants first; then move the parent only if it still has a target path or needs manual confirmation |
| Source container is reused as a target container | Keep the container in place; do not move it as source-container cleanup |
| Non-reused source container has descendants moved elsewhere | Add an explicit folder / node move plan item after descendant moves; target defaults to the source-container cleanup target |
| Source container handling is ambiguous | Move it to the manual confirmation target or mark `needs_review=true`; do not leave it in the root by default |
| Target parent token unresolved | Keep plan item but block execution until token is resolved |
| Resource title is poor or inconsistent | Report the naming issue only; do not create rename or title-patch plan items |

## PlanItem

`PlanItem` is for internal execution. It may contain tokens and internal enums.

| Field | Meaning |
|-------|---------|
| `plan_id` | Stable unique ID for plan / verification, such as `P001` |
| `source_path` | Current path |
| `title` | Resource title |
| `type` | Resource type |
| `source_token` | Drive token or normal resource token |
| `source_node_token` | Wiki node token; empty for non-Wiki resources |
| `source_parent_token` | Current parent folder token or parent Wiki node token |
| `source_depth` | Original depth in source tree |
| `target_path` | Target path |
| `target_parent_path` | Target parent path |
| `target_parent_token` | Target parent token; may be empty during planning, MUST be resolved before execution |
| `action` | Internal enum: `keep` / `create_folder` / `create_node` / `move` |
| `covered_by_parent_move` | Whether an ancestor move already covers this item |
| `reason` | Classification reason |
| `evidence_paths` | Evidence paths |
| `evidence_count` | Evidence count or hit count |
| `confidence` | Internal enum: `high` / `medium` / `low` |
| `needs_review` | Whether human review is required |
| `needs_review_reason` | Reason requiring human review |
| `rollback_origin_kind` | Internal recovery origin marker: `drive_folder` / `drive_root` / `wiki_node` / `wiki_space_root` / `unknown` |
| `rollback_origin_token` | Original parent token when applicable; empty for root markers |
| `rollback_origin_space_id` | Original Wiki space ID when `rollback_origin_kind=wiki_space_root` |
| `rollback_supported` | Whether this move item can be restored automatically if recovery is requested |
| `rollback_blocker` | Internal reason when `rollback_supported=false` |

### Rollback Origin Readiness

This is an internal execution-safety rule. Do not expose rollback readiness on the normal user-facing execution confirmation path.

Rules:

1. `action=move` items entering execution SHOULD have `rollback_origin_kind`.
2. `rollback_origin_kind` can be:
   - `drive_folder`: original Drive parent folder token is known.
   - `drive_root`: original location is the Drive root.
   - `wiki_node`: original Wiki parent node token is known.
   - `wiki_space_root`: original location is the Wiki space root and `rollback_origin_space_id` is known.
3. If `rollback_origin_kind` is missing or `unknown`, the agent MUST try to resolve it before execution from `ResourceItem.parent_token`, traversal context, `source_path`, `space_id`, or `wiki +node-get` for Wiki resources.
4. If the origin is still unresolved, set `rollback_supported=false` and `rollback_blocker`, but do not block the entire execution solely because recovery is unsupported.
5. Target resolution remains mandatory: a move item with unresolved `target_parent_token` MUST NOT execute.
6. Internal recovery metadata MUST NOT change `DisplayItem` output on the normal successful path.

## DisplayItem

`DisplayItem` is for user-facing output. It MUST NOT expose raw internal enum values.

| Display Field | Source |
|---------------|--------|
| `序号` | Page-local row number |
| `当前位置` | `source_path` |
| `标题` | `title` |
| `类型` | Human-readable `type` when possible; raw type is acceptable only when there is no clearer label |
| `目标位置` | `target_path` |
| `动作` | Convert from `action` using action display map |
| `原因` | `reason` |
| `置信度` | Convert from `confidence` using confidence display map |
| `待确认原因` | `needs_review_reason` |

Action display map:

| Internal Enum | User-Facing Label |
|---------------|-------------------|
| `keep` | 保持不变 |
| `create_folder` | 创建文件夹 |
| `create_node` | 创建知识库节点 |
| `move` | 移动到目标目录 |

`needs_review=true` is a review state, not an action. A review item MUST still use `action=move` when its target is the manual confirmation target.

### Manual Confirmation Target

Resources with insufficient classification evidence MUST be moved to the manual confirmation target after the user confirms execution.

Rules:

1. The target tree MUST include `待人工确认` or an equivalent user-specified manual confirmation path.
2. For Drive scopes, the manual confirmation target is a Drive folder.
3. For Wiki scopes, the manual confirmation target is a Wiki node.
4. Plan items for these resources MUST set `needs_review=true`, preserve `needs_review_reason`, set `target_path` to the manual confirmation target, and set `action=move`.
5. Do not leave these items in their original location by default.

Confidence display map:

| Internal Enum | User-Facing Label |
|---------------|-------------------|
| `high` | 高，证据明确 |
| `medium` | 中，有依据但建议确认 |
| `low` | 低，需要人工确认 |

### Plan Pagination

| Output Area | Rule |
|-------------|------|
| Plan details | Show at most 20 plan items per page |
| Plan item count > 20 | MUST paginate; do not output all details at once |
| Plan item count > 500 | First response MUST show overview and filters only; no detail rows until user asks |
| Pagination | Affects display only; complete `plan_items` MUST remain internal |

### Target Tree And Plan Overview

```text
建议目标目录结构

<target_tree>

移动 / 创建计划总览

本次计划共 <total_count> 项：
- 创建目录 / 节点：<create_count> 项
- 移动资源：<move_count> 项（其中来源目录本体：<source_container_move_count> 项）
- 保持不变：<keep_count> 项
- 待人工确认：<review_count> 项
- 高置信度：<high_count> 项
- 中置信度：<medium_count> 项
- 低置信度：<low_count> 项

你可以选择：
- 查看第 1 页明细
- 只看将创建的目录 / 节点
- 只看待人工确认项
- 只看高置信度移动项
- 进入执行确认
```

If `total_count > 500`, say:

```text
计划较大，我先只展示总览。
```

### Plan Revision Protocol

When the user corrects or adjusts the plan in `PLAN_GENERATION` or `EXEC_CONFIRM`, the agent MUST treat it as a full-plan revision unless the user explicitly asks to execute only the corrected items.

Revision triggers include:

- Adjusting classification rules.
- Adjusting target folder / Wiki node structure.
- Changing one or more resources' target paths.
- Excluding resources from movement.
- Restricting execution to high-confidence items.
- Moving a whole category to another target.
- Changing manual confirmation handling.
- Changing source container cleanup or retention handling.

Internal rules:

1. Record the user correction in `last_user_correction`.
2. Mark the previous `plan_items` as stale.
3. Recompute `classification_rules`, `target_tree`, and complete `plan_items` when needed.
4. Increment `plan_version`.
5. Set `active_plan_items` to the complete revised plan.
6. Append a short internal summary to `plan_revision_history`.
7. Do not execute stale `plan_items`.
8. Do not execute only the delta unless the user explicitly asks for partial execution.

User-facing output:

```text
已按你的修改重新生成完整计划。

已应用的修改：
- <correction item 1>
- <correction item 2>

当前完整计划：
- 创建目录 / 节点：<create_count> 项
- 移动资源：<move_count> 项
- 保持不变：<keep_count> 项
- 待人工确认：<review_count> 项

说明：后续执行默认基于这份完整修正版计划，不是只执行刚才的修正项。

你可以选择：
A. 查看修正版计划总览
B. 查看本次修改涉及的资源
C. 进入执行确认
D. 继续调整
```

If the user explicitly asks to execute only the corrected items, ask for confirmation before execution:

```text
你明确要求只执行本次修改涉及的 <count> 项。其余计划项不会执行。
请确认是否只执行这些项？
```

### Plan Detail Page

```text
移动 / 创建计划，第 <page>/<total_pages> 页，每页 20 项

| 序号 | 当前位置 | 标题 | 类型 | 目标位置 | 动作 | 原因 | 置信度 | 待确认原因 |
|------|----------|------|------|----------|------|------|--------|------------|

还有 <remaining_pages> 页未展示。

你可以回复：
- 继续看下一页
- 只看待人工确认项
- 只看低置信度项
- 进入执行确认
```

## State: EXEC_CONFIRM

Entry: user asks to execute.

MUST:

1. Show write-operation summary:
   - 将创建哪些目录 / 节点
   - 将移动哪些资源
   - 将移动哪些来源目录本体（如有）
   - 哪些资源仍需人工确认
   - 预计影响范围
2. Use `active_plan_items` from the latest complete plan.
3. Show `Permission Inheritance Notice`.
4. Ask for execution scope using `Execution Confirmation`.
5. Reference `Non-goals` for operations excluded from this workflow.
6. Wait for explicit confirmation.

### Permission Inheritance Notice

Before execution confirmation, MUST show this notice:

```text
权限提示：移动资源后，资源权限可能随目标位置变化，可见范围或协作权限可能变化。本 workflow 不会自动修改权限。
```

### Execution Confirmation

When the user wants execution, ask for execution scope:

Execution confirmation options MUST be renumbered by currently available choices. Do not show disabled choices, and do not ask the user to reply with skipped letters.

If a plan detail page is currently active:

```text
请确认执行范围：

A. 执行完整计划：<total_count> 项
B. 只执行当前页：<current_page_count> 项
C. 只执行高置信度项：<high_confidence_count> 项
D. 暂不执行，只保留方案

本 workflow 只执行已确认范围内的创建、移动和必要的单资源权限申请；不会重命名任何资源。
```

If no plan detail page is currently active:

```text
请确认执行范围：

A. 执行完整计划：<total_count> 项
B. 只执行高置信度项：<high_confidence_count> 项
C. 暂不执行，只保留方案

如需只执行某一页，请先查看计划明细页。

本 workflow 只执行已确认范围内的创建、移动和必要的单资源权限申请；不会重命名任何资源。
```

If there is no pagination, still state the total number of plan items covered by confirmation.
