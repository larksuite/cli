## Core Concepts

- **Message**: A single message in a chat, identified by `message_id` (om_xxx). Supports types: text, post, image, file, audio, video, sticker, interactive (card), share_chat, share_user, merge_forward, etc.
- **Chat**: A group chat or P2P conversation, identified by `chat_id` (oc_xxx).
- **Thread**: A reply thread under a message, identified by `thread_id` (om_xxx or omt_xxx).
- **Reaction**: An emoji reaction on a message.
- **Flag**: A bookmark on a message or thread.
- **Feed Shortcut**: A chat pinned to the current user's feed sidebar, identified by `feed_card_id` (an `oc_xxx` open_chat_id for CHAT type).
- **Feed Group**: A tag that groups feed cards in the feed list, identified by `feed_group_id` (ofg_xxx). Members are feed cards, each identified by `feed_id` + `feed_type`. Two types: `normal` (members managed explicitly) and `rule` (members auto-derived from rules).

## Resource Relationships

```
Chat (oc_xxx)
├── Message (om_xxx)
│   ├── Thread (reply thread)
│   ├── Reaction (emoji)
│   └── Resource (image / file / video / audio)
└── Member (user / bot)
```

## Important Notes

### Identity and Token Mapping

- `--as user` means **user identity** and uses `user_access_token`. Calls run as the authorized end user, so permissions depend on both the app scopes and that user's own access to the target chat/message/resource.
- `--as bot` means **bot identity** and uses `tenant_access_token`. Calls run as the app bot, so behavior depends on the bot's membership, app visibility, availability range, and bot-specific scopes.
- If an IM API says it supports both `user` and `bot`, the token type changes who the operator is. The same API can succeed with one identity and fail with the other because owner/admin status, chat membership, tenant boundary, or app availability are checked against the current caller.

### Sender Name Resolution

When fetching messages (`+chat-messages-list`, `+threads-messages-list`, `+messages-mget`, `+messages-search`), the CLI shows a display name for both user and bot senders:

- **Server-provided name**: the read APIs return `sender_name` (plus the full-i18n `sender_i18n_names` map) on each message `sender`; the CLI surfaces it as the sender's `name` for users and bots alike. No name lookup and no extra permission are needed — **no contact scope** and no `application:bot.basic_info:read`.
- **Fallback to id**: when the server does not provide a name, the sender is shown by its id and the command still exits 0. There is no contact-directory fallback.

The raw `sender_name` is not duplicated in output (its value is in `name`); the full `sender_i18n_names` map (all locales) is preserved for consumers that need a specific language, alongside an optional `open_bot_id` (`ou_`) for bot senders aligned with the message-receive event channel. System messages (`msg_type: system`) have no sender name — that is normal, not an error.

### Default message enrichment (reactions / update_time)

The four message-pulling shortcuts (`+messages-mget`, `+chat-messages-list`, `+messages-search`, `+threads-messages-list`) automatically attach a `reactions` block and (for edited messages) `update_time` to each returned message — no separate `im.reactions.batch_query` call is needed. Pass `--no-reactions` to opt out. For the full contract (output shape, the `im:message.reactions:read` scope requirement, and the "missing field ≠ fetch failure" data rules), read [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Opt-in resource auto-download (`--download-resources`)

`+chat-messages-list`, `+messages-mget`, and `+threads-messages-list` accept `--download-resources` (**off by default** — no `resources` block and no extra requests when omitted). When set, eligible message resources (image/file/audio/video/media + post-embedded; **stickers excluded**) are downloaded into `./lark-im-resources/` and each message gains a `resources` array of `{message_id, key, type, local_path, size_bytes}`. Downloads are deduped by `(message_id, file_key)`, run with bounded concurrency, and isolate single-resource failures (`error: true` + stderr warning). **Scope:** requires `im:message:readonly` (already declared by the listing commands — no extra scope); works under both user and bot identity. For one-off downloads use [`+messages-resources-download`](references/lark-im-messages-resources-download.md). Full contract: [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Card Messages (Interactive)

Card messages (`interactive` type) are not yet supported for compact conversion in event subscriptions. The raw event data will be returned instead, with a hint printed to stderr.

### Flag Types

Flags support two layers:

- **Message-layer flag**: `(ItemTypeDefault, FlagTypeMessage)` — regular message bookmark
- **Feed-layer flag**: `(ItemTypeThread/ItemTypeMsgThread, FlagTypeFeed)` — thread as feed-layer bookmark

Item types for feed-layer flags:
- **ItemTypeThread** (4) = thread in a topic-style chat
- **ItemTypeMsgThread** (11) = thread in a regular chat

### Feed Shortcut

Feed shortcuts add chats to the **current user's** feed sidebar. They are distinct from flags:

- **Flag** = bookmark on a message/thread, scoped to the user's bookmark list.
- **Feed shortcut** = entry in the user's feed sidebar (currently only chats).

Key limits:
- Only **CHAT-type** (`feed_card_id` is `oc_xxx`) is exposed via OpenAPI; doc/app/subscription shortcuts exist internally but are not yet whitelisted.
- All three operations (create/remove/list) are **user-identity only** — they sign with `user_access_token`.
- Batch size is **10 per call** for create/remove; list is a one-page wrapper with opaque `page_token` pagination.
