# lark-cli im +chat-disband

Disband a group chat.

This is a high-risk shortcut for `DELETE /open-apis/im/v1/chats/{chat_id}`. It permanently dissolves the target group chat, so real execution requires `--yes`.

Use this for explicit group lifecycle operations or for cleaning up temporary E2E chats created by `lark-cli`.

## Limits

- Only group chat IDs (`oc_xxx`) are accepted.
- Disbanding a group is irreversible.
- Bot calls require the app/bot to have permission to disband the target group, such as being the group owner or having the platform-required group operation permission.
- User calls require the user identity to have permission to disband the group, typically the group owner.
- Tenant and group-type restrictions still apply.

## Examples

Preview the request:

```bash
lark-cli im +chat-disband \
  --chat-id oc_xxx \
  --as bot \
  --dry-run
```

Execute the disband operation:

```bash
lark-cli im +chat-disband \
  --chat-id oc_xxx \
  --as bot \
  --yes
```

User identity:

```bash
lark-cli im +chat-disband \
  --chat-id oc_xxx \
  --as user \
  --yes
```
