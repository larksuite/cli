# 知识整理工作流

This file is the single entry point for the knowledge organization workflow. It defines the global contract, state machine, and progressive loading map. Stage-specific rules live in phase files and MUST be loaded only when the workflow reaches the corresponding state.

Phase files are references for this workflow, not independent skills. Do not route user requests directly to a phase file.

## Required Context

Before running this workflow, MUST read [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for identity, authentication, permission handling, and write-operation confirmation rules.

Load other skills / references progressively:

- Wiki / personal library target: [`../../lark-wiki/SKILL.md`](../../lark-wiki/SKILL.md)
- Content read required: [`../../lark-doc/SKILL.md`](../../lark-doc/SKILL.md) and [`../../lark-doc/references/lark-doc-fetch.md`](../../lark-doc/references/lark-doc-fetch.md)
- Sheet down-drill required: [`../../lark-sheets/SKILL.md`](../../lark-sheets/SKILL.md)
- Base down-drill required: [`../../lark-base/SKILL.md`](../../lark-base/SKILL.md)

## Agent Contract

When this workflow is triggered, the agent MUST:

1. Follow the `Execution State Machine` in order.
2. Maintain the fields in `Runtime State`.
3. Before executing a state, read the phase file listed in `Progressive Load Map`.
4. Do not pre-load all phase files. Load only the current state's required phase file unless a transition requires the next state.
5. Stop and wait whenever a state has `wait_for_user=true`.
6. Keep complete internal state even when user-facing output is paginated.
7. Never perform organization write operations before `EXEC_CONFIRM`; never perform recovery or cleanup writes before the corresponding explicit confirmation state.
8. Execute only commands allowed by `Command Map`.
9. Use command syntax, scope requirements, and API parameter rules from referenced skills / shortcut docs.
10. Convert internal enum values to natural-language Chinese labels in user-facing tables.
11. Do not invent recovery behavior; follow the active phase file's failure handling.
12. Maintain internal recovery state during execution, but do not mention recovery on the normal successful path.

## Scope

本 workflow 用于对指定 Drive 文件夹、Wiki 知识库、个人文档库或搜索范围做知识整理。默认只生成可审阅方案；只有用户明确确认执行范围后，才创建目录 / 节点或移动资源。

适用触发语包括：

- "帮我整理我的云盘 / 文档库 / 知识库"
- "帮我盘点这个知识库，给出整理后的目录结构"
- "这个文件夹太乱了，先给我一个整理方案"
- "把知识库里的文档按项目 / 客户 / 时间 / 类型归类"
- "帮我找出未归档、临时、重复、空目录和命名混乱的内容"

## Non-goals

默认不生成：

- 研究报告
- 对比分析
- 风险 / 结论 / 行动项
- 引用来源列表
- 权限治理报告

默认禁止执行：

- 删除原有文件、文件夹、Wiki 节点或知识空间
- owner 转移
- 批量权限申请
- 批量公开权限修改
- 批量协作者权限修改
- 任何资源重命名或标题修改；即使用户要求，也不由本 workflow 执行

仅在 rollback cleanup 阶段，允许删除本次 workflow 新建且当前可安全清理的空 Drive 文件夹或空 Wiki 节点，并且必须用户单独确认。不得删除知识空间。

如果用户明确要求其他非目标能力，必须转入对应专项流程，并单独确认风险。资源重命名 / 标题修改不属于本 workflow 的可执行能力。

## Responsibility Boundary

| File | Owns | Must Not Own |
|------|------|--------------|
| `lark-drive-workflow-knowledge-organize.md` | Workflow trigger, global contract, state machine, progressive load map, command family allowlist | Stage-specific rules, output templates, execution details |
| `lark-drive-workflow-knowledge-organize-discovery.md` | `PARSE_SCOPE`, `INVENTORY`, target parsing, stop conditions, inventory limits, `ResourceItem` | Classification, plan generation, write execution |
| `lark-drive-workflow-knowledge-organize-analysis.md` | `CONTENT_READ`, `ISSUE_ANALYSIS`, `RULE_GENERATION`, low-confidence reads, issue rules, problem pagination, classification, target tree | Plan execution, write confirmation, verification |
| `lark-drive-workflow-knowledge-organize-planning.md` | `PLAN_GENERATION`, `EXEC_CONFIRM`, `PlanItem`, `DisplayItem`, plan pagination, plan revision, execution scope confirmation | Scope parsing, resource inventory, write execution, verification |
| `lark-drive-workflow-knowledge-organize-execution.md` | `EXECUTE`, `VERIFY`, `PathTokenMap`, write execution, progress reporting, verification, next suggestions, internal recovery hooks | Scope parsing, resource inventory, classification, plan generation, plan revision, rollback execution details |
| `lark-drive-workflow-knowledge-organize-rollback.md` | `ROLLBACK_CONFIRM`, `ROLLBACK`, `ROLLBACK_VERIFY`, `ROLLBACK_CLEANUP_CONFIRM`, `ROLLBACK_CLEANUP`, `ROLLBACK_CLEANUP_VERIFY`, recovery plan generation, recovery execution, cleanup verification | Scope parsing, resource inventory, classification, organization plan generation, normal write execution |

## Runtime State

Agent MUST maintain these internal fields during one workflow run:

| Field | Meaning |
|-------|---------|
| `current_state` | Current state in `Execution State Machine` |
| `target_scope` | Parsed target: Drive folder, Wiki node, Wiki space, personal doc library, single resource, or search scope |
| `environment_profile` | Current environment and CLI profile, such as prod / BOE / PRE and config profile |
| `identity` | `user` by default unless user explicitly asks for app / bot perspective |
| `resource_items` | Complete normalized resource list from discovery |
| `partial` | Whether inventory or content read cannot fully continue because of auth, permission, API / pagination failure after retries, API coverage limitations, tool budget, or scope blockers; batching checkpoints alone are not partial |
| `inventory_continuation_state` | Structured checkpoint for continuing inventory batches within the confirmed scope. Must preserve `scope`, `queue`, `current_cursor`, `visited_page_keys`, `dedupe_keys`, and `blockers`; Drive queue entries carry `folder_token`, `path`, `depth`, and `page_token`; Wiki queue entries carry `space_id` / `node_token`, `path`, `depth`, and pagination cursor; search entries carry query / filters and pagination cursor. Missing or corrupt state is a blocker, not a completed inventory. |
| `low_confidence_items` | Items requiring mandatory partial content read |
| `issue_summary` | Problem types, counts, evidence paths, and suggested handling |
| `classification_rules` | Rules used to map resources to target paths |
| `target_tree` | Proposed target folder / Wiki node tree |
| `source_container_disposition` | Reused / retired source folders or nodes and their intended handling |
| `plan_items` | Complete internal execution plan |
| `plan_version` | Internal version of the current complete plan, such as `v1` / `v2` |
| `active_plan_items` | Latest complete valid plan used for execution confirmation |
| `plan_revision_history` | Internal summaries of user-requested plan revisions |
| `last_user_correction` | Most recent user correction that changed classification, target tree, or plan scope |
| `display_page_state` | Current page, page size, filters, and total count for user-facing pagination |
| `path_token_map` | Mapping from target path to real `folder_token` / `node_token` |
| `execution_scope` | Full plan, current page, filtered subset, or no execution |
| `verification_results` | Per-plan-item verification result after execution |
| `rollback_snapshot` | Internal pre-write snapshot used only for recovery after failure or user-requested restore |
| `execution_journal` | Internal write-operation journal used only for recovery after failure or user-requested restore |
| `rollback_plan` | Internal recovery plan generated only after user asks to restore |
| `rollback_verification_results` | Per-item recovery verification result |
| `rollback_cleanup_plan` | Optional cleanup plan for workflow-created empty folders / nodes after recovery |
| `rollback_cleanup_results` | Cleanup verification result |

## Execution State Machine

| State | Entry Condition | Agent MUST Do | User-Facing Output | wait_for_user | Next State |
|-------|-----------------|---------------|--------------------|---------------|------------|
| `PARSE_SCOPE` | Workflow triggered | Load discovery phase; parse target, environment, identity, and target type | Scope confirmation or clarification question | `true` | `INVENTORY` |
| `INVENTORY` | Scope confirmed | Load discovery phase; recursively list resources and build `resource_items` | Inventory progress / summary; continue automatically unless blocked | `false` unless blocked | `CONTENT_READ` |
| `CONTENT_READ` | Inventory complete | Load analysis phase; identify low-confidence items and perform mandatory partial read when needed | Low-confidence read summary | `false` unless auth / permission blocks | `ISSUE_ANALYSIS` |
| `ISSUE_ANALYSIS` | Resource list and partial reads ready | Load analysis phase; detect structure problems, evidence, and organization approach | Inventory result, problems, organization approach, and decision options | `true` | `RULE_GENERATION` |
| `RULE_GENERATION` | User confirms organization approach | Load analysis phase; generate classification rules and `target_tree` | No separate stop; target tree is shown with plan generation | `false` | `PLAN_GENERATION` |
| `PLAN_GENERATION` | Target tree ready | Load planning phase; generate complete internal `plan_items`; show target tree plus plan overview or page | Target tree and plan overview / paginated plan page | `true` | `EXEC_CONFIRM` |
| `EXEC_CONFIRM` | User wants execution | Load planning phase; ask user to choose execution scope | Execution options and write-operation summary | `true` | `EXECUTE` or `DONE` |
| `EXECUTE` | User explicitly confirmed execution scope | Load execution phase; execute only whitelisted write operations for confirmed scope while maintaining internal recovery state | Progress reports for large or long-running execution; if blocked after successful moves, ask whether to try restoring to `整理前的位置` | `false` unless blocked / recovery offered | `VERIFY`, `ROLLBACK_CONFIRM`, or `DONE` |
| `VERIFY` | Execution finished | Load execution phase; rescan target scope and compare actual path/token against plan | Verification table and final summary; if serious mismatches exist, ask whether to try restoring to `整理前的位置` | `false` unless recovery offered | `DONE` or `ROLLBACK_CONFIRM` |
| `ROLLBACK_CONFIRM` | User asks to restore after execution failure / verification mismatch / explicit rollback request | Load rollback phase; generate internal `rollback_plan`; ask whether to execute recovery | Recoverable scope and restore confirmation | `true` | `ROLLBACK` or `DONE` |
| `ROLLBACK` | User explicitly confirms restore execution | Load rollback phase; execute confirmed reverse moves only | Recovery progress / result | `false` | `ROLLBACK_VERIFY` |
| `ROLLBACK_VERIFY` | Recovery execution finished | Load rollback phase; verify restored locations and decide whether cleanup candidates exist | Recovery verification result | `false` | `ROLLBACK_CLEANUP_CONFIRM` or `DONE` |
| `ROLLBACK_CLEANUP_CONFIRM` | Cleanup candidates exist after recovery, or user asks to clean workflow-created empty folders / nodes | Load rollback phase; generate cleanup plan and ask for delete confirmation | Cleanup candidates and delete confirmation | `true` | `ROLLBACK_CLEANUP` or `DONE` |
| `ROLLBACK_CLEANUP` | User explicitly confirms cleanup deletion | Load rollback phase; delete only confirmed workflow-created safe-empty folders / nodes | Cleanup progress / result | `false` | `ROLLBACK_CLEANUP_VERIFY` |
| `ROLLBACK_CLEANUP_VERIFY` | Cleanup deletion finished | Load rollback phase; verify deleted cleanup targets | Cleanup verification result | `false` | `DONE` |
| `DONE` | No more action | Stop | Final answer | `false` | End |

## Progressive Load Map

Agent MUST read the phase file for the active state before executing that state.

| State | Required Phase File |
|-------|---------------------|
| `PARSE_SCOPE` | [`lark-drive-workflow-knowledge-organize-discovery.md`](lark-drive-workflow-knowledge-organize-discovery.md) |
| `INVENTORY` | [`lark-drive-workflow-knowledge-organize-discovery.md`](lark-drive-workflow-knowledge-organize-discovery.md) |
| `CONTENT_READ` | [`lark-drive-workflow-knowledge-organize-analysis.md`](lark-drive-workflow-knowledge-organize-analysis.md) |
| `ISSUE_ANALYSIS` | [`lark-drive-workflow-knowledge-organize-analysis.md`](lark-drive-workflow-knowledge-organize-analysis.md) |
| `RULE_GENERATION` | [`lark-drive-workflow-knowledge-organize-analysis.md`](lark-drive-workflow-knowledge-organize-analysis.md) |
| `PLAN_GENERATION` | [`lark-drive-workflow-knowledge-organize-planning.md`](lark-drive-workflow-knowledge-organize-planning.md) |
| `EXEC_CONFIRM` | [`lark-drive-workflow-knowledge-organize-planning.md`](lark-drive-workflow-knowledge-organize-planning.md) |
| `EXECUTE` | [`lark-drive-workflow-knowledge-organize-execution.md`](lark-drive-workflow-knowledge-organize-execution.md) |
| `VERIFY` | [`lark-drive-workflow-knowledge-organize-execution.md`](lark-drive-workflow-knowledge-organize-execution.md) |
| `ROLLBACK_CONFIRM` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |
| `ROLLBACK` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |
| `ROLLBACK_VERIFY` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |
| `ROLLBACK_CLEANUP_CONFIRM` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |
| `ROLLBACK_CLEANUP` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |
| `ROLLBACK_CLEANUP_VERIFY` | [`lark-drive-workflow-knowledge-organize-rollback.md`](lark-drive-workflow-knowledge-organize-rollback.md) |

## Command Map

Use only command families allowed for the current state. Detailed syntax belongs to referenced skills / shortcut docs.

| State | Allowed Command Families | Purpose |
|-------|--------------------------|---------|
| `PARSE_SCOPE` | `drive +inspect`, `wiki +node-get`, `wiki +space-list`, `wiki spaces get`, `drive +search` | Resolve target scope |
| `INVENTORY` | `wiki +node-list`, `drive files list` (schema path: `drive.files.list`), `drive metas batch_query` | Recursively list and enrich resources |
| `CONTENT_READ` | `docs +fetch`, plus `lark-sheets` / `lark-base` when conditionally required | Partial content read for low-confidence items |
| `ISSUE_ANALYSIS` | No write commands | Analyze `resource_items` only |
| `RULE_GENERATION` | No write commands | Generate classification rules and target tree |
| `PLAN_GENERATION` | No write commands | Generate internal plan and user-facing pages |
| `EXEC_CONFIRM` | No write commands | Ask user to confirm execution scope |
| `EXECUTE` | `drive +create-folder`, `drive +move`, `wiki +node-create`, `wiki +move` existing-node mode only, `drive +task_result`, `drive +apply-permission` only when explicitly confirmed | Execute whitelisted writes |
| `VERIFY` | `wiki +node-list`, `drive files list` (schema path: `drive.files.list`), `drive +task_result` if async result remains pending | Verify actual result |
| `ROLLBACK_CONFIRM` | No write commands | Generate internal recovery plan and ask for restore confirmation |
| `ROLLBACK` | `drive +move`, `wiki +move` existing-node mode only, `drive +task_result` | Execute confirmed reverse moves |
| `ROLLBACK_VERIFY` | `wiki +node-list`, `drive files list` (schema path: `drive.files.list`), `drive +task_result` if async result remains pending | Verify recovery result |
| `ROLLBACK_CLEANUP_CONFIRM` | No write commands | Generate cleanup plan and ask for delete confirmation |
| `ROLLBACK_CLEANUP` | `drive +delete`, `wiki +node-delete`, `drive +task_result` | Delete only confirmed workflow-created safe-empty folders / nodes |
| `ROLLBACK_CLEANUP_VERIFY` | `wiki +node-list`, `drive files list` (schema path: `drive.files.list`), `drive +task_result` if async result remains pending | Verify cleanup deletion result |

## Wiki Move Mode Constraint

This workflow MUST NOT use `wiki +move` docs-to-wiki mode. Wiki moves MUST use existing Wiki node mode with `--node-token` only.

## Permission Request Gate

`drive +apply-permission` is a write operation and may notify the resource owner. If any state hits resource access denial:

1. Stop the current state.
2. Show the single target resource, requested permission, reason / remark, and owner-notification implication when known.
3. Ask the user to confirm this single permission request.
4. Only after explicit confirmation, treat the permission request as a confirmed `EXECUTE` operation.
5. After the request is submitted or skipped, return to the blocked state only when the user asks to continue.

Never request permission automatically, never batch permission requests, and never hide the owner-notification implication.

## Transition Rules

1. If `PARSE_SCOPE` cannot determine the target range, ask only for target range clarification and stop.
2. If auth or API scope is missing, follow `lark-shared` permission handling and stop.
3. If resource access permission is missing, follow `Permission Request Gate`.
4. If the user asks to inspect more pages, stay in `PLAN_GENERATION` and update `display_page_state`.
5. If the user declines execution in `EXEC_CONFIRM`, output the saved plan summary and move to `DONE`.
6. If execution fails for an item, record the failure and continue only when the failed item is independent; otherwise stop, report the blocker, and ask whether the user wants to try restoring to `整理前的位置` when any move already succeeded.
7. Do not load the rollback phase merely because a snapshot or journal exists. Load it only after execution failure, serious verification mismatch, or explicit user rollback request, and only after the user chooses to try restore.

## References

- [Discovery phase](lark-drive-workflow-knowledge-organize-discovery.md)
- [Analysis phase](lark-drive-workflow-knowledge-organize-analysis.md)
- [Planning phase](lark-drive-workflow-knowledge-organize-planning.md)
- [Execution phase](lark-drive-workflow-knowledge-organize-execution.md)
- [Rollback phase](lark-drive-workflow-knowledge-organize-rollback.md)
- [lark-shared](../../lark-shared/SKILL.md)
- [lark-drive](../SKILL.md)
- [lark-drive-files-list](lark-drive-files-list.md)
- [lark-drive-search](lark-drive-search.md)
- [lark-drive-inspect](lark-drive-inspect.md)
- [lark-drive-apply-permission](lark-drive-apply-permission.md)
- [lark-drive-task-result](lark-drive-task-result.md)
- [lark-drive-delete](lark-drive-delete.md)
- [lark-wiki](../../lark-wiki/SKILL.md)
- [lark-wiki-node-delete](../../lark-wiki/references/lark-wiki-node-delete.md)
- [lark-doc](../../lark-doc/SKILL.md)
- [lark-doc-fetch](../../lark-doc/references/lark-doc-fetch.md)
- [lark-sheets](../../lark-sheets/SKILL.md)
- [lark-base](../../lark-base/SKILL.md)
