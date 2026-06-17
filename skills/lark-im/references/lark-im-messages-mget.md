# im +messages-mget

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +messages-mget`. **Run `lark-cli im +messages-mget --help` for the authoritative flags (`--message-ids` / `--no-reactions` / `--download-resources` / `--as` / `--format`), the `om_xxx` ID format, and the max-50 batch cap.** This file covers only what `--help` cannot.

Supports both `--as user` (default) and `--as bot`. Auto-resolves sender names and auto-expands `thread_replies`, both with reactions / `update_time` per [message enrichment](lark-im-message-enrichment.md).

## Gotchas

- **Batch, don't loop**: one call with comma-separated IDs is one request (up to 50); calling per-ID wastes round-trips.
- **Image content is a placeholder, not bytes**: image messages render as `[Image: img_xxx]`. Use `--download-resources` to write them to `./lark-im-resources/`, or [`im +messages-resources-download`](lark-im-messages-resources-download.md) for a single resource.
- **Use `--format json` for full bodies** — non-JSON formats truncate `content`.
- Permission denied → needs `im:message:readonly` + `contact:user.base:readonly` (sender-name resolution requires the contact scope, which the message scope alone won't satisfy).

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Return shape**: `{messages:[...], total:N}`; each message has `message_id` / `msg_type` / `create_time` / `sender` (incl. resolved `name`) / `content`. Enrichment-added fields are documented in [message enrichment](lark-im-message-enrichment.md).
