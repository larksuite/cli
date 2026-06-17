# im +flag-create

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

Maps to `lark-cli im +flag-create`. **Run `lark-cli im +flag-create --help` for the authoritative flags (`--message-id` / `--flag-type` / `--item-type`), defaults, and enums.** This file covers only what `--help` cannot.

## Gotchas

- **Message-layer vs feed-layer are distinct**: default (`--flag-type message`) creates a `(default, message)` flag visible in message history. `--flag-type feed` creates a feed-layer flag visible in the Feed tab. A message can carry both simultaneously — they are independent.
- **Feed-layer auto-detection requires extra scopes**: `--flag-type feed` without `--item-type` calls the chat API to determine `chat_mode`; this needs `im:message.group_msg:get_as_user` / `im:message.p2p_msg:get_as_user` / `im:chat:read`. If you already know the chat type, pass `--item-type` explicitly to skip the lookup.
- **Only three valid `(item_type, flag_type)` pairs**: `(default, message)`, `(thread, feed)`, `(msg_thread, feed)`. Any other combination is rejected by the server.
- **Do NOT call `+flag-list` for verification**: a success response means the flag was created. Full pagination of `+flag-list` to confirm is expensive and unnecessary.
- **Finding a message ID**: use `+messages-search --query "<keywords>"` to locate the message and extract `message_id`. Do NOT use `+flag-list` — it requires full pagination and won't reliably locate the message.
- **User identity only** — `--as bot` is not supported.
