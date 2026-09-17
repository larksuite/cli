# 知识整理工作流：Execution

Loaded by states: `EXECUTE`, `VERIFY`.

This file owns confirmed write execution, PathTokenMap, progress reporting, verification, next suggestions, and execution-stage failure handling. It MUST NOT generate or revise plans.

## Required Context

Before executing rules in this file:

1. `active_plan_items` and `execution_scope` MUST already exist from [`lark-drive-workflow-knowledge-organize-planning.md`](lark-drive-workflow-knowledge-organize-planning.md).
2. `target_tree` and `resource_items` MUST already exist.
3. Use the `PlanItem` structure already produced by the planning phase. Do not regenerate or revise plans in this file.
4. Follow command syntax, scope requirements, and confirmation behavior from referenced shortcut docs.
5. Follow `Non-goals` from the main workflow entry. Do not execute excluded operations from this file.
6. Maintain internal `rollback_snapshot` and `execution_journal` during writes, but do not mention recovery on the normal successful path.

## State: EXECUTE

Entry: user explicitly confirmed execution scope.

Allowed writes only:

- 创建 Drive 文件夹：`drive +create-folder`
- 移动 Drive 文件 / 文件夹：`drive +move`
- 创建 Wiki 节点：`wiki +node-create`
- 移动已有 Wiki 节点：`wiki +move --node-token`
- 续跑异步移动任务：`drive +task_result`
- 单个资源权限申请：`drive +apply-permission`

MUST:

1. Resolve all target paths through `PathTokenMap`.
2. Build internal `rollback_snapshot` for all move items in confirmed scope before any write operation.
3. Initialize `execution_journal` before any write operation.
4. Create target folders / nodes shallow to deep.
5. Save returned tokens immediately.
6. Append an `execution_journal` entry after every create / move / async continuation.
7. Apply parent-child source move ordering.
8. Execute only confirmed scope.
9. Record success/failure per `PlanItem`.
10. Apply `Progress Reporting`.

MUST NOT:

- Execute operations listed in `Non-goals`.
- Rename or patch resource titles.
- Move using path string instead of token.
- Use `wiki +move` docs-to-wiki mode.
- Output rollback snapshot, rollback readiness, or execution journal on the normal successful path.

### Internal Recovery Hooks

These hooks are mandatory internal state maintenance. They do not create user-facing output on the normal path.

Rules:

1. Build `rollback_snapshot` before the first write command.
2. Include only compact fields needed for recovery. Do not store full API responses.
3. Append `execution_journal` immediately after each write attempt, including successful creates, successful moves, failed moves, and async continuation results.
4. If snapshot or journal cannot be maintained, stop before further writes and report the blocker.
5. If a write blocker occurs after one or more successful moves, report the blocker and ask whether the user wants to try restoring to `整理前的位置`.
6. Do not load the rollback phase or execute recovery until the user explicitly chooses to try restore.

Recovery question template:

```text
执行暂停：已成功移动 <moved_success_count> 项，失败 <failed_count> 项。

执行出现错误，已有部分资源移动成功。是否需要尝试恢复到整理前的位置？
```

### Progress Reporting

Small execution means `created_total + moved_total <= 50`.

For small executions, a final execution result is enough. For larger or long-running executions (`created_total + moved_total > 50`), the agent MUST periodically report progress by elapsed time and meaningful stage boundaries rather than by operation count alone.

Progress reports SHOULD be stage-specific. Include only fields relevant to the current stage. Do not output empty, unknown, or irrelevant fields.

Required fields by stage:

- Start: total create count, total move count, reporting cadence.
- Create stage finished: created count, failed count.
- Move stage progress / finished: current moved count as `<moved_done>/<moved_total>`, failed count, optional recent item.
- Blocked: current stage, completed count, blocker, required next action.

Examples:

- `执行开始：本次将创建 <created_total> 个目录 / 节点，移动 <moved_total> 个资源。任务较大，我会约每 60 秒汇报一次进度。`
- `执行进度：移动资源 <moved_done>/<moved_total>，失败 <failed_count>。`
- `执行暂停：<current_stage> 阶段遇到 <blocker>，已完成 <done>/<total>，需要 <required_action>。`

Rules:

