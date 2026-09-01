# mail +thread-modify

`mail +thread-modify` is the preferred shortcut for changing labels or folder placement on existing mail threads.

Use it when the operation targets concrete `thread_id` values from `+triage`, `+message`, `+thread`, or search results. If the operation targets concrete `message_id` values instead of the whole thread, use [`mail +message-modify`](./lark-mail-message-modify.md).

## Common Commands

```bash
lark-cli mail +thread-modify --thread-ids <thread_id1>,<thread_id2> --add-label-ids unread
lark-cli mail +thread-modify --thread-ids <thread_id> --remove-label-ids FLAGGED
lark-cli mail +thread-modify --thread-ids <thread_id> --add-folder archive
lark-cli mail +thread-modify --mailbox shared@example.com --thread-ids <thread_id> --add-folder folder_xxx
lark-cli mail +thread-modify --as bot --mailbox user@example.com --thread-ids <thread_id> --add-folder archive
lark-cli mail +thread-modify --thread-ids <thread_id> --add-label-ids custom_label_id --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the threads. Defaults to `me`; with `--as bot`, pass an explicit mailbox email. |
| `--thread-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. |
| `--add-label-ids` | No | Adds labels. System labels `unread`, `important`, `other`, `flagged`, `read_receipt_request` normalize to upper case. |
| `--remove-label-ids` | No | Removes labels. Cannot overlap with `--add-label-ids`. |
| `--add-folder` | No | Moves to one folder. `inbox`, `sent`, `spam`, `archive`, `archived` normalize to system folder IDs. |

At least one of `--add-label-ids`, `--remove-label-ids`, or `--add-folder` is required.

`TRASH` is intentionally rejected by this shortcut. Use [`mail +thread-trash`](./lark-mail-thread-trash.md) with `--yes` for soft deletion.

## Behavior

- Thread IDs must come from real query results such as `+triage`, `+message`, `+thread`, thread lists, or search.
- Thread IDs are locally validated, de-duplicated in first-seen order, and sent in batches of 20.
- Custom label IDs are checked with `labels.get`; custom folder IDs are checked with `folders.get`.
- If no label or folder operation is requested, validation fails before any POST request.
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

Use raw `mail user_mailbox.threads batch_modify` only when you need a request shape that the shortcut intentionally does not expose, or when reproducing backend/API behavior exactly for diagnostics.

## Related Commands

- `lark-cli mail +triage` - Browse message summaries and obtain `thread_id` values.
- `lark-cli mail +thread` - Read a full thread.
- `lark-cli mail +message-modify` - Modify concrete messages by `message_id`.
- `lark-cli mail +thread-trash` - Soft-delete threads by `thread_id`.
