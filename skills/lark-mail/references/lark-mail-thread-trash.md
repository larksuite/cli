# mail +thread-trash

`mail +thread-trash` is the preferred shortcut for soft-deleting existing mail threads.

Use it after obtaining real `thread_id` values from `+triage`, `+message`, `+thread`, or search results, and after the user has confirmed the deletion preview. If the operation targets concrete `message_id` values instead of the whole thread, use [`mail +message-trash`](./lark-mail-message-trash.md).

## Common Commands

```bash
lark-cli mail +thread-trash --thread-ids <thread_id1>,<thread_id2> --yes
lark-cli mail +thread-trash --mailbox shared@example.com --thread-ids <thread_id> --yes
lark-cli mail +thread-trash --as bot --mailbox user@example.com --thread-ids <thread_id> --yes
lark-cli mail +thread-trash --thread-ids <thread_id1> --thread-ids <thread_id2> --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the threads. Defaults to `me`; with `--as bot`, pass an explicit mailbox email. |
| `--thread-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |
| `--yes` | Yes for execution | Required by the high-risk write confirmation framework. |

## Behavior

- Thread IDs must come from real query results such as `+triage`, `+message`, `+thread`, thread lists, or search.
- Soft deletion is a high-risk write. Show a deletion preview with affected count and key message summaries first; execute with `--yes` only after confirmation.
- Thread IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- Single batch POST failures mark every thread in that batch with the same failure reason; later batches still run.
- JSON output is intentionally compact:

```json
{
  "success_thread_ids": ["thread_id1"],
  "failed_thread_ids": [
    {"thread_id": "thread_id2", "reason": "api error"}
  ]
}
```

## When Raw API Is Still Appropriate

Use raw `mail user_mailbox.threads batch_trash` only when reproducing backend/API behavior exactly for diagnostics. For normal soft deletion, prefer this shortcut because it handles validation, batching, compact output, and `--yes` confirmation consistently.

## Related Commands

- `lark-cli mail +triage` - Browse message summaries and obtain `thread_id` values.
- `lark-cli mail +thread` - Read a full thread.
- `lark-cli mail +message-trash` - Soft-delete concrete messages by `message_id`.
- `lark-cli mail +thread-modify` - Modify thread labels or folder placement by `thread_id`.
