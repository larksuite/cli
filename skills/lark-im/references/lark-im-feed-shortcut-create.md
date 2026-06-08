# im +feed-shortcut-create

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

This skill maps to shortcut: `lark-cli im +feed-shortcut-create`. Underlying API: `POST /open-apis/im/v2/feed_shortcuts`.

## What it does

Adds one or more chats to the **current user's** feed shortcuts — equivalent to right-clicking a chat in the Feishu client and pinning it to the feed sidebar.

- Only **CHAT-type** shortcuts are exposed by the OpenAPI gateway right now (`feed_card_id` must be an `oc_xxx` open_chat_id).
- Batch up to **10 chat IDs per call**; pass more by issuing multiple calls.
- Currently only supports **user identity** (`--as user`); bot identity is not allowed by the server.
- If you only know a group name, resolve its `oc_xxx` first with `im +chat-search` or `im +chat-list`.

## Commands

```bash
# Add a single chat as a feed shortcut (defaults to head/top insertion)
lark-cli im +feed-shortcut-create --as user --chat-id oc_xxx

# Add multiple chats; comma-separated or repeated flag both work
lark-cli im +feed-shortcut-create --as user --chat-id oc_a,oc_b,oc_c
lark-cli im +feed-shortcut-create --as user --chat-id oc_a --chat-id oc_b

# Append at the bottom of the shortcut list instead of the top
lark-cli im +feed-shortcut-create --as user --chat-id oc_xxx --tail

# Preview the request without sending
lark-cli im +feed-shortcut-create --as user --chat-id oc_xxx --dry-run
```

## Parameters

| Parameter | Default | Description |
|------|------|------|
| `--chat-id <oc_xxx>` | required | open_chat_id to add as a feed shortcut; repeatable or comma-separated; **max 10 per call** |
| `--head` | true (implied) | Insert at the top of the shortcut list; mutually exclusive with `--tail` |
| `--tail` | false | Append at the bottom of the shortcut list |
| `--as user` | required | Server only accepts user_access_token for this API |

## Response

The response is a batch ledger. A full success exits `0` with `ok:true`. Any non-empty `failed_shortcuts` is a partial failure: the process exits non-zero (currently exit `1`), stdout carries `ok:false`, and the full ledger remains machine-readable:

| Field | Meaning |
|------|------|
| `total` | Number of requested shortcuts |
| `success_count` | Number of requested shortcuts not reported in `failed_shortcuts` |
| `failure_count` | Number of requested shortcuts reported as failed; `failed_shortcuts` preserves the raw server failure list |
| `succeeded_shortcuts` | Requested shortcut entries that succeeded |
| `failed_shortcuts` | Per-item failures returned by the server, enriched with `reason_label` |

The shortcut adds a `reason_label` field next to each numeric `reason`:

| `reason` | `reason_label` | Meaning |
|---:|------|------|
| 1 | `no_permission` | User has no permission on the feed card |
| 2 | `invalid_item` | `feed_card_id` is invalid or type doesn't match |
| 3 | `has_pending_delete` | The chat is being deleted |
| 4 | `type_not_support` | Type is not whitelisted (only CHAT is open now) |
| 5 | `internal_error` | Server internal error |

Example:

```json
{
  "ok": false,
  "data": {
    "total": 2,
    "success_count": 1,
    "failure_count": 1,
    "succeeded_shortcuts": [
      { "feed_card_id": "oc_good", "type": 1 }
    ],
    "failed_shortcuts": [
      {
        "shortcut": { "feed_card_id": "oc_bad", "type": 1 },
        "reason": 2,
        "reason_label": "invalid_item"
      }
    ]
  }
}
```

## Permissions

- Required scope: `im:feed.shortcut:write`
- Only available with user identity (`--as user`). The CLI will reject `--as bot` for this shortcut.

## Note

- The shortcut list is **per user**: the call adds shortcuts for the currently authenticated user only.
- Adding the same chat twice is **idempotent at the user level** (re-adding an existing shortcut is a no-op rather than an error).
- Scripts should check the process exit code, top-level `ok`, and ledger counts. Partial failures intentionally keep machine-readable success and failure details on stdout.
- To inspect the current shortcut list, use [`+feed-shortcut-list`](lark-im-feed-shortcut-list.md). To remove a shortcut, use [`+feed-shortcut-remove`](lark-im-feed-shortcut-remove.md).
