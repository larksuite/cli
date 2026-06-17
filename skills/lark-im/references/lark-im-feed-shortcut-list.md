# im +feed-shortcut-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-shortcut-list`. **Run `lark-cli im +feed-shortcut-list --help` for the authoritative flags (`--page-token` / `--no-detail` / `--as` / `--dry-run` / `--format` / `-q`), and pagination restart behavior.** This file covers only what `--help` cannot.

## Gotchas

- **Only CHAT-type shortcuts are returned** — other shortcut types exist in the API IDL but are not whitelisted by the server today. Do not assume `type` will ever be non-1.
- **No built-in auto-pagination.** Drive the loop yourself: read `data.page_token` and pass it back until `has_more=false`. The shortcut intentionally stays one-page-at-a-time so callers decide what to do when a token is rejected.
- **Detail enrichment calls `im.chats.batch_query` in batches of 50**, requires `im:chat:read`, and attaches the full chat object under `detail`. Pass `--no-detail` to avoid the extra scope and network call when only `feed_card_id` values are needed.
- **P2P chats return an empty `name`** — the Feishu client renders the partner's display name client-side. Use `p2p_target_id` to resolve the partner via `+contact-search` if a display title is needed.
- **Enrichment failure is silent on stdout**: if `im:chat:read` is missing or the batch_query errors, the list still returns successfully; a warning goes to stderr and the data payload gains a `_notice` field (`"detail enrichment skipped: ..."`). Affected entries simply lack the `detail` field. Check `_notice` to distinguish "enrichment skipped" from "nothing to enrich."
- **`detail` shape is dispatched per `type`** — switch on `type` before parsing `detail`; future shortcut types may attach a different object shape.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Output fields**: `shortcuts[].feed_card_id` (oc_xxx) · `shortcuts[].type` (1=CHAT) · `shortcuts[].detail` (full chat object; absent when `--no-detail` or enrichment fails) · `has_more` · `page_token`.
- **`detail` for CHAT**: `chat_id` · `chat_mode` (`group`/`p2p`/`topic`) · `name` · `avatar` · `description` · `external` · `tenant_key`; groups add `owner_id`/`owner_id_type`; p2p adds `p2p_target_id`/`p2p_target_type`.
- **Required scopes**: `im:feed.shortcut:read` always; `im:chat:read` conditionally (default detail path only).
