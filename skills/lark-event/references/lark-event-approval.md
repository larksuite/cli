# Approval Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (2)

| EventKey | Purpose |
|---|---|
| `approval.instance.status_changed_v4` | An approval instance status changed |
| `approval.task.status_changed_v4` | An approval task status changed |

Both keys use a **Custom schema**. The raw Lark schema 2.0 envelope is flattened: event metadata is exposed as `type`, `event_id`, and `timestamp`, while approval business fields are exposed at the top level.

Both keys carry a **PreConsume hook** that subscribes the current authorized user through the Approval subscription APIs before listening. The consumer intentionally does **not** unsubscribe on exit; the server-side Approval subscription relation remains until it is canceled outside `event consume`. These keys require `--as user`.

## Listener and subscription selection

At the raw CLI level, each `event consume` process accepts exactly one EventKey. `approval.instance.status_changed_v4` and `approval.task.status_changed_v4` have different output shapes, so listening to both still means two processes.

For Approval only, `subscription_type` is an optional setup param used by PreConsume to register server-side Approval subscription relations before the local listener starts. It is **not** an output field, a local event filter, or a local subscription identity. The pushed event does not say which subscription relation caused delivery, and one business event can match both relations; deduplicate with `event_id` when needed.

`subscription_type` may be omitted, a single value, a comma-separated list, or a JSON string array:

```bash
# Omitted: register both INVOLVED_APPROVAL and MANAGED_APPROVAL for this EventKey
lark-cli event consume approval.instance.status_changed_v4 --as user

# Single relation
lark-cli event consume approval.instance.status_changed_v4 \
  -p subscription_type=INVOLVED_APPROVAL \
  --as user

# Explicit multi-relation registration for one local consumer
lark-cli event consume approval.task.status_changed_v4 \
  -p subscription_type=INVOLVED_APPROVAL,MANAGED_APPROVAL \
  --as user

# JSON array form; quote it for the shell
lark-cli event consume approval.task.status_changed_v4 \
  -p 'subscription_type=["INVOLVED_APPROVAL","MANAGED_APPROVAL"]' \
  --as user
```

| Value | Meaning |
|---|---|
| `INVOLVED_APPROVAL` | Receive events where the current user is the approval requester or approver |
| `MANAGED_APPROVAL` | Receive events under approval definitions managed by the current user |

User-intent inference:

| User intent | EventKey(s) | `subscription_type` |
|---|---|---|
| Mentions approval instances, approval forms, approval order/status, or "instance status" | `approval.instance.status_changed_v4` | infer from relation words below |
| Mentions approval tasks, approval todo items, approver operations, or "task status" | `approval.task.status_changed_v4` | infer from relation words below |
| Says "approval status changes/events" without saying task vs instance | both EventKeys | infer from relation words below |
| Says "my approvals", "approvals involving me", "I requested/approved", "待我审批", "我发起/我参与" | requested EventKey(s) | `INVOLVED_APPROVAL` |
| Says "approvals I manage", "managed definitions", "definitions managed by me", "我管理的审批定义" | requested EventKey(s) | `MANAGED_APPROVAL` |
| Explicitly asks for both involved and managed, or says "all approval subscriptions" | requested EventKey(s), or both if EventKey is also ambiguous | omit `subscription_type`, or pass both values in one `-p` |
| Relation is ambiguous and the user wants broad coverage | requested EventKey(s), or both if EventKey is also ambiguous | omit `subscription_type` so PreConsume registers both |

If the user's wording omits the relation and broad listening is acceptable, omit `subscription_type`. Ask only when registering both relations would be materially harmful.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `approval.instance.status_changed_v4` | `approval:instance:read` | user |
| `approval.task.status_changed_v4` | `approval:task:read` | user |

## Subscription behavior

Startup calls the endpoint for the selected EventKey:

```text
POST /open-apis/approval/v4/instances/subscription
POST /open-apis/approval/v4/tasks/subscription
```

For each resolved `subscription_type`, PreConsume sends one request body:

```json
{"subscription_type":"INVOLVED_APPROVAL"}
```

