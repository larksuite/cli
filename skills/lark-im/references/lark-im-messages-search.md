# im +messages-search

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

**Run `lark-cli im +messages-search --help` for the authoritative flags, defaults, filter enums, and pagination controls.** This file covers only what `--help` cannot.

## What the shortcut does for you

`--help` says it "enriches results via mget and chats batch_query" but not what that means for your next step. Three API calls happen behind one command:

1. search returns matching `message_id` values
2. mget batch-fetches full content for those IDs
3. chat context is batch-looked-up and attached to each message

**Consequence:** results are already materialized. Do **not** follow a search with `+messages-mget` to "get the details" — that work is done.

## Completeness and recovery

`--help` states that only `meta.complete=true` proves completion. What it does not state is what to do when it is false:

- **`meta.complete=true`** — the result is materialized. Use its `message_id`, `file_key`, and other IDs directly; do not call `+messages-mget`, do not repeat the search.
- **`meta.complete=false`** — follow that response's `hint` and query **only** `materialization.missing_message_ids` with `+messages-mget`. Do not re-query IDs already materialized, do not rerun the whole search, and do not infer that missing detail means the message does not exist.

## AI Usage Guidance

### Query boundary for activity review

Use `--query` only for real message keywords. If the user asks for activity review such as "最近一周我和哪些 Bot 有过交互" or "整理我和某人的聊天记录", and the useful constraints are sender type, chat, person, or time range, keep `--query ""` and rely on those filters. Do not put generic instruction words such as "看看", "总结", "交互内容", or "聊天记录" into `--query`; those words often over-constrain message search and hide the relevant messages.

This guidance applies only when using user identity. `im +messages-search` is user-only; if the user explicitly asks for application/bot identity, do not try `--as bot`. For bot identity with a named group and history/listing intent, resolve the group with `im +chat-search --as bot`, then list messages with `im +chat-messages-list --as bot --chat-id <chat_id>`.

Compute relative time ranges at execution time — "最近一周" means deriving the start and end dates from the current day. Never copy date literals from a reference into an answer for a relative request.

For activity summaries, validate evidence by message IDs and chat context. The final answer should cite or retain the `message_id`, sender, chat, and create time for each important item. If the source data contains concrete `om_...` message IDs or `ou_...` user IDs, treat those IDs as strong recall targets during verification; do not rely only on a high-level keyword match.

### Resolving chat_id from a chat name

When the user refers to a chat by name and you need its `chat_id` for `--chat-id`, resolve it with [`+chat-search`](lark-im-chat-search.md) first.

**Do not use `im chats search` or `+chat-list` for this — always `+chat-search`.**

## Work Summary / Report Generation

When asked to summarize work, generate a weekly report, or compile activity from chat messages, **require a complete result before summarizing** — a partial result is rarely enough.

1. **Start with targeted filters** — narrow with `--chat-id`, `--sender`, `--start`, `--end` before widening.
2. **Accumulate before summarizing** — require `meta.complete=true`, then analyze. If false, recover only the returned `missing_message_ids` as the response `hint` directs.
3. **Use JSON output** — it preserves the message IDs and completion metadata needed to verify the evidence set; pretty output does not.
4. **Group by topic/thread**, not chronologically — chronological dumps read poorly as summaries.
5. If no time range is given, default to the current week (Monday to today) or ask.

## Follow-up clues in results

Each JSON message carries `chat_id` and, when the message has thread replies, `thread_id` (`omt_xxx`). Use them to go deeper: `+chat-messages-list --chat-id` for the surrounding stream, `+threads-messages-list --thread` for the thread.

## Resource Rendering

Image messages render as placeholders such as `![Image](img_xxx)`; **resource binaries are never downloaded by this command**, regardless of flags. Use `im +messages-resources-download` to fetch actual bytes for a specific message.

## Common Errors and Troubleshooting

| Symptom | Root cause | Solution |
|---------|---------|---------|
| Too few results | Time range too narrow or keyword too specific | Widen the range, try broader keywords, or drop `--query` in favour of filters |
| No results | Missing permission, or genuinely no match | Confirm `search:message` is authorized, then relax filters |
| Permission denied | Search scope not authorized | `auth login --scope "search:message"` |

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Enrichment field names.** The chat context attached to each message uses `chat_type`, `chat_name`, and — for p2p only — `chat_partner` (the other participant's `open_id` and `name`). `--help` announces that enrichment happens but names none of these fields.
- **`system` is a valid `msg_type` in results.** Search can return system messages (join/leave/rename events) alongside user content; `--help`'s attachment/sender filters give no way to exclude them, so filter in post-processing when a summary should ignore them.

## References

- [lark-im](../SKILL.md) — all message-related commands
- [lark-im-threads-messages-list](lark-im-threads-messages-list.md) — inspect thread replies
