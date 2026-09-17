---
name: lark-im
version: 1.0.0
description: "飞书即时通讯：收发消息和管理群聊。发送和回复消息、搜索聊天记录、管理群聊成员、下载图片和文件、管理表情回复、发送和处理交互卡片（Interactive Card）。当用户需要发消息、查看或搜索聊天记录、下载聊天中的文件、查看群成员、创建群聊或话题群、管理标记数据、管理标签数据时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli im --help"
---

# im (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## Core Concepts

- **Message**: A single message in a chat, identified by `message_id` (om_xxx). Supports types: text, post, image, file, audio, video, sticker, interactive (card), share_chat, share_user, merge_forward, etc.
- **Chat**: A group chat or P2P conversation, identified by `chat_id` (oc_xxx).
- **Thread**: A reply thread under a message, identified by `thread_id` (om_xxx or omt_xxx).
- **Reaction**: An emoji reaction on a message.
- **Flag**: A bookmark on a message or thread.

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

### AppLink and Share Links

Prefer CLI-returned links: use `chat_app_link` to open joined conversations, `message_app_link` to open messages, and `share_link` to invite others to groups. If manually building a joined-conversation AppLink, use `https://<applink_host>/client/chat/open?openChatId=<oc_xxx>`, never `chatId=<oc_xxx>` or `lark://...chat_id=<oc_xxx>`.

### Agent Caller

Use the current Agent credential. `--as user` remains valid; it does not mean impersonating the human who requested the task. Chat membership, owner/admin status and resource visibility apply to the actual caller. Resolve human recipients separately. See [lark-shared](../lark-shared/SKILL.md) for credential errors.

### Sender Name Resolution

When fetching messages (`+chat-messages-list`, `+threads-messages-list`, `+messages-mget`, `+messages-search`), the CLI shows a display name for both user and bot senders:

- **Server-provided name**: the read APIs return `sender_name` (plus the full-i18n `sender_i18n_names` map) on each message `sender`; the CLI surfaces it as the sender's `name` for users and bots alike. No name lookup and no extra permission are needed — **no contact scope** and no `application:bot.basic_info:read`.
- **Fallback to id**: when the server does not provide a name, the sender is shown by its id and the command still exits 0. There is no contact-directory fallback.

The raw `sender_name` is not duplicated in output (its value is in `name`); the full `sender_i18n_names` map (all locales) is preserved for consumers that need a specific language, alongside an optional `open_bot_id` (`ou_`) for bot senders aligned with the message-receive event channel. System messages (`msg_type: system`) have no sender name — that is normal, not an error.

### Default message enrichment (reactions / update_time)

