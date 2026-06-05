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
