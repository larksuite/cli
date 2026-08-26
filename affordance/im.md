# im
> skill: lark-im

## +chat-create
Use this when a new group or topic chat is needed. Choose the calling identity deliberately because it affects ownership and member visibility.

### Avoid when
- The chat already exists and only its name or description should change → use [[+chat-update]].

### Examples

**Create a private group with the configured identity**
```bash
lark-cli im +chat-create --name "My Group"
```

### Skills
- `lark-im/references/lark-im-chat-create.md`
- `lark-im/references/lark-im-chat-identity.md`

## +chat-list
Use this to enumerate chats the current identity has joined.

### Avoid when
- Looking up a group by name or member → use [[+chat-search]].

### Examples

**List joined group chats**
```bash
lark-cli im +chat-list
```

### Skills
- `lark-im/references/lark-im-chat-list.md`

## +chat-members-list
Use this instead of the raw member methods when you need users and bots separated and truncation reported explicitly.

### Tips
- Default fetches a single page; pass --page-all to walk every page.
- With --page-all and no explicit --page-size, the max page size is used to minimize round-trips.
- truncations[] in the result means the server capped a bucket due to security config — the member list is incomplete.

### Examples

**List one page of users and bots**
```bash
lark-cli im +chat-members-list --chat-id oc_xxx
```

### Skills
- `lark-im/references/lark-im-chat-members-list.md`

## +chat-messages-list
Use this for message history when the conversation is already known.

### Avoid when
- Searching across conversations → use [[+messages-search]].
- Fetching full details for known message ids → use [[+messages-mget]].

### Examples

**List messages in a group chat**
```bash
lark-cli im +chat-messages-list --chat-id oc_xxx
```

### Skills
- `lark-im/references/lark-im-chat-messages-list.md`

## +chat-search
Use this to resolve a visible group name or member set to a stable chat_id before another operation.

### Examples

**Search visible groups by keyword**
```bash
lark-cli im +chat-search --query "project"
```

### Skills
- `lark-im/references/lark-im-chat-search.md`

## +chat-update
Use this for name or description changes after identifying the group and an owner/admin-capable identity.

### Examples

**Rename a group**
```bash
lark-cli im +chat-update --chat-id oc_xxx --name "New Group Name"
```

### Skills
- `lark-im/references/lark-im-chat-update.md`
- `lark-im/references/lark-im-chat-identity.md`

## +message-read-users
Use this to list the users who have read one known message. Select user or bot identity according to who sent the message and the API's seven-day constraint.

### Examples

**List readers for one message**
```bash
lark-cli im +message-read-users --message-id om_xxx
```

### Skills
- `lark-im/references/lark-im-message-read-status.md`

## +messages-mget
Use this when one or more message_ids are already known and full message details are needed.

### Avoid when
- Discovering messages by chat or keyword → use [[+chat-messages-list]] or [[+messages-search]].

### Examples

**Fetch one known message**
```bash
lark-cli im +messages-mget --message-ids om_xxx
```

### Skills
- `lark-im/references/lark-im-messages-mget.md`

## +messages-read-status
Use this to query whether the current user has read one or more known messages. The shortcut is user-only and accepts at most 50 message ids.

### Examples

**Query the current user's read status**
```bash
lark-cli im +messages-read-status --as user --message-ids om_xxx,om_yyy
```

### Skills
- `lark-im/references/lark-im-message-read-status.md`

## +messages-reply
Use this when the response must remain attached to a specific message or thread.

### Prerequisites
- Confirm the target message, reply content, and sending identity before sending.

### Examples

**Reply with plain text using the configured identity**
```bash
lark-cli im +messages-reply --message-id om_xxx --text "Received"
```

### Skills
- `lark-im/references/lark-im-messages-reply.md`

## +messages-resources-download
Use this after a read command exposes a message_id and matching file_key and the binary content is actually needed.

### Examples

**Download an image resource to the current directory**
```bash
lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image
```

### Skills
- `lark-im/references/lark-im-messages-resources-download.md`

## +messages-search
Use this to find messages across conversations by keyword or structured filters.

### Examples

**Search messages by keyword**
```bash
lark-cli im +messages-search --query "project progress"
```

