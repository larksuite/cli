# im +chat-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +chat-list`. **Run `lark-cli im +chat-list --help` for the authoritative flags (`--types` / `--sort` / `--page-size` / `--page-token` / `--exclude-muted` / `--user-id-type` / `--as`), enums, and defaults.** This file covers only what `--help` cannot.

## Gotchas

- **Not a search API — there is no `--query`.** It always returns the full member list, paginated. For keyword/name/member lookup use [`+chat-search`](lark-im-chat-search.md); do NOT loop `+chat-list` to find a named group.
- **Defaults to groups only.** p2p single chats appear only when you pass `--types` to include `p2p`, and **only under `--as user`** (see below).
- **`--exclude-muted` is user-only.** Under `--as bot` the mute API is UAT-only, so the filter is silently skipped and all chats come back unfiltered — don't trust it under bot identity.

### Bot identity + p2p (silent-downgrade trap)

`tenant_access_token` cannot enumerate p2p chats (user-privacy protection). Under `--as bot`:

- `--types=p2p` alone → **rejected at validation** with an actionable error; no request sent.
- `--types=p2p,group` → CLI **silently strips `p2p`** and sends `group` only. The strip surfaces two ways (so neither humans nor agents miss it): a `warning: bot_strip_p2p: ...` line on **stderr**, and a structured entry in the top-level **`notices`** array of the stdout JSON (`{"code":"bot_strip_p2p","message":"..."}`). DryRun emits the same stderr warning.

To include p2p, switch to `--as user --types=p2p,group`.

### `--exclude-muted` output envelope

When set, the JSON gains a top-level `filter` sub-object (absent otherwise, so existing consumers are unaffected). Invariant **`fetched_count == returned_count + filtered_count`** always holds. Filtering is **per page, client-side** (the page's chat_ids are batched through the mute-status API after the list call), so a filtered page can return fewer than `--page-size` rows — keep paginating via `page_token`. `filter` and `notices` are separate top-level keys and never collide.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Output fields**: `chat_id` (oc_xxx) · `name` · `description` · `owner_id` (type per `--user-id-type`) · `external` · `chat_status` (`normal` / `dissolved` / `dissolved_save`) · `chat_mode` (`group` / `topic` / `p2p`) · `p2p_target_type` (e.g. `user`) · `p2p_target_id`. For p2p rows, `name` is the peer's display name and `p2p_target_*` identify the peer.
- **`filter` envelope shape**: `{"applied":"exclude_muted","fetched_count","returned_count","filtered_count","hint"}`.
- **Pagination fields** (`--format json`): `has_more` / `page_token` live under `.data`.
