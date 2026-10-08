# task +update

> **Prerequisites:** Please read `../../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.

Update an existing task in Lark.

> [!IMPORTANT]
> Use `+update` only for fields exposed by `--print-schema`. Task assignees are relationships managed by `+assign`; do not pass a `members` field copied from task search or get output.

## Recommended Commands

```bash
# Inspect the accepted --data fields before constructing JSON
lark-cli task +update --print-schema --flag-name data

# Update task summary
lark-cli task +update --task-id "<task_guid>" --summary "New Summary"

# Update multiple tasks' due dates
lark-cli task +update --task-id "<task_guid>,<another_task_guid>" --due "+2d"

# A task applink is accepted directly; the CLI extracts its guid query value
lark-cli task +update --task-id "https://applink.larksuite.com/client/todo/task?guid=<task_guid>" --summary "New Summary"

# Update with JSON data
lark-cli task +update --task-id "<task_guid>" --data '{"description": "New description"}'
```

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `--task-id <guid-or-applink>` | Yes | Task OpenAPI GUID or a task applink containing `guid=`. Comma-separated GUIDs/applinks are supported for multiple tasks. Display task IDs such as `t104121` / `suite_entity_num` are rejected. |
| `--summary <text>` | No | New summary/title for the task. |
| `--description <text>` | No | New description for the task. |
| `--due <time>` | No | New due date (supports relative time). |
| `--data <json>` | No | JSON object containing only fields exposed by `--print-schema --flag-name data`. |

## Workflow

1. Confirm with the user the tasks to update and the fields.
2. Before constructing `--data`, run `lark-cli task +update --print-schema --flag-name data` and use only listed fields.
3. Execute `lark-cli task +update --task-id "..." ...`
4. Read `data.updated_fields` and `data.tasks[].confirmed` from the result and report only the fields confirmed by the server.
5. Do not routinely call `task tasks get` after the update when `confirmed` already contains the required state. Query details only if a required field is absent or the user explicitly asks for a full verification.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
