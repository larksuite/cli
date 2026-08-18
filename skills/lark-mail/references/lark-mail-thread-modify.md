# mail +thread-modify

`mail +thread-modify` is the preferred shortcut for changing labels or folder placement on existing mail conversations when you have `thread_id` values.

Use it instead of raw `user_mailbox.threads batch_modify` for normal thread-level organization. If the operation targets concrete `message_id` values rather than conversations, use [`mail +message-modify`](./lark-mail-message-modify.md).

## Common Commands

```bash
lark-cli mail +thread-modify --thread-ids <thread_id1>,<thread_id2> --add-label-ids unread
lark-cli mail +thread-modify --thread-ids <thread_id> --remove-label-ids FLAGGED
lark-cli mail +thread-modify --thread-ids <thread_id> --add-folder archive
lark-cli mail +thread-modify --mailbox shared@example.com --thread-ids <thread_id> --add-folder folder_xxx
lark-cli mail +thread-modify --thread-ids <thread_id> --add-label-ids custom_label_id --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the threads. Defaults to `me`. |
| `--thread-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. Up to 20 IDs per command. |
| `--add-label-ids` | No | Adds labels. System labels `unread`, `important`, `other`, `flagged`, and `read_receipt_request` normalize to upper case. |
| `--remove-label-ids` | No | Removes labels. Cannot overlap with `--add-label-ids`. |
| `--add-folder` | No | Moves to one folder. `inbox`, `sent`, `spam`, `archive`, and `archived` normalize to system folder IDs. |

At least one of `--add-label-ids`, `--remove-label-ids`, or `--add-folder` is required.

`TRASH` is intentionally rejected by this shortcut. Use [`mail +thread-trash`](./lark-mail-thread-trash.md) with `--yes` for soft deletion.

## Behavior

- Thread IDs are locally trimmed, de-duplicated in first-seen order, and submitted in one request.
- The raw API batch limit is 20 thread IDs; the shortcut validates this before sending.
- JSON output is intentionally request-side only:

```json
{
  "operation": "thread_modify",
  "mailbox": "me",
  "submitted_thread_ids": ["thread_id1", "thread_id2"],
  "submitted_count": 2,
  "add_label_ids": ["UNREAD"],
  "remove_label_ids": [],
  "add_folder": "ARCHIVED"
}
```

`submitted_count` is the number of IDs submitted by the CLI. It does not mean every thread was changed by the server. The shortcut does not output `updated_count`, `failed_ids`, or per-thread results.

## When Raw API Is Still Appropriate

Use raw `mail user_mailbox.threads batch_modify` only when reproducing backend/API behavior exactly for diagnostics or when you need a request shape that the shortcut intentionally does not expose.
