# +feed-group-list-item

> Shortcut for `lark-cli im +feed-group-list-item`. List the feed cards inside one feed group (tag), enriched with a readable `chat_name`.

`+feed-group-list-item` is the only CLI surface for the `feed.groups.list_item` read API — there is no raw `feed.groups list_item` command. It resolves a human-readable `chat_name` for every feed card it returns: a v1 feed card's `feed_id` is always a chat ID (`oc_xxx`), so the shortcut issues a follow-up `POST /open-apis/im/v1/chats/batch_query` and injects `chat_name` into each entry of both `items[]` and `deleted_items[]`.

## Identity

User-only. Run with `--as user`.

## Scopes

Because chat-name resolution always runs, this shortcut needs **two** user scopes unconditionally:

- `im:feed_group_v1:read` — to read the items
- `im:chat:read` — to resolve names

`chat_name` resolution always runs, so there is no single-scope, un-enriched path. For the other raw `feed.groups.*` methods, see [lark-im-feed-groups.md](lark-im-feed-groups.md).

## Usage

```bash
# First page, enriched with chat names
lark-cli im +feed-group-list-item --as user --feed-group-id ofg_xxx

# Auto-paginate through everything within a time window
lark-cli im +feed-group-list-item --as user --feed-group-id ofg_xxx \
  --page-all --start-time 1767196800000 --end-time 1767200000000
```

## Flags

| Flag | Required | Description |
|---|---|---|
| `--feed-group-id` | Yes | Feed group ID (`ofg_xxx`); path parameter |
| `--page-size` | No | Records per page, 1–50 (default 50) |
| `--page-token` | No | Continuation token for a specific page |
| `--page-all` | No | Auto-paginate and merge all pages |
| `--page-limit` | No | Max pages when `--page-all` is set, 1–1000 (default 20) |
| `--start-time` | No | Update-time window start (Unix milliseconds as a decimal string) |
| `--end-time` | No | Update-time window end (Unix milliseconds as a decimal string) |

When `--page-token` is set explicitly, it wins over `--page-all` (you get exactly that page).

## Output

JSON keeps the raw envelope and adds `chat_name` to each resolvable item:

```json
{
  "items": [
    { "feed_id": "oc_abc", "feed_type": "chat", "update_time": "1767196800000", "chat_name": "Release Team" }
  ],
  "deleted_items": [
    { "feed_id": "oc_def", "feed_type": "chat", "update_time": "1767196800000", "chat_name": "Old Channel" }
  ],
  "page_token": "",
  "has_more": false
}
```

A feed card whose chat cannot be resolved (soft-deleted or no permission) simply omits `chat_name` — the command still exits 0. p2p (direct) chats also omit `chat_name`: the server returns an empty `name` for them (the client UI shows the partner's display name instead); if a label is needed, fetch the chat via `chats/batch_query`, read `p2p_target_id`, and resolve it with a contact lookup.

## See also

- [lark-im-feed-groups.md](lark-im-feed-groups.md) — raw `feed.groups.*` APIs, enums, and rule guidance
- [lark-im-feed-group-list.md](lark-im-feed-group-list.md) — list your feed groups
- [lark-im-feed-group-query-item.md](lark-im-feed-group-query-item.md) — look up specific feed cards by ID
