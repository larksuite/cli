# +feed-group-list-item

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-group-list-item`. **Run `lark-cli im +feed-group-list-item --help` for the authoritative flags (`--feed-group-id` / `--page-size` / `--page-token` / `--page-all` / `--page-limit` / `--start-time` / `--end-time` / `--as`), the page-size range, and the time format.** This file covers only what `--help` cannot.

**User identity only** (`--as user`); bot/tenant tokens are rejected. This is the **only** CLI surface for `feed.groups.list_item` — there is no raw command.

## Gotchas

- **`chat_name` enrichment is unconditional → needs a second scope.** A v1 feed card's `feed_id` is always a chat ID (`oc_xxx`), so the shortcut always issues a follow-up `chats/batch_query` and injects `chat_name` into each entry of **both** `items[]` and `deleted_items[]`. There is no single-scope, un-enriched path — so this needs `im:chat:read` **in addition to** `im:feed_group_v1:read` (vs. `+feed-group-list`, which needs only the read scope).
- **Unresolvable cards silently omit `chat_name`** — a soft-deleted chat or one you can't see just lacks the field; the command still exits 0. Do not treat a missing `chat_name` as an error. **p2p (direct) cards also omit it** (the server returns an empty `name`; clients show the partner's display name instead) — if you need a label, fetch the chat via `chats/batch_query`, read `p2p_target_id`, and resolve it with a contact lookup.
- **Dual-list response.** Like `+feed-group-list`, results split into `items[]` (live) and `deleted_items[]` (soft-deleted); `--page-all` merges both. Incremental-sync consumers must read both.
- **`--page-token` wins over `--page-all`** when both are set — you get exactly that one page.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Required scopes**: `im:feed_group_v1:read` **plus** `im:chat:read` (always, because enrichment always runs).
- **Output fields** (raw envelope): `items[]` / `deleted_items[]`, each `{feed_id (oc_xxx), feed_type (chat), update_time, chat_name (when resolvable)}` · `page_token` · `has_more`.

## See also

- [lark-im-feed-groups.md](lark-im-feed-groups.md) — raw `feed.groups.*` write APIs, enums, and rule guidance
- [lark-im-feed-group-list.md](lark-im-feed-group-list.md) — list your feed groups
- [lark-im-feed-group-query-item.md](lark-im-feed-group-query-item.md) — look up specific feed cards by ID
