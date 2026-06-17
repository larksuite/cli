# im +messages-search

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +messages-search`. **Run `lark-cli im +messages-search --help` for the authoritative flags (`--query` / `--chat-id` / `--sender` / `--include-attachment-type` / `--chat-type` / `--sender-type` / `--exclude-sender-type` / `--is-at-me` / `--at-chatter-ids` / `--start` / `--end` / `--page-size` / `--page-all` / `--page-limit`), enums, time format (ISO 8601 + offset), and `--no-reactions`.** This file covers only what `--help` cannot.

**User identity only** (`--as user`); bot not supported. Auto-orchestrates: search → batch mget → batch chat-context enrich (plus reactions / update_time per [message enrichment](lark-im-message-enrichment.md)).

## Gotchas

- **Identity is user-only — do NOT try `--as bot`.** For bot identity with a named group + history intent, resolve via `im +chat-search --as bot`, then `im +chat-messages-list --as bot --chat-id <id>`.
- **Resolving a chat by name**: use `im +chat-search` first to get `chat_id`, then pass `--chat-id`. **Do not use `im chats search` or `+chat-list`.**
- `--at-chatter-ids` results also include messages that `@all`.
- **Resources not downloaded**: images render as `[Image: img_xxx]` placeholders; use `im +messages-resources-download` for the bytes.

## Query construction (high-signal vs over-constrained)

- Use `--query` only for **real message keywords**. For activity review ("最近一周和哪些 Bot 交互"、"整理和某人的聊天记录"), keep `--query ""` and rely on `--sender` / `--sender-type` / `--chat-id` / `--start` / `--end`. Do NOT put instruction words ("看看"、"总结"、"聊天记录") into `--query` — they over-constrain and hide the relevant messages.
- **Relative time**: compute start/end from the current day at execution time; never copy date literals from this file ("最近一周" = compute the actual range, don't hardcode).
- For activity summaries, retain each item's `message_id` / sender / chat / create_time as recall targets; don't rely on a high-level keyword match alone.

## Work summary / report generation

- Narrow first (`--chat-id` / `--sender` / `--start` / `--end`), then **paginate exhaustively** — one page (20–50) is rarely enough.
- Default to `--page-all --format json` (JSON carries `has_more` / `page_token`); use `--page-limit <n>` to bound the run; resume via `--page-token` if a bounded run still returns `has_more=true`.
- **Accumulate all pages, then summarize** — don't summarize after page 1. Group by topic/thread, not chronology.
- If no time range is given, default to the current week (Mon→today), or ask.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Auto-enrich fields** (JSON): conversation `chat_type` (p2p/group) · `chat_name` · `chat_partner` (p2p only: other party `open_id`+`name`). Per message: `deleted` (true=recalled) · `updated` (edited after send) · `mentions` `[{id,key,name}]` **present only when @mentions exist** · `thread_id` (omt_xxx) **present only when replies exist**.
- **Follow-up clues**: each result carries `chat_id` (+ `thread_id` when present) → `im +chat-messages-list --chat-id <id>` / `im +threads-messages-list --thread <id>`.
