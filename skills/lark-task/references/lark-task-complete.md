# task +complete

> **Prerequisites:** Please read `../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.

Mark a task as completed.

## Recommended Commands

```bash
# Complete a task
lark-cli task +complete --task-id "<task_guid>"
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--task-id <guid>` | Yes | The task GUID to complete. For Feishu task applinks, use the `guid` query parameter, not the `suite_entity_num` / display task ID like `t104121`. |

## Workflow

1. Confirm the task to complete.
2. If the user gives both a tasklist and a task name, resolve the task inside that tasklist before using global task search:
   - Locate the tasklist first. If `+tasklist-search` has no recall, use `tasklists.list` pagination and exact local name matching.
   - Read incomplete tasks from the list with `lark-cli task tasklists tasks --as user --params '{"tasklist_guid":"<tasklist_guid>","completed":false,"page_size":100,"user_id_type":"open_id"}'`.
   - Match `summary` exactly against the requested task name. If exactly one incomplete task matches, use that task `guid`.
   - If the incomplete list has no unique exact match, global `lark-cli task +search --query "<task name>" --as user` may be used only as a fallback. For every fallback candidate, call `tasks.get` and keep only candidates whose `tasklists[].tasklist_guid` contains the target tasklist GUID.
   - If more than one in-scope task still matches, ask the user to disambiguate before completing; do not choose the first result.
3. Execute `lark-cli task +complete --task-id "<task_guid>"`.
4. Verify completion by reading the same tasklist's incomplete tasks again with `completed:false`; confirm the completed task's `guid` or exact `summary` is absent. Optionally read `completed:true` or `tasks.get` to show the task is now done.
5. Report success.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