The four message-pulling shortcuts (`+messages-mget`, `+chat-messages-list`, `+messages-search`, `+threads-messages-list`) automatically attach a `reactions` block and (for edited messages) `update_time` to each returned message — no separate `im.reactions.batch_query` call is needed. Pass `--no-reactions` to opt out. For the full contract (output shape, the `im:message.reactions:read` scope requirement, and the "missing field ≠ fetch failure" data rules), read [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Compact message output (`--concise`)

Some message-listing shortcuts support `--concise` for compact Markdown output. Use it when the user asks for concise output or a smaller result/file; check `--help` for availability and do not combine it with an explicit `--format`, an enabled `--json`, or a non-empty `--jq`.

### Opt-in resource auto-download (`--download-resources`)

`+chat-messages-list`, `+messages-mget`, and `+threads-messages-list` accept `--download-resources` to save eligible attachments into `./lark-im-resources/` and add a `resources` array to each message. It is off by default; stickers are not downloadable. A failed attachment is reported on that resource without aborting the message pull. Use [`+messages-resources-download`](references/lark-im-messages-resources-download.md) for one attachment. See [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md) for the output contract.

**Folder resources** are containers, not files — a folder `file_key` cannot be downloaded directly. Expand it first with `lark-cli im files folder --recursive --file-key <folder_key> --srctype message --srcid <message_id>`, then download the files inside with [`+messages-resources-download`](references/lark-im-messages-resources-download.md).

### Card Messages (Interactive)

**Before sending, replying with, or updating any `interactive` card (`+messages-send` / `+messages-reply` / `messages.patch`), you MUST read [`references/card/lark-im-card-create.md`](references/card/lark-im-card-create.md) and follow its workflow.** The card JSON passed to `--msg-type interactive --content` (send/reply) or `messages.patch --data` (update) must be the output of that workflow — never hand-write or copy a card payload.

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

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli im +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+chat-create`](references/lark-im-chat-create.md) | Create a group chat or topic chat; --chat-mode group|topic; private/public; invites users/bots |
| [`+chat-search`](references/lark-im-chat-search.md) | Search visible group chats by --query keyword and/or --member-ids; e.g. look up chat_id by group name; supports type filters, sorting, auto-pagination, and --exclude-muted |
| [`+chat-list`](references/lark-im-chat-list.md) | List chats the Agent Employee is a member of; defaults to groups; pass --types=p2p,group to include p2p single chats; supports sorting, auto-pagination, --exclude-muted |
| [`+chat-members-list`](references/lark-im-chat-members-list.md) | List members of a chat; returns separate users[] / bots[] buckets; --member-types filters which kinds to return; --page-all pagination; surfaces truncations[] when the server caps a bucket |
| [`+chat-messages-list`](references/lark-im-chat-messages-list.md) | List messages in a chat or P2P conversation; accepts --chat-id or --user-id, resolves P2P chat_id, supports time range, --order asc/desc sorting, auto-pagination |
| [`+chat-update`](references/lark-im-chat-update.md) | Update group chat name or description; updates a chat's name or description |
| [`+message-read-users`](references/lark-im-message-read-status.md) | List users who read one message; identity-specific scopes; supports bounded auto-pagination |
| [`+messages-edit`](references/lark-im-messages-edit.md) | Edit a message's content (text/post, including the attachment zone); see the reference for a known capability conflict; PUT /open-apis/im/v1/messages/:message_id |
| [`+messages-mget`](references/lark-im-messages-mget.md) | Batch get messages by IDs; fetches up to 50 om_ message IDs, formats sender names, expands thread replies |
| [`+messages-read-status`](references/lark-im-message-read-status.md) | Batch query whether the current user read 1–50 messages; returns readable items and invalid message IDs |
| [`+messages-reply`](references/lark-im-messages-reply.md) | Reply to a message (supports thread replies); supports text/markdown/post/media replies, reply-in-thread, idempotency key |
| [`+messages-resources-download`](references/lark-im-messages-resources-download.md) | Download an image/file from a message; folders are not directly downloadable — expand with `im files folder --recursive` first, then download the files inside |
| [`+messages-search`](references/lark-im-messages-search.md) | Search messages across chats (supports keyword, sender, time range filters) ; filters by chat/sender/attachment/time, supports auto-pagination via `--page-all` / `--page-limit`, enriches results via batched mget and chats batch_query |
| [`+messages-send`](references/lark-im-messages-send.md) | Send a message to a chat or direct message; sends to chat-id or user-id with text/markdown/post/media, supports idempotency key |
| [`+threads-messages-list`](references/lark-im-threads-messages-list.md) | List messages in a thread; accepts om_/omt_ input, resolves message IDs to thread_id, supports --order asc/desc sorting, auto-pagination |
| [`+flag-create`](references/lark-im-flag-create.md) | Create a bookmark on a message; defaults to message-layer flag; use --flag-type feed for feed-layer flag (item_type auto-detected from chat mode) |
| [`+flag-cancel`](references/lark-im-flag-cancel.md) | Cancel (remove) a bookmark. When no --flag-type is given, best-effort double-cancel: removes message layer and (when chat_type is determinable) feed layer |
| [`+flag-list`](references/lark-im-flag-list.md) | List bookmarks; auto-enriches feed-type thread entries with message content; `--page-all` is capped by `--page-limit` (default 20, max 1000), and `has_more=true` means the result is incomplete |

## API Resources

```bash
lark-cli schema im.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli im <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### chats

  - `create` — 创建群。
  - `get` — 获取群信息。The caller must be in the target chat to get full details, and must belong to the same tenant for internal chats.
  - `link` — 获取群分享链接。The caller must be in the target chat, must be an owner or admin when chat sharing is restricted to owners/admins, and must belong to the same tenant for internal chats.
  - `update` — 更新群信息。

### chat.members

  - `create` — 将用户或机器人拉入群聊。The caller must be in the target chat; for internal chats the operator must belong to the same tenant; if only owners/admins can add members, the caller must be an owner/admin.
  - `delete` — 将用户或机器人移出群聊。Only the group owner or an admin can remove others; max 50 users or 5 bots per request.

### chat.user_setting

  - `batch_query` — 批量查询当前用户在群内的个人偏好设置 (e.g. `is_muted` mutes normal messages, `is_mute_at_all` mutes @all messages); up to 10 chats per request. The caller must be in each target chat.
  - `batch_update` — 批量更新当前用户在群内的个人偏好设置 (e.g. `is_muted` mutes normal messages, `is_mute_at_all` mutes @all messages); up to 10 chats per request. The caller must be in each target chat.

### chat.nickname

  - `get` — 获取自己的群昵称。Get your own nickname in the chat (self-only). Returns an empty string when no nickname is set.
  - `update` — 设置自己的群昵称。Set or update your own nickname in the chat (self-only). `nickname` must be a non-empty string (max 300 bytes). Use DELETE to clear it.
  - `delete` — 清空自己的群昵称。Clear your own nickname in the chat (self-only).

### chat.join_requests

  - `list` — 列出群的待审批入群申请（仅群主/管理员）。List pending join requests for a chat. The caller must be the chat owner or an admin. Paginated (`page_size` 1-100); stop on `has_more == false` — `page_token` is returned even on the last page, so paging while it is present never terminates.
  - `handle` — 批量审批入群申请（approve/reject，仅群主/管理员）。Approve or reject pending join requests in bulk (1-50 items, processed in order). The caller must be the chat owner or an admin. `results[]` mirrors `items[]` in count and order — check each `result` (`success` / `failed` / `already_handled`); exit 0 does not mean every item succeeded.

### chat.managers

  - `add_managers` — 指定群管理员。Only the group owner can add managers; max 10 managers per chat (20 for super-large chats), and at most 5 bots per request.
  - `delete_managers` — 删除群管理员。Only the group owner can remove managers; max 50 users or 5 bots per request.

### chat.moderation

  - `get` — 获取群成员发言权限。The caller must be in the target chat and belong to the same tenant.
  - `update` — 更新群发言权限。Only the group owner can update; the caller must be in the chat.

### messages

  - `read_status` — 批量查询当前用户对消息的已读状态。Accepts up to 50 message IDs and returns readable items plus invalid message IDs.[Must-read](references/lark-im-message-read-status.md)
  - `delete` — 撤回消息。The caller must be in the chat to revoke a group message, and must be the owner, an admin or the sender to revoke someone else's.
  - `forward` — 转发消息。
  - `merge_forward` — 合并转发消息。
  - `read_users` — 查询消息已读信息。The caller must still be in the chat, and can query only messages it sent within the last 7 days.[Must-read](references/lark-im-message-read-status.md)
  - `patch` — 更新已发送的消息卡片。Update an interactive message card sent by the app. The message must have been sent within the last 14 days, and `content` must be a JSON-serialized string no larger than 30 KB.[Must-read](references/card/lark-im-card-create.md)

### reactions

  - `batch_query` — 批量获取消息表情。[Must-read](references/lark-im-reactions.md)
  - `create` — 添加消息表情回复。The caller must be in the conversation that contains the message.[Must-read](references/lark-im-reactions.md)
  - `delete` — 删除消息表情回复。The caller must be in the conversation that contains the message, and can only delete reactions added by itself.[Must-read](references/lark-im-reactions.md)
  - `list` — 获取消息表情回复。The caller must be in the conversation that contains the message.[Must-read](references/lark-im-reactions.md)

### threads

  - `forward` — 转发话题。

### pins

  - `create` — Pin 消息。
  - `delete` — 移除 Pin 消息。
  - `list` — 获取群内 Pin 消息。

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `chats.create` | `im:chat:create` |
| `chats.get` | `im:chat:read` |
| `chats.link` | `im:chat:read` |
| `chats.update` | `im:chat:update` |
| `chat.members.create` | `im:chat.members:write_only` |
| `chat.members.delete` | `im:chat.members:write_only` |
| `chat.members.get` | `im:chat.members:read` |
| `+chat-members-list` | `im:chat.members:read` |
| `chat.user_setting.batch_query` | `im:chat.user_setting:read` |
| `chat.user_setting.batch_update` | `im:chat.user_setting:write` |
| `chat.managers.add_managers` | `im:chat.managers:write_only` |
| `chat.managers.delete_managers` | `im:chat.managers:write_only` |
| `chat.moderation.get` | `im:chat.moderation:read` |
| `chat.moderation.update` | `im:chat:moderation:write_only` |
| `chat.join_requests.list` | `im:chat.membership_application:read` |
| `chat.join_requests.handle` | `im:chat.membership_application:write` |
| `+messages-read-status` | user: `im:message:readonly` (recommended), `im:message`, or `im:message:get_as_user` |
| `+message-read-users` | user: `im:message:readonly` (recommended), `im:message`, `im:message:basic`, or `im:message:get_as_user`; bot: `im:message:readonly` |
| `messages.read_status` | `im:message:readonly` (recommended), `im:message`, or `im:message:get_as_user` |
| `messages.delete` | `im:message:recall` |
| `messages.forward` | `im:message` |
| `messages.merge_forward` | `im:message` |
| `messages.read_users` | user: `im:message:readonly` (recommended), `im:message`, `im:message:basic`, or `im:message:get_as_user`; bot: `im:message:readonly` |
| `messages.patch` | `im:message:update` |
| `reactions.batch_query` | `im:message.reactions:read` |
| `reactions.create` | `im:message.reactions:write_only` |
| `reactions.delete` | `im:message.reactions:write_only` |
| `reactions.list` | `im:message.reactions:read` |
| `threads.forward` | `im:message` |
| `pins.create` | `im:message.pins:write_only` |
| `pins.delete` | `im:message.pins:write_only` |
| `pins.list` | `im:message.pins:read` |
