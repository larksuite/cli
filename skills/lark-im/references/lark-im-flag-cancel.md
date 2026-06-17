# im +flag-cancel

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

Maps to `lark-cli im +flag-cancel`. **Run `lark-cli im +flag-cancel --help` for the authoritative flags (`--message-id` / `--flag-type` / `--item-type`), defaults, and enums.** This file covers only what `--help` cannot.

## Gotchas

- **Default is double-cancel (both layers)**: omitting `--flag-type` best-effort cancels both the message-layer flag `(default, message)` and the feed-layer flag `(thread, feed)` or `(msg_thread, feed)`. The server treats cancel of a non-existent flag idempotently — no error — so double-cancel is safe even when only one layer has a flag.
- **Feed-layer `item_type` is determined by `chat_mode`**: `topic` chat → `item_type=thread`; regular `group` chat → `item_type=msg_thread`. The double-cancel path auto-detects this (requires `im:chat:read`). **Best-effort, not all-or-nothing**: the message layer is always removed; the feed layer is removed only when the chat type can be determined — otherwise a warning is printed on stderr and the feed layer is silently skipped (the command still exits 0). When calling single-layer feed cancel with `--flag-type feed`, you must supply `--item-type` explicitly.
- **Idempotent**: repeated cancel calls on an already-unflagged message succeed silently.
- **Do NOT call `+flag-list` for verification**: a success response means the flag was removed. Paginating `+flag-list` to confirm is expensive and unnecessary.
- **Finding a message ID**: use `+messages-search --query "<keywords>"` to locate the message and extract `message_id`. Do NOT use `+flag-list` — it requires full pagination and won't reliably locate the message.
- **User identity only** — `--as bot` is not supported.