1. When `created_total + moved_total > 50`, output one progress notice when execution starts.
2. After the create stage finishes, output one progress report when `created_total > 0`.
3. During the move stage, output a progress report about every 60 seconds, and once more when the move stage finishes.
4. Every move-stage progress report MUST include the current moved count as `<moved_done>/<moved_total>` and failed count, even if fewer than 50 additional moves completed since the previous report.
5. If execution is blocked by auth, permission, unresolved token, or API error, output the current progress and blocker before stopping.
6. Do not mention operation-count milestones such as "not yet reached 50 moves"; progress is time-based.
7. Do not output filler messages such as "still running", "no failure yet", or "not yet reached the next progress point" without current counts.
8. Do not report progress after every item unless the user explicitly asks for verbose execution logs.

## PathTokenMap

`PathTokenMap` maps target paths to real tokens before execution.

| Scope | Mapping |
|-------|---------|
| Drive | `target_path -> folder_token` |
| Wiki | `target_path -> node_token`, with `space_id` retained |

Rules:

1. Scan existing target folders / nodes before creating new ones.
2. Create planned folders / nodes from shallow to deep.
3. Save returned `folder_token` or `node_token` immediately after each successful create.
4. Execute `move` only when `target_parent_path` resolves to a token.
5. If same-name target ambiguity exists, inspect existing children when possible; otherwise mark `needs_review=true`.
6. Before writing to `my_library` root, resolve the real `space_id` according to `lark-wiki`.

## State: VERIFY

Entry: execution finished.

MUST:

1. Rescan target scope.
2. Compare each executed `PlanItem` with actual path/token.
3. Verify items covered by `covered_by_parent_move=true`.
4. Output success/failure/manual-confirmation counts.
5. Report mismatches with expected vs actual path/token.
6. Verify non-reused source containers planned for cleanup are no longer left in their original top-level position.
7. Verify reused target containers remain in place.
8. If serious mismatches exist, ask whether the user wants to try restoring to `整理前的位置`.
9. Do not load the rollback phase or execute recovery until the user explicitly chooses to try restore.

Verification table:

| plan_id | 动作 | 标题 | 预期目标 | 实际目标 | 预期 token | 实际 token | 状态 | 失败原因 |
|---------|------|------|----------|----------|------------|------------|------|----------|

### Verification Result

```text
执行完成。

| 项目 | 数量 |
|------|------|
| 创建成功 |  |
| 移动成功 |  |
| 待人工确认 |  |
| 失败 |  |

| plan_id | 动作 | 预期目标 | 实际目标 | 状态 | 失败原因 |
|---------|------|----------|----------|------|----------|
```

Serious mismatch recovery question template:

```text
验证发现 <mismatch_count> 项结果与计划不一致。

是否需要尝试恢复到整理前的位置？
```

### Next Suggestions

Only output `建议下一步` when at least one trigger exists. Do not add generic suggestions when execution and verification are clean.

Triggers:

- `partial=true`: inventory or content read was incomplete.
- Manual confirmation or low-confidence items remain.
- Failed items exist.
- One target folder / Wiki node contains more than 100 direct child resources after organization.
- Root-level loose resources remain.
- Non-reused source containers remain in their original top-level position after cleanup was planned.
- Verification found mismatches.

Template:

```text
建议下一步：
- <trigger-based suggestion>
```

## Execution Failure Handling

| Failure / Blocker | Agent MUST Do | Agent MUST NOT Do |
|-------------------|---------------|-------------------|
| Missing API scope | Follow `lark-shared` permission handling and stop | Do not retry the same command repeatedly |
| Resource access denied | Stop and follow the main workflow `Permission Request Gate` | Do not request permission automatically or in batch |
| Target path cannot resolve to token | Mark affected plan item failed or `needs_review=true` | Do not execute move with a path string |
| Target path has same-name ambiguity | Read existing children if possible; otherwise mark `needs_review=true` | Do not create duplicate target blindly |
| Async move returns `ready=false` or `next_command` | Follow the returned async continuation command | Do not assume completion |
| Parent-child move conflict | Follow source-depth ordering; move divergent children before parent | Do not move parent first when child target differs |
| Verification mismatch | Report expected vs actual path/token and failure reason | Do not silently mark success |
| Write blocker after successful moves | Report current progress and ask whether to try restoring to `整理前的位置` | Do not load rollback phase or execute recovery without explicit user choice |
