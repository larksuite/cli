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