If `subscription_type` is omitted, PreConsume sends two registration requests for that EventKey: one with `INVOLVED_APPROVAL`, then one with `MANAGED_APPROVAL`. If listening to both instance and task events, run two consumers; each consumer may omit `subscription_type` to register both relations for its own EventKey.

Do not start two consumers for the same Approval EventKey merely to split `INVOLVED_APPROVAL` and `MANAGED_APPROVAL`. The server push and flattened output are keyed by EventKey and cannot be distinguished by subscription relation.

Shutdown behavior:

`event consume` does not call the Approval unsubscribe APIs when it exits. This applies to graceful exit, Ctrl+C / SIGTERM, stdin EOF, `--timeout`, and `--max-events`.

To stop future delivery for a user, cancel the Approval subscription relation outside this consumer. The unsubscribe APIs are separate operations and are not called by `event consume`.

## Output fields

Common fields:

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type |
| `event_id` | string | Globally unique event ID; use for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time in milliseconds, taken from `header.create_time` |

Instance event fields:

| Field | Type | Description |
|---|---|---|
| `approval_code` | string | Approval definition code; not a subscription dimension |
| `instance_code` | string | Approval instance code |
| `external_id` | string | Third-party approval instance id, when present |
| `status` | string enum | `PENDING`, `APPROVED`, `REJECTED`, `CANCELED`, `DELETED`, `REVERTED`, `OVERTIME_CLOSE`, `OVERTIME_RECOVER` |
| `operate_time` | string (timestamp_ms) | Status change time |
| `start_user` | object | Instance starter user IDs, omitted when unavailable |
| `start_user.open_id` | string (open_id) | Instance starter open_id, when present |
| `start_user.union_id` | string (union_id) | Instance starter union_id, when present |
| `start_user.user_id` | string (user_id) | Instance starter tenant user_id, when present |

Task event fields:

| Field | Type | Description |
|---|---|---|
| `approval_code` | string | Approval definition code; not a subscription dimension |
| `instance_code` | string | Approval instance code |
| `task_id` | string | Approval task id |
| `external_id` | string | Third-party approval external id, when present |
| `task_external_id` | string | Third-party task external id, when emitted |
| `assigned_user` | object | Task assignee or operator user IDs, omitted for automatic flows without an operator |
| `assigned_user.open_id` | string (open_id) | Task assignee or operator open_id, when present |
| `assigned_user.union_id` | string (union_id) | Task assignee or operator union_id, when present |
| `assigned_user.user_id` | string (user_id) | Task assignee or operator tenant user_id, when present |
| `status` | string enum | `REVERTED`, `PENDING`, `APPROVED`, `REJECTED`, `TRANSFERRED`, `ROLLBACK`, `DONE`, `OVERTIME_CLOSE`, `OVERTIME_RECOVER` |
| `operate_time` | string (timestamp_ms) | Status change time |

## Examples

```bash
# Stream approval instance updates broadly; registers both involved and managed relations
lark-cli event consume approval.instance.status_changed_v4 \
  --as user

# Stream approval instance updates only for approvals involving the current user
lark-cli event consume approval.instance.status_changed_v4 \
  -p subscription_type=INVOLVED_APPROVAL \
  --as user

# Stream approval task updates for definitions managed by the current user
lark-cli event consume approval.task.status_changed_v4 \
  -p subscription_type=MANAGED_APPROVAL \
  --as user

# Broad approval status listening:
# run both EventKeys as separate processes; omit subscription_type so each registers both relations.
lark-cli event consume approval.instance.status_changed_v4 \
  --as user > approval-instance.ndjson &
lark-cli event consume approval.task.status_changed_v4 \
  --as user > approval-task.ndjson &
wait

# Listen to both involved and managed task subscriptions with one local consumer.
lark-cli event consume approval.task.status_changed_v4 \
  -p subscription_type=INVOLVED_APPROVAL,MANAGED_APPROVAL \
  --as user > approval-task.ndjson

# Project a compact approval-task record
lark-cli event consume approval.task.status_changed_v4 \
  -p subscription_type=INVOLVED_APPROVAL \
  --as user \
  --jq '{event_id, task_id, status, at: .operate_time}'
```
