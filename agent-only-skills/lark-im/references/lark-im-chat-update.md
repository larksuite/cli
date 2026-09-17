# im +chat-update

> **Prerequisite:** Read [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

Update a group's name or description. Uses the current Agent credential.

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
| Permission denied (99991679) | Missing `im:chat:update` permission | Follow lark-shared Agent permission guidance |
| Non-owner/admin cannot update (232016/232002/232017) | Caller is not owner/admin | Report the required role; do not switch identities |
| Not in the group (232011) | Caller is not a member | Report the required membership |

## AI Usage Guidance

### Identity Selection

`+chat-update` uses the current Agent credential; `--as user` remains valid.

Use the configured Agent caller. If ownership is unclear, query the group and confirm `owner_id`; report role restrictions rather than selecting another identity.

Owner and admin requirements follow [Chat caller and targets](lark-im-chat-identity.md): confirm the Agent holds the required role on the target chat before a role-restricted change, and report the missing role instead of retrying.

## References

- [lark-im](../SKILL.md) - all IM commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
