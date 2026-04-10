# task +subscribe-event

> **Prerequisites:** Please read `../lark-shared/SKILL.md` to understand authentication, global parameters, and security rules.
>
> **⚠️ Note:** This API must be called with a user identity. **Do NOT use an app identity, otherwise the call will fail.**

Subscribe the current user to task update events for tasks they can access.

This shortcut is different from `event +subscribe`:
- `task +subscribe-event` uses a **user identity**
- it subscribes the **current user** to task events for tasks they created, are responsible for, or follow
- it is scoped to the user's task access, not a bot-level global event stream

The task event type is:

```text
task.task.update_user_access_v2
```

In practice, this means the subscribed user can receive updates for tasks that are visible to them through authorship, assignment, or following.

To actually receive the subscribed events, use the standard event WebSocket receiver:

```bash
lark-cli event +subscribe --event-types task.task.update_user_access_v2 --compact --quiet
```

The full flow is:
1. Register the user-facing subscription with `lark-cli task +subscribe-event`
2. Receive those events with `lark-cli event +subscribe --event-types task.task.update_user_access_v2 ...`

## Recommended Commands

```bash
lark-cli task +subscribe-event
```

## Parameters

This shortcut has no additional parameters.

## Workflow

1. Confirm the user wants to subscribe their own account to task update events.
2. Execute `lark-cli task +subscribe-event`
3. Report whether the subscription succeeded, and clarify that this applies to the user's own accessible tasks.

> [!CAUTION]
> This is a **Write Operation** -- You must confirm the user's intent before executing.
