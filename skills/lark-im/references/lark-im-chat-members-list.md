# im +chat-members-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

List all members (users and bots) of a group chat in **one call**, returning results split into `users[]` and `bots[]` buckets with totals. Both user and bot identity are supported — the caller must be a member of the target chat.

This skill maps to the shortcut: `lark-cli im +chat-members-list` (internally calls `GET /open-apis/im/v1/chats/{chat_id}/members/list`).

## Commands

```bash
# List all members (users + bots) of a chat
lark-cli im +chat-members-list --chat-id oc_xxx

# List only bot members
lark-cli im +chat-members-list --chat-id oc_xxx --member-types bot

# List only user members
lark-cli im +chat-members-list --chat-id oc_xxx --member-types user

# Use union_id for member IDs in the response
lark-cli im +chat-members-list --chat-id oc_xxx --member-id-type union_id

# Control page size
lark-cli im +chat-members-list --chat-id oc_xxx --page-size 50

# Paginate to the next page
lark-cli im +chat-members-list --chat-id oc_xxx --page-token "xxx"

# Automatically paginate through all pages (accumulates users[] + bots[])
lark-cli im +chat-members-list --chat-id oc_xxx --page-all

# Limit max pages when using --page-all (default 20, max 1000)
lark-cli im +chat-members-list --chat-id oc_xxx --page-all --page-limit 5

# JSON output
lark-cli im +chat-members-list --chat-id oc_xxx --format json

# Preview the request without executing it
lark-cli im +chat-members-list --chat-id oc_xxx --dry-run

# Use bot identity
lark-cli im +chat-members-list --chat-id oc_xxx --as bot
```

## When to use (recipes)

You only have a group **name**, not a `chat_id`? Resolve it first, then list members:

```bash
# Step 1: name -> chat_id (one identity is enough; don't re-search with the other)
lark-cli im +chat-search --query "<group name>" --format json   # read chat_id from results
# Step 2: list members
lark-cli im +chat-members-list --chat-id <chat_id>
```

- **"List all members, including bots"** → call `+chat-members-list` with **no** `--member-types`. One call returns both `users[]` and `bots[]`. Do not also call `--member-types user` or `--member-types bot` separately, and do not re-run with a different `--member-id-type` to "fill in" extra IDs.
- **"Which bots are in the group?"** → add `--member-types bot`. **"Which users?"** → `--member-types user`. Filtering is what isolates one type; don't post-filter a full result by hand. **Answer from the filtered result.** Once `--member-types bot` returns (`users[]` will be empty, only `bots[]` is populated), that *is* the answer — do **not** follow it with an unfiltered `+chat-members-list` call "to be complete". A second, unfiltered query defeats the filter and over-exposes the very members the user did not ask about.
- **This shortcut replaces the `im chat.members get` / `im chat.members bots` Meta APIs.** It already returns both buckets in one call — do **not** fall back to those raw Meta APIs, and do not cross-check this shortcut's output against them.

> **CAUTION — don't over-fetch.** Once a single `+chat-members-list` call returns, you have the complete answer for that page set. Resist re-querying with `--member-id-type user_id` "just in case": `user_id` requires the extra `contact:user.employee_id:readonly` scope and will error if it is not granted. For plain member listing, the default `open_id` is sufficient — only use `user_id` when the user explicitly needs employee IDs.

## Parameters

| Parameter | Required | Limits | Description |
|------|------|------|------|
| `--chat-id <id>` | **Yes** | `oc_xxx` format | Group chat ID to query |
| `--member-types <strings>` | No | `user`, `bot` (repeatable) | Filter by member type. Omitted = both user and bot returned |
| `--member-id-type <type>` | No | `open_id` (default), `union_id`, `user_id` | ID type used for `member_id` in the response |
| `--page-size <n>` | No | 1-100, default 20 | Number of members per page |
| `--page-token <token>` | No | - | Pagination token from the previous response |
| `--page-all` | No | - | Auto-paginate through all pages, accumulating `users[]` + `bots[]` |
| `--page-limit <n>` | No | 1-1000, default 20 | Max pages when `--page-all` is enabled |
| `--format json` | No | - | Output as JSON |
| `--dry-run` | No | - | Preview the request without executing it |

> **Note:** Supports both `--as user` (default) and `--as bot`. The caller must be a member of the target chat regardless of identity.

## Output

The output is split into two buckets regardless of which types are present. Empty buckets are always rendered as `[]` for stable downstream parsing.

```json
{
  "data": {
    "users": [
      {
        "member_id": "ou_xxx",
        "name": "Alice",
        "tenant_key": "736588c9xxx"
      }
    ],
    "bots": [
      {
        "member_id": "ou_yyy",
        "name": "MyBot",
        "app_id": "cli_zzz",
        "tenant_key": "736588c9xxx"
      }
    ],
    "user_total": 5,
    "bot_total": 1,
    "has_more": false,
    "page_token": "",
    "truncations": []
  }
}
```

To get a flat list of all members (users and bots combined):

```bash
lark-cli im +chat-members-list --chat-id oc_xxx --format json | jq '.data.users + .data.bots'
```

To extract all member IDs:

```bash
lark-cli im +chat-members-list --chat-id oc_xxx --page-all --format json | \
  jq '[(.data.users + .data.bots)[].member_id]'
```

## Notes

- **Format and bucket completeness**: Only `--format json` and `--format pretty` render both `users[]` and `bots[]` buckets in full. `--format csv`, `--format table`, and `--format ndjson` use the generic flatten path which only renders the `users[]` bucket — `bots[]` will be silently dropped. Use `--format json` or `--format pretty` (the default) whenever bot member data is needed.

- **`member_id_type=user_id` and cross-tenant members**: When `--member-id-type user_id` is used and the chat contains cross-tenant members (external members from another tenant), their `member_id` field may be omitted in the response. Use `open_id` (default) or `union_id` if you need a stable identifier for all members including external ones.

- **`truncations` non-empty = server-side truncation**: If the response `truncations` array is non-empty, the server truncated that member type's bucket and not all members are returned. A warning is emitted to stderr for each truncated type. Use `--page-all` to accumulate across pages, or reduce `--page-size` if individual pages are hitting the truncation limit.

- **Bot bucket is always complete**: The bots in a group chat are typically few in number and the server always returns the full bot list without truncation. Bot bucket truncation warnings are rare and indicate an unusually large number of bots.

- **Caller must be in the chat**: Both user and bot identity require the caller to be a member of the target chat. If the caller is not a member, the API returns a permission error.

## Common Errors

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `--page-size must be an integer between 1 and 100` | page-size out of range | Use an integer between 1 and 100 |
| `--page-limit must be an integer between 1 and 1000` | page-limit out of range (when using `--page-all`) | Use an integer between 1 and 1000 |
| `--member-types contains invalid value "xxx"` | Unknown member type | Use `user` or `bot` only |
| Permission denied (99991672) | Bot app lacks `im:chat.members:read` scope | Enable the permission in the Open Platform console |
| Permission denied (99991679) with `--as user` | UAT not authorized for `im:chat.members:read` | Run `lark-cli auth login --scope "im:chat.members:read"` |
| Caller not in chat (error 102021) | The caller (user or bot) is not a member of the target chat | Add the caller to the chat first, or use an identity that is already a member |
