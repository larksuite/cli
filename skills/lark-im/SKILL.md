---
name: lark-im
version: 1.0.0
description: "飞书即时通讯：收发消息、建群/查群、读取或搜索群消息（含 @我/未回复）、管理群成员、消息置顶/取消置顶、发送和处理交互卡片、下载消息资源、管理表情/标记/Feed。用户要操作 IM 群聊、私聊、消息、线程、Pin、Feed 快捷入口、Feed 分组或卡片回调时使用；认证初始化、全局权限排查、历史会议和日程任务不属于本 skill。用户明确要求直接执行时，进入本 skill 用 lark-cli 执行，CLI 风险门禁负责必要确认。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli im --help"
---

# im (v1)

Use this skill directly for IM tasks. Read `../lark-shared/SKILL.md` only when you must initialize auth, switch profiles, recover from an auth/permission error, or interpret `_notice`; do not front-load shared rules for ordinary IM read/write flows.

## Fast Routes

Prefer these routes before `schema`, raw `api`, or broad list commands:

| User intent | Start here | Avoid |
| --- | --- | --- |
| Find a group by name | `lark-cli im +chat-search --query "<name>" --format json` | `+chat-list` unless search fails; it can return many unrelated chats |
| Create a group | `lark-cli im +chat-create --as user|bot --name "<name>" ... --format json` | Raw `im chats create` unless shortcut flags cannot express the request |
| Send text / markdown / card / media | `lark-cli im +messages-send --chat-id oc_xxx --text ...` or `--msg-type interactive --content '<card JSON>'` | Guessing `schema im.messages.create`; that method is not exposed as an IM resource command |
| Pin / unpin a message | `lark-cli im pins create --data '{"message_id":"om_xxx"}'` then `lark-cli im pins delete --message-id om_xxx --yes` | `+feed-shortcut-*`; Feed shortcuts pin chats in the sidebar, not messages |
| Read messages in a named group | `+chat-search` first, then `+chat-messages-list --chat-id oc_xxx --page-size 10 --format json` | `+chat-list` or large `--page-size 50` pages when only a few messages are needed |
| Search messages / @me | `+messages-search --as user --chat-id oc_xxx --is-at-me --page-size 20 --format json` | Bot identity for `+messages-search`; this shortcut is user-only |

For large message pages, project fields with `--jq` instead of reading full persisted output, for example:

```bash
lark-cli im +chat-messages-list --chat-id oc_xxx --page-size 20 --format json \
  --jq '.data.messages[] | {message_id, create_time, sender, content, mentions}'
```

## Guardrails

- `@me` / `@我` means messages mentioning the current user. If a matched message was sent by the current user, do not count it as "someone mentioned me and I have not replied"; compare with the current user identity from `lark-cli auth status --json --verify` when needed.
- For "unreplied @me" questions, search mentions first with `+messages-search --is-at-me`; do not dump a large chat history page before narrowing candidates.
- If the user asks to create something and then update it or send an updated version, the final answer must preserve both facts: the initially created object and the updated/sent result.

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

### Sender Name Resolution with Bot Identity

When using bot identity (`--as bot`) to fetch messages (e.g. `+chat-messages-list`, `+threads-messages-list`, `+messages-mget`), sender names may not be resolved (shown as open_id instead of display name). This happens when the bot cannot access the user's contact info.

**Root cause**: The bot's app visibility settings do not include the message sender, so the contact API returns no name.

**Solution**: Check the app's visibility settings in the Lark Developer Console — ensure the app's visible range covers the users whose names need to be resolved. Alternatively, use `--as user` to fetch messages with user identity, which typically has broader contact access.

### Default message enrichment (reactions / update_time)