### Skills
- `lark-im/references/lark-im-messages-search.md`

## +messages-send
Use this for new outbound content. Select text, markdown, exact JSON, or one media flag according to the content shape.

### Prerequisites
- Confirm the recipient, content, and sending identity before sending.

### Tips
- Do not pin user or bot in a generic call: both are supported, and the sender must match the user's intent.

### Examples

**Send plain text using the configured identity**
```bash
lark-cli im +messages-send --chat-id oc_xxx --text "Hello"
```

### Skills
- `lark-im/references/lark-im-messages-send.md`

## +threads-messages-list
Use this when a message or thread id is known and the replies inside that thread are needed.

### Examples

**List replies in a thread**
```bash
lark-cli im +threads-messages-list --thread omt_xxx
```

### Skills
- `lark-im/references/lark-im-threads-messages-list.md`

## +flag-create
Use this for a personal bookmark, not a chat-visible pin.

### Examples

**Bookmark a message at the default message layer**
```bash
lark-cli im +flag-create --as user --message-id om_xxx
```

### Skills
- `lark-im/references/lark-im-flag-create.md`

## +flag-cancel
Use this to remove a personal bookmark. Omitting --flag-type performs the skill's best-effort double-cancel across message and feed layers.

### Examples

**Remove both bookmark layers when discoverable**
```bash
lark-cli im +flag-cancel --as user --message-id om_xxx
```

### Skills
- `lark-im/references/lark-im-flag-cancel.md`

## +flag-list
Use this to inspect the current user's bookmarks.

### Tips
- Results are oldest first; when has_more=true, paginate before treating the final item or count as authoritative.

### Examples

**Fetch the first page of bookmarks**
```bash
lark-cli im +flag-list --as user
```

### Skills
- `lark-im/references/lark-im-flag-list.md`

## +feed-shortcut-create
Use this to pin one or more chats in the current user's feed sidebar.

### Prerequisites
- Resolve each chat_id with [[+chat-search]] or [[+chat-list]] first.

### Examples

**Pin one chat at the top of the feed**
```bash
lark-cli im +feed-shortcut-create --as user --chat-id oc_xxx
```

### Skills
- `lark-im/references/lark-im-feed-shortcut-create.md`

## +feed-shortcut-remove
Use this to unpin one or more chats from the current user's feed sidebar.

### Examples

**Unpin one chat**
```bash
lark-cli im +feed-shortcut-remove --as user --chat-id oc_xxx
```

### Skills
- `lark-im/references/lark-im-feed-shortcut-remove.md`

## +feed-shortcut-list
Use this to inspect the current user's feed shortcuts.

### Tips
- This fetches one page only. Continue with the returned page_token until has_more=false; if a token becomes invalid after the list changes, restart without it.

### Examples

**Fetch the first page**
```bash
lark-cli im +feed-shortcut-list --as user
```

### Skills
- `lark-im/references/lark-im-feed-shortcut-list.md`

## +feed-group-list
Use this to discover the current user's feed-group ids; --page-all merges both live and soft-deleted groups.

### Examples

**Fetch the first page of feed groups**
```bash
lark-cli im +feed-group-list --as user
```

### Skills
- `lark-im/references/lark-im-feed-group-list.md`

## +feed-group-list-item
Use this to enumerate every feed card in a known group and enrich chat cards with chat_name.

### Examples

**List one feed group's first page**
```bash
lark-cli im +feed-group-list-item --as user --feed-group-id ofg_xxx
```

### Skills
- `lark-im/references/lark-im-feed-group-list-item.md`

## +feed-group-query-item
Use this lightweight lookup when the feed-group id and chat ids are already known.

### Avoid when
- Discovering all cards in a group → use [[+feed-group-list-item]].

### Examples

**Look up two known chat cards**
```bash
lark-cli im +feed-group-query-item --as user --feed-group-id ofg_xxx --feed-id oc_a,oc_b
```

### Skills
- `lark-im/references/lark-im-feed-group-query-item.md`

## chat.members create
Use this raw method for the skill's two-step recovery flow when a bot-created group cannot invite users because they are invisible to the bot.

