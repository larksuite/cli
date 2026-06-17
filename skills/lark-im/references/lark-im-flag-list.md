# im +flag-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

Maps to `lark-cli im +flag-list`. **Run `lark-cli im +flag-list --help` for the authoritative flags (`--page-size` / `--page-token` / `--page-all` / `--page-limit` / `--enrich-feed-thread`), limits, and enums.** This file covers only what `--help` cannot.

## Gotchas

- **Sort order is ascending (oldest first)**: the API returns `flag_items` sorted by `update_time` ascending. When `has_more=true`, the first page's items are the oldest, not the newest. To get the latest flag, paginate all pages (`--page-all`) and take the last item: `-q '.data.flag_items[-1]'`.
- **`delete_flag_items` are NOT enriched**: message content is fetched only for active flags (`flag_items`). To get message content for a canceled flag, query separately via `+messages-mget --message-ids <item_id>`.
- **`(thread, feed)` / `(msg_thread, feed)` entries are enriched automatically**: the shortcut calls `messages/mget` for feed-type thread entries and writes the result into each entry's `message` field. Disable with `--enrich-feed-thread=false` to avoid the extra scope requirement.
- **User identity only** — `--as bot` is not supported.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Output fields**: `flag_items` (active flags, ascending `update_time`) · `delete_flag_items` (canceled flags, ascending `update_time`, unenriched) · `messages` (server-inlined content for `(default, message)` flags) · `has_more` · `page_token`.
- **Enrichment write target**: for `(thread, feed)` / `(msg_thread, feed)` entries, enriched message content is written to the entry's `message` field (not a top-level `messages` array).
