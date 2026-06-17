# +feed-group-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-group-list`. **Run `lark-cli im +feed-group-list --help` for the authoritative flags (`--page-size` / `--page-token` / `--page-all` / `--page-limit` / `--start-time` / `--end-time` / `--as`), the page-size range, and the time format.** This file covers only what `--help` cannot.

**User identity only** (`--as user`); bot/tenant tokens are rejected by the server. This is the **only** CLI surface for listing feed groups — there is no raw `feed.groups list` command.

## Gotchas

- **Dual-list response, merged by `--page-all`.** The response carries two parallel arrays — `groups` (live) and `deleted_groups` (soft-deleted). `--page-all` merges **both** across pages; a naive single-array pager would silently drop one list's later pages. Incremental-sync consumers must read both arrays. Adds no enrichment.
- **`--page-token` wins over `--page-all`** when both are set — you get exactly that one page, not a full sweep.
- **Never infer completeness from counts.** `--page-size` caps `groups` + `deleted_groups` *combined*, so a page may hold fewer live groups than the size suggests, and per-page counts can be smaller still when entries are filtered. Pagination is governed solely by `has_more`.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Required scope**: `im:feed_group_v1:read`.
- **Output fields** (raw envelope): `groups[]` / `deleted_groups[]`, each `{group_id (ofg_xxx), type (normal|rule), name, rules{rules[]}}` · `page_token` · `has_more`. The `rules` shape is documented in [lark-im-feed-groups.md](lark-im-feed-groups.md).

## See also

- [lark-im-feed-groups.md](lark-im-feed-groups.md) — raw `feed.groups.*` write APIs, enums, and rule guidance
- [lark-im-feed-group-list-item.md](lark-im-feed-group-list-item.md) — list the feed cards inside one group
- [lark-im-feed-group-query-item.md](lark-im-feed-group-query-item.md) — look up specific feed cards by ID
