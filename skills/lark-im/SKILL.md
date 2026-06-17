---
name: lark-im
version: 1.0.0
description: "飞书即时通讯：收发消息和管理群聊。发送和回复消息、搜索聊天记录、管理群聊成员、上传下载图片和文件（支持大文件分片下载）、管理表情回复、发送应用内/短信/电话加急。当用户需要发消息、查看或搜索聊天记录、下载聊天中的文件、查看群成员、搜索群、创建群聊或话题群、管理标记数据、管理 Feed 置顶、管理标签数据时使用。不负责收发邮件（→ lark-mail）、日程与会议安排（→ lark-calendar）、会议回放与纪要（→ lark-vc）、IM 事件订阅（→ lark-event）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli im --help"
---

# im (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli im +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+chat-create`](references/lark-im-chat-create.md) | Create a group chat or topic chat |
| [`+chat-list`](references/lark-im-chat-list.md) | List chats the current user/bot is a member of |
| [`+chat-messages-list`](references/lark-im-chat-messages-list.md) | List messages in a chat or P2P conversation |
| [`+chat-search`](references/lark-im-chat-search.md) | Search visible group chats by --query keyword and/or --member-ids |
| [`+chat-update`](references/lark-im-chat-update.md) | Update group chat name or description |
| [`+messages-mget`](references/lark-im-messages-mget.md) | Batch get messages by IDs |
| [`+messages-reply`](references/lark-im-messages-reply.md) | Reply to a message (supports thread replies) |
| [`+messages-resources-download`](references/lark-im-messages-resources-download.md) | Download images/files from a message |
| [`+messages-search`](references/lark-im-messages-search.md) | Search messages across chats (supports keyword, sender, time range filters) with user identity |
| [`+messages-send`](references/lark-im-messages-send.md) | Send a message to a chat or direct message |
| [`+threads-messages-list`](references/lark-im-threads-messages-list.md) | List messages in a thread |
| [`+flag-create`](references/lark-im-flag-create.md) | Create a bookmark on a message |
| [`+flag-cancel`](references/lark-im-flag-cancel.md) | Cancel (remove) a bookmark |
| [`+flag-list`](references/lark-im-flag-list.md) | List bookmarks |
| [`+feed-shortcut-create`](references/lark-im-feed-shortcut-create.md) | Add chats to the user's feed shortcuts |
| [`+feed-shortcut-remove`](references/lark-im-feed-shortcut-remove.md) | Remove chats from the user's feed shortcuts |
| [`+feed-shortcut-list`](references/lark-im-feed-shortcut-list.md) | List one page of the user's feed shortcuts |
| [`+feed-group-list`](references/lark-im-feed-group-list.md) | List the caller's feed groups (tags) |
| [`+feed-group-list-item`](references/lark-im-feed-group-list-item.md) | List feed cards in a feed group (tag) |
| [`+feed-group-query-item`](references/lark-im-feed-group-query-item.md) | Look up specific feed cards in a feed group (tag) by ID |

## Core Concepts

- **Message** `message_id` (om_xxx) · **Chat** `chat_id` (oc_xxx, group or P2P) · **Thread** `thread_id` (om_xxx / omt_xxx).
- **Flag** — bookmark on a message/thread (two layers, see below).
- **Feed Shortcut** `feed_card_id` (oc_xxx) — a chat pinned to the user's feed sidebar.
- **Feed Group** `feed_group_id` (ofg_xxx) — a tag grouping feed cards (`feed_id`+`feed_type`); `normal` (explicit) / `rule` (auto-derived).

## Important Notes

### Identity (user vs bot)

- `--as user` (`user_access_token`): runs as the authorized user; permission = app scopes + that user's own access to the target.
- `--as bot` (`tenant_access_token`): runs as the app bot; depends on bot's chat membership, app visibility range, bot scopes.
- When an API supports both, the token decides *who* operates — owner/admin, membership, tenant, visibility are checked against the caller, so the same API can pass on one identity and fail on the other.

### Sender name resolution

As **bot**, the sender may show as `open_id` (bot visibility range doesn't cover it); `--as user` gives real names.

```bash
lark-cli im +chat-messages-list --chat-id oc_xxx --as bot   # BAD: sender = open_id
lark-cli im +chat-messages-list --chat-id oc_xxx --as user  # GOOD: sender = real name
```

### Default message enrichment

The four message-pulling shortcuts auto-attach `reactions` (+ `update_time` for edited messages) — no separate `reactions.batch_query` (needs `im:message.reactions:read`); `--no-reactions` opts out. `+chat-messages-list` / `+messages-mget` / `+threads-messages-list` also accept `--download-resources` (opt-in, off by default) to fetch message binaries into `./lark-im-resources/`. Contract: [`references/lark-im-message-enrichment.md`](references/lark-im-message-enrichment.md).

### Flag Types

Two layers (item_type auto-detected from chat mode — rarely set by hand):
- **Message-layer** `(ItemTypeDefault, FlagTypeMessage)` — regular message bookmark.
- **Feed-layer** `(ItemType{Thread|MsgThread}, FlagTypeFeed)` — thread bookmarked at feed level:
  - **ItemTypeThread** (4) = a topic in a topic-style chat (an entry in the group's Thread tab).
  - **ItemTypeMsgThread** (11) = a reply thread under a single message in a regular group.

### Feed Shortcut

Pins a chat to the **current user's** feed sidebar. Limits: **CHAT-type only** (oc_xxx); **user-identity only**; **10 per call** for create/remove; list uses opaque `page_token`.

## 不在本 skill 范围

- 邮件 → [`lark-mail`](../lark-mail/SKILL.md)｜日程/会议 → [`lark-calendar`](../lark-calendar/SKILL.md)｜会议回放/纪要 → [`lark-vc`](../lark-vc/SKILL.md)
- 文档评论 → [`lark-drive`](../lark-drive/SKILL.md)｜IM 事件订阅 → [`lark-event`](../lark-event/SKILL.md)｜姓名解析 open_id → [`lark-contact`](../lark-contact/SKILL.md)

群禁言 / 管理员 / 角色 / 解散 / 转让 / 群设置 等群治理 lark-cli im 暂无命令：如实告知“暂不支持”、勿臆造，引导用户到飞书客户端群设置手动操作（高风险写操作，勿擅自走原生 API 代执行）。

## API Resources

```bash
lark-cli schema im.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli im <resource> <method> [flags] # 调用 API
```

### chats

`create`(bot) · `get`(user/bot) · `link`(user/bot) · `update`(user/bot)

### chat.members

`bots`(user/bot) · `create`(user/bot) · `delete`(user/bot) · `get`(user/bot)

### chat.user_setting

`batch_query`(user) · `batch_update`(user)

### chat.managers

`add_managers`(user/bot) · `delete_managers`(user/bot)

### chat.moderation

`get`(user/bot) · `update`(user/bot)

### messages

`delete`(user/bot) · `forward`(user/bot) · `merge_forward`(bot) · `read_users`(bot) · `urgent_app`(bot) · `urgent_phone`(bot) · `urgent_sms`(bot)

### reactions

`batch_query`(user/bot) · `create`(user/bot) · `delete`(user/bot) · `list`(user/bot)

### threads

`forward`(user/bot)

### images

`create`(bot)

### pins

`create`(user/bot) · `delete`(user/bot) · `list`(user/bot)

### feed.groups

`batch_add_item`(user) · `batch_query`(user) · `batch_remove_item`(user) · `create`(user) · `delete`(user) · `update`(user)

