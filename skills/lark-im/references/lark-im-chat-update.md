# im +chat-update

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

Update a group's name or description. Supports both **TAT (bot)** and **UAT (user)** identity.

This skill maps to the shortcut: `lark-cli im +chat-update` (internally calls `PUT /open-apis/im/v1/chats/:chat_id`).

## Commands

```bash
# Update the group name
lark-cli im +chat-update --chat-id oc_xxx --name "New Group Name"

# Update the group description
lark-cli im +chat-update --chat-id oc_xxx --description "Updated group description"

# Update multiple fields at once
lark-cli im +chat-update --chat-id oc_xxx \
  --name "Q2 Project Team" \
  --description "Owns Q2 goal tracking"

# Preview the request without executing it
lark-cli im +chat-update --chat-id oc_xxx --name "Test" --dry-run
```

## Parameters

### Required

| Parameter | Description |
|------|------|
| `--chat-id <oc_xxx>` | Group ID |

### Optional Fields

| Parameter | Limits | Description |
|------|------|------|
| `--name <name>` | Max 60 characters | Group name |
| `--description <text>` | Max 100 characters | Group description |

### Global Parameters

| Parameter | Description |
|------|------|
| `--format json` | Output as JSON (default) |
| `--dry-run` | Preview the request without executing it |

## Usage Scenarios

### Scenario 1: Rename a group and update its description

```bash
lark-cli im +chat-update --chat-id oc_xxx \
  --name "Q2 Project Team" \
  --description "Owns Q2 goal tracking"
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `invalid --chat-id: expected chat ID (oc_xxx)` | Invalid chat_id format | Use a valid `oc_xxx` chat ID |
| `--name exceeds the maximum of 60 characters` | Group name too long | Shorten the name to 60 characters or fewer |
| `--description exceeds the maximum of 100 characters` | Group description too long | Shorten the description to 100 characters or fewer |
| `at least one field must be specified to update` | No update field was provided | Specify at least one field to update |
| Permission denied (99991679) | Missing `im:chat:update` permission | Run `lark-cli auth login --scope "im:chat:update"` |
| Non-owner/admin cannot update (232016/232002/232017) | Current identity is not the owner/admin | Read the current owner/admin state. If the requested actor lacks authority, stop before writing; do not switch actor or try the write first |
| Not in the group (232011) | The requested actor is not a member of the group | Read current membership. If that actor is not a member, stop or ask the user to add it; do not retry as another actor |

## AI Usage Guidance

### Identity Selection

`+chat-update` supports both user and bot identity (`--as user` / `--as bot`).

Read the current group state and confirm `owner_id` or admin authority before writing. Creation history may guide discovery, but it does not override current state or a user-requested actor.

Identity choice follows [Group Chat Identity Rules](lark-im-chat-identity.md): preserve an explicit actor; if current authority proves that actor cannot perform the update, stop before the write instead of switching identities.

## References

- [lark-im](../SKILL.md) - all IM commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
