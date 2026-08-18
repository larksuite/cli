# mail +thread-trash

`mail +thread-trash` is the preferred shortcut for soft-deleting existing mail conversations when you have `thread_id` values.

Use it after obtaining real `thread_id` values from a conversation list, search, `+triage`, `+message`, or `+thread`, and after the user has confirmed the deletion preview. If the operation targets concrete `message_id` values rather than whole conversations, use [`mail +message-trash`](./lark-mail-message-trash.md).

## 命令

```bash
lark-cli mail +thread-trash --thread-ids <thread_id1>,<thread_id2> --yes
lark-cli mail +thread-trash --mailbox shared@example.com --thread-ids <thread_id> --yes
lark-cli mail +thread-trash --thread-ids <thread_id1> --thread-ids <thread_id2> --dry-run
```

## 参数

| Flag | Required | Notes |
| --- | --- | --- |
| `--mailbox` | No | Mailbox that owns the threads. Defaults to `me`. |
| `--thread-ids` | Yes | `string_array`; supports comma-separated values and repeated flags. Up to 20 IDs per command. |
| `--yes` | Yes for execution | Required by the high-risk write confirmation framework. |

## 行为

- Thread IDs are locally trimmed, de-duplicated in first-seen order, and submitted in one request.
- The raw API batch limit is 20 thread IDs; the shortcut validates this before sending.
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

## 原生 API

Use raw `mail user_mailbox.threads batch_trash` only when reproducing backend/API behavior exactly for diagnostics. For normal conversation soft deletion, prefer this shortcut because it handles validation, compact output, and `--yes` confirmation consistently.
