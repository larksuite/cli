# im +chat-search

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +chat-search`. **Run `lark-cli im +chat-search --help` for the authoritative flag list (`--query` / `--member-ids` / `--search-types` / `--chat-modes` / `--sort` / `--page-size` / `--page-token` / `--is-manager` / `--exclude-muted` / `--disable-search-by-user`), limits, and enums.** This file covers only what `--help` cannot.

## Gotchas

- **Visibility-scoped, not global**: only finds chats visible to the current identity (joined chats + visible public chats). A chat the caller can't see won't appear.
- **At least one of `--query` / `--member-ids`** must be given (either alone, or combined).
- `--query` containing `-` is auto-wrapped in quotes.
- **On empty results, do NOT fall back to `+chat-list` / `GET /chats`** — list is not a search API (returns all chats unfiltered, won't locate the target). Refine the keyword or check visibility under the current identity instead.
- Supports `--as user` (default) and `--as bot` (bot needs bot capability enabled).
- **`--sort` is always descending** (`create_time` / `update_time` / `member_count`, high→low). There is no ascending option. If the user asks for "fewest first / ascending / 从少到多", tell them the search API doesn't support ascending and re-sort the fetched page client-side — do **NOT** invent `member_count_asc` or pass `asc` (rejected).

## `--exclude-muted` (user identity only)

Drops do-not-disturb chats; under `--as bot` the filter is silently inactive (mute is per-user / UAT-only). When set, the JSON envelope gains a `filter` sub-object (absent otherwise, so existing consumers are unaffected); the invariant **`fetched_count == returned_count + filtered_count`** always holds. Only confirmed-muted chats count toward `filtered_count`; non-member public groups are retained and surfaced in `hint`. For strict member-only results, combine with `--search-types "private,public_joined,external"`.

## Error → remediation

- `99991679` (`--as user`): UAT not authorized for `im:chat:read` → `lark-cli auth login --scope "im:chat:read"`.
- `99991672` (`--as bot`): app lacks `im:chat:read` TAT → enable in the Open Platform console.
- `232025`: bot capability not activated → enable in the console.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Output fields**: `chat_id` (oc_xxx) · `name` · `description` · `owner_id` · `external` · `chat_status` (`normal` / `dissolved` / `dissolved_save`).
- `--member-ids`: up to 50, format `ou_xxx` (`--help` only says "comma-separated").
