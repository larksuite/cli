# contact
> skill: lark-contact

## +search-user
The primary user lookup for user identity: search by keyword or email, resolve known ids with --user-ids, or get yourself with --user-ids me — it does by-id reads too, so as a user you rarely need `+get-user`. Each match returns an open_id and p2p_chat_id to chain into follow-ups.

### Skills
- lark-contact/references/lark-contact-search-user.md

### Avoid when
- Running as a bot — this shortcut is user-only; use [[+get-user]] instead (it supports bot identity)
- You only need users' personal status for ids you already hold → use [[user_profiles batch_query]]

### Examples

**Find a user by name**
```bash
lark-cli contact +search-user --query "alice" --as user
```

**Fetch known users by open_id (me = yourself)**
```bash
lark-cli contact +search-user --user-ids "ou_3a8b****6a7b,me" --as user
```

## +search-bot
Find bots (apps) the calling user can see, by keyword. Each match returns the bot's open_id plus p2p_chat_id / has_chatted so you can tell whether a conversation with it already exists. A keyword is mandatory — the filters narrow a search, they cannot list bots on their own.

### Avoid when
- Looking for a person rather than a bot → use [[+search-user]]
- Running as a bot — this shortcut is user-only

### Examples

**Find a bot by name**
```bash
lark-cli contact +search-bot --query "会议助手" --as user
```

**Narrow to bots in one chat**
```bash
lark-cli contact +search-bot --query "助手" --chat-ids "oc_3a8b****6a7b" --as user
```

**Resolve several bot names in one call**
```bash
lark-cli contact +search-bot --queries "会议助手,日报助手,审批助手" --as user
```

## +get-user
Fetch one user's profile by id, or your own with --user-id omitted. Use it under bot identity — `+search-user` is user-only.

### Skills
- lark-contact/references/lark-contact-get-user.md

### Avoid when
- You don't have the user's id yet, or want to match by name/keyword → use [[+search-user]]
- Running as a user — [[+search-user]] --user-ids covers by-id reads and more in one tool

### Tips
- Self lookup (omit --user-id) needs user identity; a bot must pass --user-id
- --user-id-type must match the id you pass (default open_id)

## user_profiles batch_query
Bulk-fetch personal status and signature for user ids you already have.

### Avoid when
- Need more than status/signature (name, dept, email), or don't have the open_id yet → use [[+search-user]]

### Tips
- Off by default — set include_personal_status / include_description to true under query_option
- ids in user_ids must match --user-id-type (default open_id)

### Examples

**Bulk-query status and signature**
```bash
lark-cli contact user_profiles batch_query --data '{"user_ids":["ou_3a8b****6a7b"],"query_option":{"include_personal_status":true,"include_description":true}}'
```
