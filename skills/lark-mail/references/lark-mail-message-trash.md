# mail +message-trash

`mail +message-trash` is the preferred shortcut for soft-deleting existing messages.

Use it after obtaining real `message_id` values from `+triage`, `+message`, or `+messages`, and after the user has confirmed the deletion preview.

## Common Commands

```bash
lark-cli mail +message-trash --message-ids <id1>,<id2> --yes
lark-cli mail +message-trash --mailbox shared@example.com --message-ids <id> --yes
lark-cli mail +message-trash --message-ids <id1> --message-ids <id2> --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the messages. Defaults to `me`. |
| `--message-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |
| `--yes` | Yes for execution | Required by the high-risk write confirmation framework. |

## Behavior

- Message IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- The shortcut calls `POST /open-apis/mail/v1/user_mailboxes/<mailbox>/messages/batch_trash` sequentially.
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

Use raw `mail user_mailbox.messages batch_trash` only when reproducing backend/API behavior exactly for diagnostics. For normal soft deletion, prefer this shortcut because it handles validation, batching, compact output, and `--yes` confirmation consistently.
