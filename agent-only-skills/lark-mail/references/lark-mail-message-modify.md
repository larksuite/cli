# mail +message-modify

`mail +message-modify` is the preferred shortcut for changing labels, read-state labels, or folder placement on existing messages.

Use it instead of raw `user_mailbox.messages batch_modify` when the operation targets concrete `message_id` values from `+triage`, `+message`, or `+messages`.

## Common Commands

```bash
lark-cli mail +message-modify --message-ids <id1>,<id2> --add-label-ids unread
lark-cli mail +message-modify --message-ids <id> --remove-label-ids FLAGGED
lark-cli mail +message-modify --message-ids <id> --add-folder archive
lark-cli mail +message-modify --mailbox shared@example.com --message-ids <id> --add-folder folder_xxx
lark-cli mail +message-modify --message-ids <id> --add-label-ids custom_label_id --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the messages. Defaults to `me`. |
| `--message-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |
| `--add-label-ids` | No | Adds labels. System labels `unread`, `important`, `other`, `flagged` normalize to upper case. |
| `--remove-label-ids` | No | Removes labels. Cannot overlap with `--add-label-ids`. |
| `--add-folder` | No | Moves to one folder. `inbox`, `sent`, `spam`, `archive`, `archived` normalize to system folder IDs. |

`TRASH` is intentionally rejected by this shortcut. Use `mail +message-trash --message-ids <id> --yes` for soft deletion.

## Behavior

- Message IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- Custom label IDs are checked with `labels.get`; custom folder IDs are checked with `folders.get`.
- If no label or folder operation is requested, the command succeeds locally, emits all message IDs as `success_message_ids`, and makes no POST request.
- Single batch POST failures mark every message in that batch with the same failure reason; later batches still run.
- JSON output is intentionally compact:

```json
{
  "success_message_ids": ["id1"],
  "failed_message_ids": [
    {"message_id": "id2", "reason": "api error"}
  ]
}
```

## When Raw API Is Still Appropriate

Use raw `mail user_mailbox.messages batch_modify` only when you need a request shape that the shortcut intentionally does not expose, or when reproducing backend/API behavior exactly for diagnostics.
