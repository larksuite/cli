# im +feed-shortcut-remove

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

This skill maps to shortcut: `lark-cli im +feed-shortcut-remove`. Underlying API: `POST /open-apis/im/v2/feed_shortcuts/remove`.

## What it does

Removes one or more chats from the **current user's** feed shortcuts.

- Only **CHAT-type** shortcuts are supported (`feed_card_id` must be an `oc_xxx`).
- Batch up to **10 chat IDs per call**.
- Currently only supports **user identity** (`--as user`).
- Removing a chat that is not currently in the shortcut list is idempotent success: the call returns `ok:true`, `failure_count=0`, and no `failed_shortcuts` entry for that chat.

## Commands

```bash
# Remove a single feed shortcut
lark-cli im +feed-shortcut-remove --as user --chat-id oc_xxx

# Remove multiple feed shortcuts in one call
lark-cli im +feed-shortcut-remove --as user --chat-id oc_a,oc_b
lark-cli im +feed-shortcut-remove --as user --chat-id oc_a --chat-id oc_b

# Preview the request
lark-cli im +feed-shortcut-remove --as user --chat-id oc_xxx --dry-run
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `--chat-id <oc_xxx>` | yes | open_chat_id to remove from feed shortcuts; repeatable or comma-separated; max 10 per call |
| `--as user` | yes | Server only accepts user_access_token for this API |

## Response

The response uses the same batch ledger as [`+feed-shortcut-create`](lark-im-feed-shortcut-create.md#response): `total`, `success_count`, `failure_count`, `succeeded_shortcuts`, and `failed_shortcuts`. A non-empty `failed_shortcuts` is a partial failure: stdout carries `ok:false` with the full ledger and the process exits non-zero (currently exit `1`).

## Permissions

- Required scope: `im:feed.shortcut:write`
- Only available with user identity (`--as user`).

## Note

- To see what is currently in the shortcut list before removing, run [`+feed-shortcut-list`](lark-im-feed-shortcut-list.md). Use `--no-detail` when you only need the `feed_card_id` values.
