# im +chat-messages-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +chat-messages-list`. **Run `lark-cli im +chat-messages-list --help` for the authoritative flags (`--chat-id` / `--user-id` / `--start` / `--end` / `--order` / `--page-size` / `--page-token` / `--no-reactions` / `--download-resources` / `--as`), enums, time format (ISO 8601 or date-only), and the `--chat-id`/`--user-id` mutual-exclusion.** This file covers only what `--help` cannot.

Supports both `--as user` (default) and `--as bot`. Auto-resolves the p2p `chat_id`, auto-expands `thread_replies`, and enriches with reactions / `update_time` per [message enrichment](lark-im-message-enrichment.md).

## Gotchas

- **`--user-id` (DM by open_id) is user-identity only — and the constraint is silent until you hit it.** The p2p-resolution endpoint requires user identity; with `--as bot` it errors. For bot identity, look up the p2p `chat_id` yourself and pass `--chat-id`.
- **`P2P chat not found for this user`** means no DM exists *for the current identity* with that user — not a bad ID. Confirm the DM relationship under the identity you're calling as.
- **Resolve a chat name → `chat_id` via [`+chat-search`](lark-im-chat-search.md) first**, then pass `--chat-id`. **Do NOT use `im chats search` or `+chat-list`** — those are not search APIs and won't locate the target.
- **`--order` defaults to `desc`** (newest first); pass `--order asc` for chronological reading. (Note: the flag is `--order`, not `--sort`.) It is the **only** sort axis — messages are always ordered by creation time. There is no field sort: `--sort sender` (or any field) is rejected. If asked to group/sort by sender, fetch with `--order` and aggregate client-side, and say so (local post-processing, not a CLI/API sort).
- **Image content is a placeholder, not bytes**: images render as `[Image: img_xxx]`; files/audio/video carry resource keys. Nothing downloads unless you pass `--download-resources` (writes to `./lark-im-resources/`) or use [`im +messages-resources-download`](lark-im-messages-resources-download.md).

## Thread expansion (`thread_id`)

A message carrying `thread_id` (`omt_xxx`) has thread replies. Auto-expansion attaches them as `thread_replies` (see enrichment doc); to drive the thread yourself use [`im +threads-messages-list --thread <id>`](lark-im-threads-messages-list.md) — `--order desc --page-size 10` for recent replies, `--order asc --page-size 50` (then paginate) for the full discussion, skip it for an overview.

## Bot identity + named-group history (cross-command recipe)

When the user says "以 bot 身份 / use application identity" and wants historical messages for a *named* group, use bot identity for **both** steps:

```bash
lark-cli im +chat-search --as bot --query "<chat name keyword>" --format json
lark-cli im +chat-messages-list --as bot --chat-id <chat_id> --page-size 50 --format json
```

**Do NOT reach for `im +messages-search --as bot`** — that command is user-only. Continue with `--page-token` while `has_more=true`.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Per-message fields** (JSON): `deleted` (always present; `true` = recalled) · `updated` (edited after send) · `mentions` `[{id,key,name}]` **present only when @mentions exist** · `thread_id` (omt_xxx) **present only when replies exist**. Page envelope carries `has_more` / `page_token`.
