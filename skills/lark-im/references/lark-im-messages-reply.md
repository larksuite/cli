# im +messages-reply

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +messages-reply`. **Run `lark-cli im +messages-reply --help` for the authoritative flags (`--message-id` / `--text` / `--markdown` / `--content` / `--msg-type` / `--image` / `--file` / `--video` / `--video-cover` / `--audio` / `--reply-in-thread` / `--idempotency-key` / `--as`), content/media flags, mutual-exclusion rules, msg-type inference, `--video`/`--video-cover` pairing, and the cwd-relative media path rules.** This file covers only what `--help` cannot.

Safety: replies are visible to others — confirm the target message, content, and identity before sending (see lark-shared risk policy). `--as bot` requires the app to already be in the target chat.

**Shared with [`im +messages-send`](lark-im-messages-send.md)**: picking the content flag (`--markdown` vs `--text` vs `--content`), `--markdown` boundary rules, the `images.create` pre-upload step for local images in Markdown, `$'...'` for exact formatting, `@mention` syntax, the `--content` JSON shape per `msg_type`, the return value, and the `--idempotency-key` 1-hour window all behave identically — see that reference, do not re-derive here.

## Gotchas

- **`--reply-in-thread` posts into the target message's thread** (thread stream), not the main chat stream; without it the reply appears inline in the main stream referencing the target. Only meaningful in chats that support thread replies — in a non-thread chat the flag is a no-op.
- **`--message-id` is the message being replied to** (`om_xxx`), not a chat id. To reply by named group, resolve the message first via `im +chat-search` → `im +chat-messages-list`, then take its `message_id`.