The four message-pulling shortcuts (`+messages-mget`, `+chat-messages-list`, `+messages-search`, `+threads-messages-list`) automatically attach a `reactions` block and (for edited messages) `update_time` to each returned message — no separate `im.reactions.batch_query` call is needed. Pass `--no-reactions` to opt out. For the full contract (output shape, the `im:message.reactions:read` scope requirement, and the "missing field ≠ fetch failure" data rules), read [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Opt-in resource auto-download (`--download-resources`)

`+chat-messages-list`, `+messages-mget`, and `+threads-messages-list` accept `--download-resources` (**off by default** — no `resources` block and no extra requests when omitted). When set, eligible message resources (image/file/audio/video/media + post-embedded; **stickers excluded**) are downloaded into `./lark-im-resources/` and each message gains a `resources` array of `{message_id, key, type, local_path, size_bytes}`. Downloads are deduped by `(message_id, file_key)`, run with bounded concurrency, and isolate single-resource failures (`error: true` + stderr warning). **Scope:** requires `im:message:readonly` (already declared by the listing commands — no extra scope); works under both user and bot identity. For one-off downloads use [`+messages-resources-download`](references/lark-im-messages-resources-download.md). Full contract: [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Card Messages (Interactive)

Card messages (`interactive` type) are not yet supported for compact conversion in event subscriptions. The raw event data will be returned instead, with a hint printed to stderr.

`interactive` cards support callback events (`card.action.trigger`) — see [`references/lark-im-card-action-reply.md`](references/lark-im-card-action-reply.md).

### Audio Messages

`--audio` sends a voice message and supports only Opus audio files, for example `.opus` files or Ogg Opus (`.ogg`) files. For `mp3`, `wav`, or other non-Opus audio, either convert to `.opus` first and keep using `--audio`, or send the original file as an attachment with `--file`.

### Sending Doc Content as a Message

When sending content fetched from a Lark doc as a message, fetch the doc with --doc-format im-markdown, then send it as a message using the --markdown format. The fetched content is already in markdown; in any content-forwarding scenario, keep the fetched original text and send it in the --markdown format. Note: if the doc contains a cite tag with type="user", keep it as-is and do not strip the tag.

### Flag Types

Flags support two layers:

- **Message-layer flag**: `(ItemTypeDefault, FlagTypeMessage)` — regular message bookmark
- **Feed-layer flag**: `(ItemTypeThread/ItemTypeMsgThread, FlagTypeFeed)` — thread as feed-layer bookmark

Item types for feed-layer flags:
- **ItemTypeThread** (4) = thread in a topic-style chat
- **ItemTypeMsgThread** (11) = thread in a regular chat

### Feed Shortcut

Feed shortcuts add chats to the current user's feed sidebar. They are distinct from flags:

- **Flag** = bookmark on a message/thread, scoped to the user's bookmark list.
- **Feed shortcut** = entry in the user's feed sidebar (currently only chats).

Key limits:
- Only **CHAT-type** (`feed_card_id` is `oc_xxx`) is exposed via OpenAPI; doc/app/subscription shortcuts exist internally but are not yet whitelisted.
- All three operations (create/remove/list) are **user-identity only** — they sign with `user_access_token`.
- Batch size is **10 per call** for create/remove; list is a one-page wrapper with opaque `page_token` pagination.

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli im +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+chat-create`](references/lark-im-chat-create.md) | Create a group chat or topic chat; user/bot; --chat-mode group|topic; private/public; invites users/bots; optionally sets bot manager |
| [`+chat-list`](references/lark-im-chat-list.md) | List chats the current user/bot is a member of; defaults to groups; pass --types=p2p,group to include p2p single chats (user-only); user/bot; supports sorting, pagination, --exclude-muted (user-only) |
| [`+chat-messages-list`](references/lark-im-chat-messages-list.md) | List messages in a chat or P2P conversation; user/bot; accepts --chat-id or --user-id, resolves P2P chat_id, supports time range/sort/pagination |
| [`+chat-search`](references/lark-im-chat-search.md) | Search visible group chats by --query keyword and/or --member-ids; user/bot; e.g. look up chat_id by group name; supports type filters, sorting, pagination, and --exclude-muted (user identity only) |
| [`+chat-update`](references/lark-im-chat-update.md) | Update group chat name or description; user/bot; updates a chat's name or description |
| [`+messages-mget`](references/lark-im-messages-mget.md) | Batch get messages by IDs; user/bot; fetches up to 50 om_ message IDs, formats sender names, expands thread replies |
| [`+messages-reply`](references/lark-im-messages-reply.md) | Reply to a message (supports thread replies); user/bot; supports text/markdown/post/media replies, reply-in-thread, idempotency key |
| [`+messages-resources-download`](references/lark-im-messages-resources-download.md) | Download images/files from a message; user/bot; supports automatic chunked download for large files (8MB chunks), auto-detects file extension from Content-Type |
| [`+messages-search`](references/lark-im-messages-search.md) | Search messages across chats (supports keyword, sender, time range filters) with user identity; user-only; filters by chat/sender/attachment/time, supports auto-pagination via `--page-all` / `--page-limit`, enriches results via batched mget and chats batch_query |
| [`+messages-send`](references/lark-im-messages-send.md) | Send a message to a chat or direct message; user/bot; sends to chat-id or user-id with text/markdown/post/media, supports idempotency key |
| [`+threads-messages-list`](references/lark-im-threads-messages-list.md) | List messages in a thread; user/bot; accepts om_/omt_ input, resolves message IDs to thread_id, supports sort/pagination |
| [`+flag-create`](references/lark-im-flag-create.md) | Create a bookmark on a message; user-only; defaults to message-layer flag; use --flag-type feed for feed-layer flag (item_type auto-detected from chat mode) |
| [`+flag-cancel`](references/lark-im-flag-cancel.md) | Cancel (remove) a bookmark. When no --flag-type is given, best-effort double-cancel: removes message layer and (when chat_type is determinable) feed layer |
| [`+flag-list`](references/lark-im-flag-list.md) | List bookmarks; user-only; auto-enriches feed-type thread entries with message content; supports `--page-all` auto-pagination |
| [`+feed-shortcut-create`](references/lark-im-feed-shortcut-create.md) | Add chats to the user's feed shortcuts; user-only; oc_xxx chat IDs only; batch up to 10 per call; `--head`/`--tail` controls insertion order; partial failures return an `ok:false` ledger |
| [`+feed-shortcut-remove`](references/lark-im-feed-shortcut-remove.md) | Remove chats from the user's feed shortcuts; user-only; batch up to 10 per call; removing an absent shortcut is idempotent success; real per-item failures return an `ok:false` ledger |
| [`+feed-shortcut-list`](references/lark-im-feed-shortcut-list.md) | List one page of the user's feed shortcuts; user-only; omit `--page-token` for the first page; default output enriches CHAT entries under `detail`; pass `--no-detail` to skip the extra lookup and `im:chat:read` scope |
| [`+feed-group-list`](references/lark-im-feed-group-list.md) | List the caller's feed groups (tags); user-only; supports `--page-all` auto-pagination |
| [`+feed-group-list-item`](references/lark-im-feed-group-list-item.md) | List feed cards in a feed group (tag); user-only; enriches each item with chat_name resolved from feed_id; supports --page-all auto-pagination |
| [`+feed-group-query-item`](references/lark-im-feed-group-query-item.md) | Look up specific feed cards in a feed group (tag) by ID; user-only; enriches each item with chat_name resolved from feed_id |

## Raw API Fallback

```bash
lark-cli schema im.<resource>.<method>   # for one raw method
lark-cli im <resource> <method> [flags] # call raw IM resource
```

Use raw methods only when no shortcut covers the task. Do not guess versioned names such as `im.v1.*`; this CLI uses resource names like `im.chats.create`, `im.chat.members.get`, `im.messages.forward`, `im.pins.create`, and `im.feed.groups.*`. For exact flags and scopes, use the single command's `--help` or single-method `schema`, not a broad resource schema dump.
