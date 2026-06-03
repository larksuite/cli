# im +chat-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

List chats the current user (or bot, with `--as bot`) is a member of. **Not a search API — there is no `--query` parameter; the call always returns the full member list, paginated.** For keyword-based lookup (e.g. find a group by name or by member), use [`+chat-search`](lark-im-chat-search.md) instead.

**Defaults to groups only**; pass `--types=p2p,group` (or `--types p2p --types group`) to also include p2p single chats (user identity only — see ["Bot identity and p2p"](#bot-identity-and-p2p)). Supports pagination, sort order, and (user identity only) muted-chat filtering.

This skill maps to the shortcut: `lark-cli im +chat-list` (internally calls `GET /open-apis/im/v1/chats`).

## Commands

```bash
# List the user's chats (default sort: ByCreateTimeAsc)
lark-cli im +chat-list

# Sort by recent activity (most recently active first)
lark-cli im +chat-list --sort-type ByActiveTimeDesc

# Limit page size
lark-cli im +chat-list --page-size 50

# Pagination
lark-cli im +chat-list --page-token "xxx"

# Drop muted chats (user identity only)
lark-cli im +chat-list --exclude-muted

# JSON output
lark-cli im +chat-list --format json

# Preview the request without executing it
lark-cli im +chat-list --dry-run

# Include p2p single chats (user identity only) — comma form
lark-cli im +chat-list --as user --types p2p,group

# Same, using repeat flag instead of CSV
lark-cli im +chat-list --as user --types p2p --types group

# Only p2p single chats (user identity only)
lark-cli im +chat-list --as user --types p2p
```

## Parameters

| Parameter | Required | Limits | Description |
|------|------|------|------|
| `--user-id-type <type>` | No | `open_id` (default), `union_id`, `user_id` | ID type used for `owner_id` in the response |
| `--types <strings>` | No | `group`, `p2p` (comma-separated or repeated) | Chat types to include. Omitted = groups only (backward compatible). `p2p` requires user identity (`--as user`); under `--as bot`, `--types=p2p` alone is rejected and `--types=p2p,group` is silently downgraded to `group` |
| `--sort-type <type>` | No | `ByCreateTimeAsc` (default), `ByActiveTimeDesc` | Result ordering |
| `--page-size <n>` | No | 1-100, default 20 | Number of results per page |
| `--page-token <token>` | No | - | Pagination token from the previous response |
| `--exclude-muted` | No | User identity only | Drop chats the current user has muted (do-not-disturb). Under `--as bot`, the flag is silently inactive; see "Filtering muted chats" below |
| `--format json` | No | - | Output as JSON |
| `--dry-run` | No | - | Preview the request without executing it |

> **Note:** Supports both `--as user` (default) and `--as bot`. When using bot identity, the app must have bot capability enabled.

## Output Fields

| Field | Description |
|------|------|
| `chat_id` | Chat ID (`oc_xxx` format) |
| `name` | Chat name |
| `description` | Chat description |
| `owner_id` | Owner ID (type controlled by `--user-id-type`) |
| `external` | Whether the chat is external |
| `chat_status` | Chat status (`normal` / `dissolved` / `dissolved_save`) |
| `chat_mode` | Chat mode discriminator: `group` (regular) / `topic` (topic group) / `p2p` (single chat) |
| `p2p_target_type` | Peer type, e.g., `user` |
| `p2p_target_id` | Peer ID (type controlled by `--user-id-type`) |

## Including p2p single chats

Default behavior lists groups only — same as before this feature. To include p2p, pass `--types`:

| User intent | Call | Identity |
|---|---|---|
| "list my groups" / 我的群 / 我加入了哪些群 | (default, omit `--types`) | user or bot |
| "list my p2p chats" / 我的单聊 / 我跟谁有 1v1 | `--types p2p` | **user only** |
| "all my chats" / 全部聊天 / 所有会话 (ambiguous) | `--types p2p,group` | **user only** |

For p2p rows in the response: `name` is the peer's display name, `owner_id` follows group semantics, `chat_mode = "p2p"`, and `p2p_target_type` / `p2p_target_id` identify the peer.

## Bot identity and p2p

`tenant_access_token` cannot list p2p chats — to protect user privacy, bot identity is not permitted to enumerate p2p single chats. Behavior under `--as bot`:

- `--as bot --types=p2p` → rejected at validation time with an actionable error; no request is sent.
- `--as bot --types=p2p,group` → CLI strips `p2p` and sends `types=group`. Request proceeds; only groups are returned. The strip is a **request-level adjustment**, surfaced two ways so neither humans nor agents miss it:
    - **stderr**: `warning: bot_strip_p2p: To protect user privacy, bot identity cannot list p2p chats; --types=p2p,group was sent as types=group. Use --as user to include p2p.` (matches the `warning: <code>: <message>` convention in `shortcuts/common/runner.go`)
    - **stdout JSON**: a top-level `notices` array gains a structured entry:
      ```json
      {
        "chats": [...],
        "notices": [
          { "code": "bot_strip_p2p", "message": "To protect user privacy, bot identity cannot list p2p chats; …" }
        ]
      }
      ```
    - The `filter` slot stays scoped to `--exclude-muted`; `notices` is a separate top-level key, so the two never collide and no priority is needed when both fire.
    - DryRun emits the same stderr warning so a previewed request truthfully reflects what Execute will send (parity with `shortcuts/drive/drive_search.go`).
- `--as bot --types=group` → accepted, returns groups normally.
- `--as bot` (no `--types`) → unchanged, returns groups.

To include p2p single chats, switch to user identity: `--as user --types=p2p,group`.

## Filtering muted chats

`--exclude-muted` (user identity only) drops chats the current user has set to do-not-disturb. After the list call, the CLI batches the page's chat_ids through `POST /open-apis/im/v1/chat_user_setting/batch_get_mute_status` and filters client-side. Under `--as bot`, the mute API is UAT-only and the filter is silently skipped.

When the flag is set, the JSON envelope gains a `filter` sub-object (absent otherwise, so existing consumers are unaffected); `fetched_count == returned_count + filtered_count` always holds:

```json
{
  "chats": [...],
  "filter": {
    "applied": "exclude_muted",
    "fetched_count": 20,
    "returned_count": 17,
    "filtered_count": 3,
    "hint": "Filtered out 3 muted chat(s) on this page (17 remaining); use --page-token to fetch more."
  }
}
```

## Usage Scenarios

### Scenario 1: List my recent chats

```bash
lark-cli im +chat-list --sort-type ByActiveTimeDesc --page-size 10
```

### Scenario 2: List my non-muted chats sorted by activity

```bash
lark-cli im +chat-list --sort-type ByActiveTimeDesc --exclude-muted
```

### Scenario 3: Iterate all my chats programmatically

```bash
TOKEN=""
while :; do
  RESP=$(lark-cli im +chat-list --page-size 100 --page-token "$TOKEN" --format json)
  echo "$RESP" | jq -r '.data.chats[].chat_id'
  HAS_MORE=$(echo "$RESP" | jq -r '.data.has_more')
  [ "$HAS_MORE" = "true" ] || break
  TOKEN=$(echo "$RESP" | jq -r '.data.page_token')
done
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `--page-size must be an integer between 1 and 100` | page-size is out of range or not an integer | Use an integer between 1 and 100 |
| Permission denied (99991672) | The bot app does not have `im:chat:read` TAT permission enabled | Enable the permission for the app in the Open Platform console |
| Permission denied (99991679) with `--as user` | UAT is not authorized for `im:chat:read` | Run `lark-cli auth login --scope "im:chat:read"` |
| `Bot ability is not activated` (232025) | The app does not have bot capability enabled | Enable bot capability in the Open Platform console |
| `--exclude-muted` returns all chats unfiltered and `hint` says "no effect under bot identity" | Running under `--as bot` (mute API is UAT-only) | Switch to `--as user` for mute filtering |
| `--types=p2p (single chats) is only supported with user identity` | `--as bot` + `--types=p2p` (single-value only; mixed `--types=p2p,group` is downgraded to `group` and surfaces a `bot_strip_p2p` notice via stderr + `outData["notices"]` — see "Bot identity and p2p") | Use `--as user`, or include `group` in `--types` (the bot proceeds with `group` only and emits the `bot_strip_p2p` notice) |

> Full error message of the row above: `--types=p2p (single chats) is only supported with user identity (--as user). To protect user privacy, bot identity cannot list p2p chats. Use --as user, or include "group" in --types.`
