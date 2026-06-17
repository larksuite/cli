# im default message enrichment (reactions / update_time)

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Cross-cutting behavior shared by the message-pulling shortcuts. `--help` documents only the `--no-reactions` opt-out; the field semantics, caps, and failure contract below are not in any `--help` or schema.

**Relied on by:** [`+messages-mget`](lark-im-messages-mget.md) · [`+chat-messages-list`](lark-im-chat-messages-list.md) · [`+messages-search`](lark-im-messages-search.md) · [`+threads-messages-list`](lark-im-threads-messages-list.md). All four attach `reactions` + `update_time` so callers do **not** need the raw [`im.reactions.batch_query`](lark-im-reactions.md) API. **Only `+messages-mget` and `+chat-messages-list` additionally auto-expand `thread_replies`**; `+messages-search` and `+threads-messages-list` do reactions/`update_time` only.

## Gotchas

- **Missing field ≠ fetch failure.** `reactions` is attached only when the server returns data — a message with no reactions *omits* the field (not `{}`, not `null`). To decide "has the user already reacted?", branch on **presence of `reactions` + its `counts` contents**, never on `null`. Fetch failure is signaled separately on stderr (`warning: reactions_batch_query_failed` for a whole batch; `warning: reactions_partial_failed: N message(s) failed` for some IDs).
- **`update_time` is gated, not raw.** It's emitted only when `updated == true`. The server echoes `update_time == create_time` for unedited messages, but the CLI suppresses that so you don't misread every message as edited.
- **Requires `im:message.reactions:read`.** Declared in each shortcut's scopes, so the pre-flight surfaces `missing_scope` before sending. Bots registered before this scope existed need an incremental authorization in the developer console (a user re-login picking up the scope is enough for user identity).
- **High-N pulls are batched, not serialized.** Reaction lookups split into batches of ≤20 ids (server cap) dispatched with bounded concurrency (≤4 in flight), so e.g. 550 ids → 28 batches finish in a few round-trips, not tens of seconds. Don't add your own throttling/looping on top.

## Resource auto-download (`--download-resources`, opt-in)

`+chat-messages-list` / `+messages-mget` / `+threads-messages-list` accept `--download-resources` (**off by default** — omitted = no `resources` block and zero extra requests). When set, eligible resources (`image`/`file`/`audio`/`video`/`media` + post-embedded; **stickers excluded** — Feishu can't fetch them) download into `./lark-im-resources/`, and each message gains a `resources` array of `{message_id, key, type, local_path, size_bytes}`.

- **`merge_forward` trap**: for a resource inside a forwarded message, `message_id` is the **top-level container** id, not the sub-item's — the download endpoint rejects sub-item ids with `234003 File not in msg`. Thread replies each get their own block.
- **Fail-silent isolation**: one resource that fails to download is flagged `"error": true` + a single stderr `warning: resource_download_failed: …`; the message and other resources are unaffected (exit 0).
- Deduped by `(message_id, file_key)`, bounded concurrency (≤3), output confined to `./lark-im-resources/` (path-separator / `..` / absolute `file_key` rejected).
- **No extra scope**: uses `im:message:readonly`, already declared by the listing commands; works under user and bot identity. For one-off fetches use [`+messages-resources-download`](lark-im-messages-resources-download.md).

## Thread-replies expansion caps (mget / chat-messages-list only)

Any returned message carrying a `thread_id` triggers a fetch of that thread's replies, attached as a `thread_replies` array on the host (distinct threads fetched with ≤4 concurrent). Two caps gate it:

- **`perThread` (default 50)** — max replies per single thread.
- **`totalLimit` (default 500)** — max cumulative replies across all threads on the page.

**`totalLimit` is enforced post-fetch against *actual* returned counts, not the planned per-thread ceiling.** So 12 threads × 3 real replies = 36 all attach, even though the planned sum (12 × 50 = 600) would blow the budget. When a thread's real replies push the running total past `totalLimit`, that thread is truncated to fit and its host is flagged **`thread_has_more: true`**. A per-thread fetch *failure* instead flags the host **`thread_replies_error: true`** — budget-truncated/skipped threads do NOT carry that flag (so the two cases are distinguishable).
