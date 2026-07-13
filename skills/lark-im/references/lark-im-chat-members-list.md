# im +chat-members-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

List the members of a chat. Users and bots are returned in **separate buckets** — `users[]` and `bots[]` — with per-bucket totals (`user_total` / `bot_total`). Use `--member-types` to return only one kind.

This skill maps to the shortcut: `lark-cli im +chat-members-list` (internally calls `GET /open-apis/im/v1/chats/{chat_id}/members/list`).

## Commands

```bash
# Single page (default)
lark-cli im +chat-members-list --chat-id oc_xxx

# Only users, or only bots
lark-cli im +chat-members-list --chat-id oc_xxx --member-types user
lark-cli im +chat-members-list --chat-id oc_xxx --member-types user,bot

# Walk every page (capped by --page-limit; 0 = unlimited)
lark-cli im +chat-members-list --chat-id oc_xxx --page-all --page-limit 0

# Resume from a specific cursor (single page; --page-all is ignored)
lark-cli im +chat-members-list --chat-id oc_xxx --page-token "xxx"

# JSON output / preview the request
lark-cli im +chat-members-list --chat-id oc_xxx --format json
lark-cli im +chat-members-list --chat-id oc_xxx --dry-run
```

## Parameters

| Parameter | Required | Limits | Description |
|------|------|------|------|
| `--chat-id <id>` | Yes | `oc_xxx` | Target chat |
| `--member-types <strings>` | No | `user`, `bot` (comma-separated or repeated) | Member types to return. Omitted = all |
| `--member-id-type <type>` | No | `open_id` (default), `union_id`, `user_id` | ID type for `member_id` in the response |
| `--page-size <n>` | No | 1-100, default 20 | Results per page. With `--page-all` and no explicit `--page-size`, the max (100) is used automatically to minimize round-trips |
| `--page-token <token>` | No | - | Pagination cursor; **implies a single-page fetch** (disables auto-pagination) |
| `--page-all` | No | - | Automatically walk every page (capped by `--page-limit`) |
| `--page-limit <n>` | No | default 10, `0` = unlimited | Max pages to fetch with `--page-all` |
| `--page-delay <ms>` | No | default 200, `0` = no delay | Delay between pages during `--page-all` (throttle to avoid rate limits on large lists) |
| `--format json` | No | - | Output as JSON |
| `--dry-run` | No | - | Preview the request without executing it |

> Supports both `--as user` (default) and `--as bot`. The caller must be in the target chat, and must belong to the same tenant for internal chats.

## Output Fields

| Field | Description |
|------|------|
| `chat_id` | The queried chat ID |
| `users` | Array of user members (`member_id`, `name`, `tenant_key`, …) |
| `bots` | Array of bot members (`member_id`, `app_id`, `name`, …) |
| `user_total` / `bot_total` | Server-reported totals for each bucket |
| `truncations` | Non-empty when the server **capped a bucket** due to security config — see below |
| `has_more` / `page_token` | Paging signals from the final page fetched |

## Truncation: the result may be incomplete

The server applies a security cap to large member lists. When a bucket is capped, the response carries a `truncations[]` entry (e.g. `[{"limit": 100, "member_type": "user"}]`) **on the final page only**. The shortcut surfaces this two ways so it is never missed:

- **stderr**: `⚠️  member list truncated by server security config: user bucket capped at 100 — the list is INCOMPLETE.`
- **stdout JSON**: the `truncations` array is preserved verbatim in the output.

A truncated result is *not* fixable by paging further — it is a server-side cap. Treat `users`/`bots` as a partial list whenever `truncations` is non-empty.

## Pagination notes

- Default fetches a single page. Pass `--page-all` to drain every page.
- With `--page-all` and no explicit `--page-size`, the shortcut uses the maximum page size (100) so a full walk takes the fewest round-trips. An explicit `--page-size` is always honored.
- `--page-all` sleeps `--page-delay` ms (default 200) between pages to avoid hammering the API when a tenant has no server-side member cap and the list spans many pages. Set `--page-delay 0` to disable.
- `--page-all` stops at `--page-limit` pages (default 10). When it stops early, `has_more` stays `true` so you know the result is incomplete; re-run with `--page-limit 0` for everything.
- `--page-token` and `--page-all` together: `--page-token` wins (single-page fetch from the supplied cursor); a stderr warning is emitted.
- Across pages, `users[]` and `bots[]` are concatenated; `truncations` / `has_more` / `page_token` come from the last page fetched.

## Common Errors and Troubleshooting

| Symptom | Root Cause |   | Solution |
|---------|---------|---|---------|
| `--chat-id is required` | `--chat-id` omitted |   | Provide the `oc_xxx` chat ID |
| `--page-size must be an integer between 1 and 100` | out of range |   | Use 1-100 |
| `--member-types contains invalid value` | value other than `user`/`bot` |   | Use `user`, `bot`, or both |
| Permission denied | missing `im:chat.members:read` |   | Bot: enable the scope in the console. User: `lark-cli auth login --scope "im:chat.members:read"` |
