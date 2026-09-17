# IM message read status

> **Prerequisite:** Read [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication and global parameters.

Use two focused shortcuts for message read-status queries:

- `im +messages-read-status` queries whether the current user has read 1–50 messages.
- `im +message-read-users` lists users who have read one message and supports automatic pagination.

Both shortcuts and their catalog APIs support the current Agent credential. Retain `--as user`; “current” read status refers to the Agent caller. For read-users queries, the caller must be in the chat and the queried message must have been sent by the caller within the supported seven-day window. Required scope and credential errors follow lark-shared.

## Batch query the current user's read status

```bash
# Preview one request
lark-cli im +messages-read-status \
  --message-ids om_xxx,om_yyy \
  --as user \
  --dry-run

# Execute with the current Agent credential
lark-cli im +messages-read-status \
  --message-ids om_xxx,om_yyy \
  --as user \
  --json
```

The command accepts 1–50 comma-separated `om_` message IDs. The CLI checks the required read permission. The response keeps the OpenAPI response unchanged:

- `items[].message_id` and `items[].is_read` contain statuses the server could determine.
- `invalid_message_ids` contains messages that do not exist, are not visible to the current user, or do not support this query. The API deliberately does not expose a more specific reason.

## List users who read one message

```bash
# Fetch one page as the current user
lark-cli im +message-read-users \
  --message-id om_xxx \
  --as user \
  --json

# Fetch every page, bounded to ten pages by default
lark-cli im +message-read-users \
  --message-id om_xxx \
  --user-id-type open_id \
  --page-all \
  --as user \
  --json
```

Pagination flags:

- `--page-size`: 1–100, default 100.
- `--page-token`: start from a known cursor.
- `--page-all`: continue until the endpoint is exhausted.
- `--page-limit`: maximum pages with `--page-all`; default 10, range 1–1000.
- `--page-delay`: delay in milliseconds between pages; default 200, and 0 disables the delay.

The command preserves each server item, including `user_id_type`, `user_id`, `timestamp`, and `tenant_key`. Pagination metadata reports whether the endpoint was exhausted and retains the next token when a bounded run can be resumed.

## Raw API commands

The corresponding catalog methods support Agent credentials. Inspect their method schemas before invoking:

```bash
lark-cli im messages read_status --data '{"message_ids":["om_xxx"]}' --as user
lark-cli im messages read_users --params '{"message_id":"om_xxx","user_id_type":"open_id"}' --as user
```

Prefer the shortcuts for flag validation, identity-specific scope hints, and read-users auto-pagination.

## Troubleshooting

| Symptom | Meaning | Action |
|---|---|---|
| Missing read permission | Agent credential lacks a required scope | Follow lark-shared Agent permission guidance |
| Empty read-user list | No user has read the message, or sender/time constraints are not met | Verify chat membership, the message sender, and the seven-day window |

## References

- [lark-im](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
