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
