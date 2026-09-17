# 知识整理工作流：Rollback

Loaded by states: `ROLLBACK_CONFIRM`, `ROLLBACK`, `ROLLBACK_VERIFY`, `ROLLBACK_CLEANUP_CONFIRM`, `ROLLBACK_CLEANUP`, `ROLLBACK_CLEANUP_VERIFY`.

This file owns recovery plan generation, recovery confirmation, recovery execution, recovery verification, cleanup confirmation, cleanup execution, and cleanup verification. It also defines the internal `rollback_snapshot` and `execution_journal` contracts.

It MUST NOT generate organization plans, revise classification rules, execute unconfirmed deletes, rename resources, modify permissions, or use `wiki +move` docs-to-wiki mode.

User-facing language should use "恢复到整理前的位置" / "恢复". Internal state and field names may use `rollback`.

## Required Context

Before executing rules in this file:

1. `active_plan_items`, `execution_scope`, `target_scope`, and `path_token_map` MUST already exist.
2. `rollback_snapshot` MUST have been built before the first write operation in `EXECUTE`.
3. `execution_journal` MUST contain write-operation results from `EXECUTE`.
4. Follow command syntax and risk behavior from referenced shortcut docs.
5. Follow `Non-goals` from the main workflow entry.

## Normal Path Visibility

Do not mention rollback, recovery readiness, snapshot, or journal on the normal successful execution path.

Load this file only after:

1. Execution failed after one or more successful moves and the user chose to try restoring.
2. Verification found serious mismatches and the user chose to try restoring.
3. The user explicitly asks to rollback / recover the previous organization run.

## Internal State Contracts

### RollbackSnapshot

`rollback_snapshot` records original locations before any write command.

Fields:

| Field | Meaning |
|-------|---------|
| `plan_id` | Matching `PlanItem.plan_id` |
| `source_kind` | `drive` / `wiki` |
| `title` | Resource title |
| `type` | Resource type used by move commands |
| `original_token` | Original Drive token when applicable |
| `original_node_token` | Original Wiki node token when applicable |
| `original_parent_kind` | `drive_folder` / `drive_root` / `wiki_node` / `wiki_space_root` / `unknown` |
| `original_parent_token` | Original parent token; empty for root markers |
| `original_space_id` | Original Wiki space ID when restoring to Wiki space root |
| `original_path` | Original path before organization |
| `planned_target_parent_token` | Planned target parent token |
| `planned_target_path` | Planned target path |
| `rollback_supported` | Whether this item can be restored automatically |
| `rollback_blocker` | Reason when `rollback_supported=false` |

Rules:

1. Store compact fields only. Do not store full API responses.
2. `drive_root` and `wiki_space_root` are valid origins; do not treat empty parent token as missing when the root marker is known.
3. Items without reliable origin can still execute, but MUST be marked `rollback_supported=false`.

### ExecutionJournal

`execution_journal` records every write attempt.

Fields:

| Field | Meaning |
|-------|---------|
| `journal_id` | Stable journal row ID |
| `plan_id` | Matching `PlanItem.plan_id` when applicable |
| `operation` | `create_folder` / `create_node` / `move_drive` / `move_wiki_node` / `delete_created_folder` / `delete_created_node` |
| `status` | `success` / `failed` / `pending` |
| `input_token` | Token supplied to the command |
| `input_node_token` | Wiki node token supplied to the command |
| `input_parent_token` | Source parent token when known |
| `target_parent_token` | Target parent token supplied to the command |
| `returned_token` | Token returned by the command |
| `returned_node_token` | Wiki node token returned by the command |
| `returned_parent_token` | Parent token returned by the command |
| `task_id` | Async task ID when returned |
| `next_command` | Async continuation command when returned |
| `error` | Error summary when failed |
| `created_by_workflow` | Whether the resource was created by this workflow run |
| `rollback_eligible` | Whether this successful operation can be included in `rollback_plan` |

Rules:

1. Append a journal entry immediately after each write attempt.
2. `create_folder` and `create_node` entries MUST set `created_by_workflow=true`.
3. Successful `move_drive` and `move_wiki_node` entries may set `rollback_eligible=true` only when matching snapshot origin is supported.
4. Failed or pending moves MUST NOT enter automatic recovery execution.
5. Async operations are `pending` until `drive +task_result` proves completion.

## State: ROLLBACK_CONFIRM

Entry: user chose to try restoring after execution failure / verification mismatch, or explicitly asked to rollback.

MUST:

1. Generate `rollback_plan` from successful eligible move journal entries.
2. Use `execution_journal` current token / current node token as the recovery source.
3. Use `rollback_snapshot` original origin as the recovery target.
4. Generate recovery items in reverse successful move order.
5. Exclude failed, pending, and unsupported items from executable recovery.
6. Do not include delete actions in `rollback_plan`.
7. Ask for explicit restore execution confirmation.

Confirmation output:

```text
可恢复范围如下：

| 项目 | 数量 |
|------|------|
| 可尝试恢复到原位置 | <recoverable_move_count> |
| 无法安全自动恢复 | <unsupported_count> |
| 未完成 / 等待中的移动 | <pending_count> |
| 本次新建目录 / 节点 | <created_container_count> |

恢复操作只会尝试把已成功移动的资源移回原位置，不会删除、重命名或修改权限。是否执行恢复？
```

