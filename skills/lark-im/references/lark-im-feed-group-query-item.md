# +feed-group-query-item

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-group-query-item`. **Run `lark-cli im +feed-group-query-item --help` for the authoritative flags (`--feed-group-id` / `--feed-id` / `--as`); `--feed-id` is a comma-separated list of chat IDs and `feed_type` is fixed to `chat`.** This file covers only what `--help` cannot.

**User identity only** (`--as user`); bot/tenant tokens are rejected. This is the **only** CLI surface for `feed.groups.batch_query_item` — there is no raw command.

## Gotchas

- **Lightweight ID lookup — prefer it over the list methods when you already hold the IDs.** Use this when you have the `feed_id`s (the `oc_xxx` you passed to `batch_add_item`); reserve `+feed-group-list-item` (paginated, heavier) for discovering IDs you don't have. **No pagination** for this method.
- **`chat_name` enrichment is unconditional → needs a second scope.** Resolves `chat_name` for each card exactly as `+feed-group-list-item` does (follow-up `chats/batch_query`, injected into both `items[]` and `deleted_items[]`). So this needs `im:chat:read` **in addition to** `im:feed_group_v1:read`; there is no un-enriched path.
- **Unresolvable cards silently omit `chat_name`** — soft-deleted or no-permission chats just lack the field; the command still exits 0. **p2p (direct) cards also omit it** (server returns an empty `name`); to label one, fetch the chat via `chats/batch_query`, read `p2p_target_id`, and resolve it with a contact lookup.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Required scopes**: `im:feed_group_v1:read` **plus** `im:chat:read` (always).
- **Output fields** (raw envelope): `items[]` (live) / `deleted_items[]` (soft-deleted), each `{feed_id (oc_xxx), feed_type (chat), update_time, chat_name (when resolvable)}`.

## See also

- [lark-im-feed-groups.md](lark-im-feed-groups.md) — raw `feed.groups.*` write APIs, enums, and rule guidance
- [lark-im-feed-group-list.md](lark-im-feed-group-list.md) — list your feed groups
- [lark-im-feed-group-list-item.md](lark-im-feed-group-list-item.md) — list all feed cards in a group (paginated)
