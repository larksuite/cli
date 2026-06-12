# im default message enrichment (reactions / update_time)

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

This is the single source of truth for the automatic message-enrichment contract shared by the four message-pulling shortcuts — [`+messages-mget`](lark-im-messages-mget.md), [`+chat-messages-list`](lark-im-chat-messages-list.md), [`+messages-search`](lark-im-messages-search.md), [`+threads-messages-list`](lark-im-threads-messages-list.md). They automatically attach `reactions` and `update_time` to each returned message, so callers do **not** need to invoke the raw [`im.reactions.batch_query`](lark-im-reactions.md) API separately.

- **`reactions`** — populated from `im.reactions.batch_query` as `{counts, details}`. The field is only attached when the server actually returns data; messages with no reactions omit it. Replies inside `thread_replies` are enriched alongside their parent (collected into the same id set), so outer and inner messages follow identical semantics. The id set is split into batches of <= 20 (server-side cap) and the batches are dispatched with bounded concurrency (up to 4 in flight), so high-N pulls — e.g. page 50 + ~500 expanded thread replies = 550 ids → ⌈550 / 20⌉ = **28 batches** — finish in a few round-trips instead of serializing into tens of seconds.
- **`update_time`** — emitted only when `updated == true` (message was actually edited). The server echoes `update_time == create_time` for unedited messages too, but the CLI gates that output away so consumers don't misread every message as "edited".
- **Opt-out** — each shortcut accepts `--no-reactions` to skip the extra round-trip when the caller only needs message bodies.

## Thread replies expansion

`+messages-mget` and `+chat-messages-list` also auto-expand thread replies: any returned message that carries a `thread_id` triggers a fetch of that thread's replies, which are attached as a `thread_replies` array on the host. Fetches across distinct threads run with bounded concurrency (up to 4 in flight). Two caps gate the result:

- **`perThread` (default 50)** — max replies fetched for any single thread.
- **`totalLimit` (default 500)** — max cumulative replies across all threads on the page.

`totalLimit` is enforced **post-fetch against actual returned reply counts**, not against the planned per-thread ceiling — so a chat with many short threads (e.g. 12 threads × 3 actual replies = 36 ≪ 500) attaches every thread, even though the planned sum (12 × 50 = 600) would exceed the budget. When a thread's actual replies push the running total across `totalLimit`, that thread is truncated to fit the remaining budget and its host is flagged with `thread_has_more: true` so consumers know the server has more.

On per-thread fetch failure the host gets `thread_replies_error: true` (mirrors the reactions data contract); budget-truncated or budget-skipped threads do NOT carry that flag.

## Resource auto-download (`--download-resources`, opt-in)

`+chat-messages-list`, `+messages-mget`, and `+threads-messages-list` accept an **opt-in** `--download-resources` flag. It is **off by default** — when omitted, output and the request count are identical to before (no `resources` block, no extra round-trips).

When enabled:

- Each message that carries downloadable resources gets a `resources` array. Eligible types: `image`, `file`, `audio`, `video`, `media`, and post-embedded `img` / `media`. **Stickers are excluded** (Feishu does not support fetching sticker resources).
- Each ref is `{message_id, key, type, local_path, size_bytes}` — `type` is `image` or `file`; `message_id` is the id used to fetch the resource. For a standalone message that is its own id; for a resource inside a **merge_forward** it is the **top-level container** `message_id`, not the sub-item's own id (the download endpoint rejects sub-item ids with `234003 File not in msg` and can only fetch a forwarded resource through the container). Thread replies each get their own block.
- Files download into `./lark-im-resources/` under the current working directory. Each distinct `(message_id, file_key)` is downloaded once (deduped) with bounded concurrency (up to 3 in flight).
- **Fail-silent isolation**: a single resource that fails to download is flagged `"error": true` with one stderr line (`warning: resource_download_failed: <message_id>/<key>: ...`); the main message and the other resources are unaffected.
- Output paths are confined to `./lark-im-resources/` by the same guards as [`+messages-resources-download`](lark-im-messages-resources-download.md) (abnormal `file_key` with path separators / `..` / absolute paths is rejected).
- **Scope**: the download uses `GET /open-apis/im/v1/messages/:message_id/resources/:file_key`, which requires `im:message:readonly` — already declared in each listing command's `Scopes`, so `--download-resources` needs **no extra scope** beyond what's required to read the messages (user identity also needs `im:message.group_msg:get_as_user` / `im:message.p2p_msg:get_as_user`; bot identity needs `im:message.group_msg` / `im:message.p2p_msg:readonly`, all already declared). Works under both user and bot identity. If a bot was registered before `im:message:readonly` was granted, a single resource will fail-silently (`error: true` + stderr warning) rather than aborting the pull.

Use `--download-resources` when you want the binaries on disk in one pass; otherwise the message content keeps the inline resource markers (e.g. `[Image: img_xxx]`, `<file .../>`, `<audio key="..." duration="Xs"/>`) and you can fetch individual resources later with [`+messages-resources-download`](lark-im-messages-resources-download.md).

## Scope requirement

The default enrichment requires `im:message.reactions:read`, already declared in each shortcut's `UserScopes` / `BotScopes` (or `Scopes` for the user-only search command), so the framework's pre-flight check surfaces a `missing_scope` error before the request is sent. Bots that were registered before this scope was added need an incremental authorization in the Feishu developer console; users can run:

```bash
lark-cli auth login --scope "im:message.reactions:read"
```

## Data contract — missing field ≠ fetch failure

| Situation | Output |
|---|---|
| Message has no reactions | `reactions` field is omitted (not `{}`, not an empty list) |
| Message was never edited | `update_time` field is omitted |
| Whole batch failed | Messages in that batch carry no `reactions`; one line on stderr: `warning: reactions_batch_query_failed: ...` |
| Some message IDs failed | Failed IDs go to stderr: `warning: reactions_partial_failed: N message(s) failed (...)` |

When deciding "has the user already reacted?", branch on the **presence of the `reactions` field plus its `counts` contents**, not on whether a value is `null` — the field's absence means "no data attached" (which usually means "no reactions exist"), not "fetch failed".