If no move can be restored automatically, report that no automatic restore is available and move to `DONE`.

## Recovery Command Rules

Use only these command forms:

```bash
# Drive resource back to original parent folder
lark-cli drive +move \
  --file-token <current_token> \
  --type <type> \
  --folder-token <original_parent_token>

# Drive resource back to root
lark-cli drive +move \
  --file-token <current_token> \
  --type <type>

# Wiki node back to original parent node
lark-cli wiki +move \
  --node-token <current_node_token> \
  --target-parent-token <original_parent_token>

# Wiki node back to original space root
lark-cli wiki +move \
  --node-token <current_node_token> \
  --target-space-id <original_space_id>
```

MUST NOT:

- Use `wiki +move` docs-to-wiki mode.
- Move using path strings.
- Recover failed or pending moves as if they succeeded.
- Delete created folders / nodes in `ROLLBACK`.

## State: ROLLBACK

Entry: user explicitly confirmed restore execution.

MUST:

1. Execute only confirmed `rollback_plan` items.
2. Execute reverse moves in reverse successful move order.
3. Continue async move tasks with `drive +task_result` when needed.
4. Record recovery success / failure per rollback item.
5. Stop on blockers that make following recovery items unsafe.

Progress output should stay concise:

```text
恢复进度：已尝试 <done>/<total> 项，失败 <failed_count> 项。
```

## State: ROLLBACK_VERIFY

Entry: recovery execution finished.

MUST:

1. Rescan the relevant Drive folder / Wiki nodes.
2. Compare each rollback item with its original origin.
3. Mark status per item.
4. If cleanup candidates clearly remain from this workflow run, transition to `ROLLBACK_CLEANUP_CONFIRM`.
5. Do not ask for deletion confirmation in this state.

Verification table:

| plan_id | 标题 | 原位置 | 当前实际位置 | 状态 | 失败原因 |
|---------|------|--------|--------------|------|----------|

Status values:

| Status | Meaning |
|--------|---------|
| `rollback_success` | Resource is back under the original parent / root |
| `rollback_failed` | Resource is still outside the original origin |
| `missing` | Resource cannot be found |
| `needs_manual_review` | Actual state is ambiguous or affected by external changes |

Do not delete anything from this state.

## State: ROLLBACK_CLEANUP_CONFIRM

Entry: cleanup candidates exist after recovery, or user asks to view / perform cleanup after recovery.

Cleanup is optional and separate from recovery. It may delete resources, so it requires separate confirmation.

Candidate rules:

1. Candidate MUST have `created_by_workflow=true` in `execution_journal`.
2. Candidate MUST be a Drive folder or Wiki node created by this workflow run.
3. Candidate MUST currently be empty, or contain only workflow-created cleanup candidates that are themselves safe to delete.
4. Candidate MUST NOT contain original resources, unknown resources, rollback-failed resources, or user-created resources.
5. If child origin is uncertain, mark the candidate `cleanup_blocked`.

Generate `rollback_cleanup_plan` with:

| Field | Meaning |
|-------|---------|
| `cleanup_id` | Stable cleanup row ID |
| `type` | `drive_folder` / `wiki_node` |
| `path` | Current path |
| `token` | Folder token or node token |
| `depth` | Current path depth |
| `safe_to_delete` | Whether deletion is allowed after confirmation |
| `blocker` | Reason when deletion is blocked |

Confirmation output:

```text
恢复已完成。本次整理新建的部分空目录 / 节点如下，是否需要删除？

| 项目 | 数量 |
|------|------|
| 可删除的新建空目录 / 节点 | <safe_count> |
| 不可删除，需人工确认 | <blocked_count> |

注：删除只会作用于本次 workflow 新建且当前可安全清理的空目录 / 节点。
```

If the user wants details, paginate cleanup items at 20 rows per page.

## State: ROLLBACK_CLEANUP

Entry: user explicitly confirmed cleanup deletion.

MUST:

1. Delete only `safe_to_delete=true` cleanup items.
2. Delete deepest paths first.
3. Record delete results in `rollback_cleanup_results`.
4. Continue async delete tasks with `drive +task_result` when needed.

Command forms:

```bash
# Delete workflow-created Drive folder
lark-cli drive +delete \
  --file-token <folder_token> \
  --type folder \
  --yes

# Delete workflow-created Wiki node
lark-cli wiki +node-delete \
  --node-token <node_token> \
  --obj-type wiki \
  --include-children=true \
  --yes
```

`--yes` is allowed only after the user explicitly confirmed cleanup deletion.

MUST NOT:

- Delete original resources.
- Delete unknown resources.
- Delete rollback-failed resources.
- Delete non-empty folders / nodes that contain anything outside cleanup candidates.
- Delete a knowledge space.

## State: ROLLBACK_CLEANUP_VERIFY

Entry: cleanup deletion finished.

MUST:

1. Verify each confirmed cleanup target is gone.
2. Report failed or pending deletes.
3. Stop after reporting cleanup results.

Verification table:

| 类型 | 路径 | token | 状态 | 失败原因 |
|------|------|-------|------|----------|

Status values:

| Status | Meaning |
|--------|---------|
| `deleted` | Cleanup target was deleted |
| `delete_pending` | Async deletion is still pending |
| `delete_failed` | Delete command failed |
| `still_exists` | Target still exists after deletion attempt |
| `skipped` | Target was not safe to delete or user did not confirm it |