### Prerequisites
- Create the group first, then add members as a user who is already in that group.

### Examples

**Add reachable users and report invalid ids separately**
```bash
lark-cli im chat.members create --params '{"chat_id":"oc_xxx","member_id_type":"open_id","succeed_type":1}' --data '{"id_list":["ou_aaa","ou_bbb"]}' --as user
```

### Skills
- `lark-im/references/lark-im-chat-create.md`
- `lark-im/references/lark-im-chat-identity.md`

## feed.groups create
Use this raw method to create a feed group; prefer a normal group unless membership must be rule-derived.

### Examples

**Create an empty normal feed group**
```bash
lark-cli im feed.groups create --as user --data '{"feed_group_creator":{"type":"normal","name":"Releases"}}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## feed.groups update
Use this raw method to rename a feed group or replace its rules; restrict update_fields to what actually changes.

### Examples

**Rename only, leaving rules untouched**
```bash
lark-cli im feed.groups update --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"feed_group_updater":{"name":"测试标签名称","update_fields":[1]}}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## feed.groups delete
Use this raw method only when the user intends to delete the identified feed group.

### Prerequisites
- Confirm the exact feed_group_id and deletion intent before executing.

### Examples

**Delete one feed group**
```bash
lark-cli im feed.groups delete --as user --params '{"feed_group_id":"ofg_xxx"}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## feed.groups batch_query
Use this instead of listing when the feed-group ids are already known; consume both live and soft-deleted result arrays.

### Examples

**Look up two feed groups by id**
```bash
lark-cli im feed.groups batch_query --as user --params '{"user_id_type":"open_id"}' --data '{"group_ids":["ofg_xxx","ofg_yyy"]}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## feed.groups batch_add_item
Use this raw method to add known chat cards to a normal feed group.

### Examples

**Add two chats to a feed group**
```bash
lark-cli im feed.groups batch_add_item --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"},{"feed_id":"oc_yyy","feed_type":"chat"}]}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## feed.groups batch_remove_item
Use this raw method to remove known chat cards from a normal feed group.

### Examples

**Remove one chat from a feed group**
```bash
lark-cli im feed.groups batch_remove_item --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"}]}'
```

### Skills
- `lark-im/references/lark-im-feed-groups.md`

## images create
Use this raw upload when an image_key must be reused.

### Avoid when
- Sending an image once → use [[+messages-send]] --image to upload and send in one step.

### Examples

**Upload a local message image**
```bash
lark-cli im images create --data '{"image_type":"message"}' --file ./diagram.png
```

### Skills
- `lark-im/references/lark-im-messages-send.md`

## reactions create
Use this raw method to add an emoji reaction, not a text reply.

### Examples

**Add a smile reaction**
```bash
lark-cli im reactions create --params '{"message_id":"om_xxx"}' --data '{"reaction_type":{"emoji_type":"SMILE"}}'
```

### Skills
- `lark-im/references/lark-im-reactions.md`

## reactions list
Use this raw method for reaction records on one standalone message; message-reading shortcuts already enrich reactions automatically.

### Examples

**List reactions on one message**
```bash
lark-cli im reactions list --params '{"message_id":"om_xxx"}'
```

### Skills
- `lark-im/references/lark-im-reactions.md`

## reactions delete
Use this raw method only for a reaction created by the calling identity.

### Prerequisites
- Obtain reaction_id from [[reactions list]] or the [[reactions create]] response.

### Examples

**Delete one reaction record**
```bash
lark-cli im reactions delete --params '{"message_id":"om_xxx","reaction_id":"ZCaCIjUBVVWSrm5L-3ZTw_xxx"}'
```

### Skills
- `lark-im/references/lark-im-reactions.md`

## reactions batch_query
Use this raw method only for standalone message ids; message-reading shortcuts already attach reactions.

### Examples

**Query the first page of reactions for two messages**
```bash
lark-cli im reactions batch_query --params '{"user_id_type":"open_id"}' --data '{"queries":[{"message_id":"om_xxx"},{"message_id":"om_yyy"}],"page_size_per_message":10,"reaction_type":"LAUGH"}'
```

### Skills
- `lark-im/references/lark-im-reactions.md`
