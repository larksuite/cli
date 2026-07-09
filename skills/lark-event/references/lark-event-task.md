# Task Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `task.task.update_user_access_v2` | A visible task has been created, deleted, or updated |

This key uses a **Native schema** (V2 envelope; output rooted at `.event`) and carries a **PreConsume hook** that calls the Task event subscription API before listening.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `task.task.update_user_access_v2` | `task:task:read` | user, bot |

Supports `--as user` or `--as bot`.

- `--as user`: receive task updates visible to the current user through authorship, assignment, following, or other access.
- `--as bot`: receive task updates for tasks the application is responsible for.

## `task.task.update_user_access_v2`

### Subscription behavior

On startup, `event consume` calls:

```text
POST /open-apis/task/v2/task_v2/task_subscription?user_id_type=open_id
```

The Task subscription API has no matching unsubscribe endpoint in the current CLI metadata, so graceful exit has no cleanup call for this EventKey. Re-running the consumer repeats the subscribe call for the selected identity.

This EventKey is single-consumer per local bus subscription: start one `event consume task.task.update_user_access_v2` process for a given app/profile/identity at a time.

### Output fields (V2 envelope; root path `.event`)

| Field | Type | Description |
|---|---|---|
| `.header.event_id` | string | Globally unique event ID; safe for deduplication |
| `.header.create_time` | string (timestamp_ms) | Event creation time in milliseconds |
| `.event.event_types[]` | string enum | Task commit types included in this event |
| `.event.task_guid` | string (kind=task_guid) | Task GUID that changed |

Commit types:

```text
task_assignees_update
task_completed_update
task_create
task_deleted
task_desc_update
task_followers_update
task_reminders_update
task_start_due_update
task_summary_update
```

### Example

```bash
# Stream task update events for the current user
lark-cli event consume task.task.update_user_access_v2 --as user

# Sample one event for payload inspection
lark-cli event consume task.task.update_user_access_v2 \
  --as user --max-events 1 --timeout 2m

# Project to a compact task-update record
lark-cli event consume task.task.update_user_access_v2 \
  --as user \
  --jq '{event_id: .header.event_id, task_guid: .event.task_guid, event_types: .event.event_types, timestamp: .header.create_time}'

# Consume as the app identity
lark-cli event consume task.task.update_user_access_v2 --as bot
```
