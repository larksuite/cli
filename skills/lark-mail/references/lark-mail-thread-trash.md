# mail +thread-trash

`mail +thread-trash` is the preferred shortcut for soft-deleting existing mail conversations when you have `thread_id` values.

Use it after obtaining real `thread_id` values from a conversation list, search, `+triage`, `+message`, or `+thread`, and after the user has confirmed the deletion preview. If the operation targets concrete `message_id` values rather than whole conversations, use [`mail +message-trash`](./lark-mail-message-trash.md).

## Common Commands

```bash
lark-cli mail +thread-trash --thread-ids <thread_id1>,<thread_id2> --yes
lark-cli mail +thread-trash --mailbox shared@example.com --thread-ids <thread_id> --yes
lark-cli mail +thread-trash --thread-ids <thread_id1> --thread-ids <thread_id2> --dry-run
```

## Flags

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the threads. Defaults to `me`. |
| `--thread-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. Values are trimmed, empty values are ignored, and duplicates are removed in first-seen order. |
| `--yes` | Yes for execution | Required by the high-risk write confirmation framework. |

## Behavior

- Thread IDs are locally trimmed, de-duplicated in first-seen order, and submitted in one `batch_trash` request.
- The shortcut calls `POST /open-apis/mail/v1/user_mailboxes/<mailbox>/threads/batch_trash`.
- Backend diagnostics such as permission failures, missing threads, conflicts, or network errors are preserved in the structured CLI error.
- JSON output is intentionally request-side only:

```json
{
  "operation": "thread_trash",
  "mailbox": "me",
  "submitted_thread_ids": ["thread_id1", "thread_id2"],
  "submitted_count": 2
}
```

`submitted_count` is the number of IDs submitted by the CLI. It does not mean every thread was trashed by the server. The shortcut does not output `trashed_count`, `failed_ids`, or per-thread results.

## Getting Thread IDs

If you do not already have thread IDs, first use a conversation listing/search flow such as `mail +triage`, `mail user_mailbox.messages list`, or `mail +message` to obtain `thread_id` values.

## When Raw API Is Still Appropriate

Use raw `mail user_mailbox.threads batch_trash` only when reproducing backend/API behavior exactly for diagnostics. For normal conversation soft deletion, prefer this shortcut because it handles validation, compact output, and `--yes` confirmation consistently.
