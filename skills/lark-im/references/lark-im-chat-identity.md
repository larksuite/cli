# Group Chat Identity Rules

> Warning: The most common source of failure in group operations is choosing the wrong identity. Confirm the identity before performing the action.

Group-chat operations support both `--as user` (UAT user identity) and `--as bot` (TAT bot identity). Choosing the correct identity is critical for success.

## Basic Principles

- **If the user explicitly specifies an identity:** use exactly what the user requested (`--as user` or `--as bot`) without guessing.
- **Before a permission-sensitive write:** read the current owner/admin/member state. If the requested actor lacks authority, stop before writing; never switch actor or try the write first.
- **If the user does not specify an identity:** use command constraints and current authority as evidence; examples, credentials, empty results, and permission errors never justify an identity switch.

## Identity Selection by Operation

| Operation | Recommended Identity | Why |
|------|---------|-----------------------------------|
| Create group (`+chat-create`) | Depends on the scenario | Infer from context |
| Add members (member-management flow) | `--as user` | Bot visibility is limited and often fails when the target user is mutually invisible to the bot (232024) |
| Update group (`+chat-update`) | Owner identity | Permission changes require owner/admin privileges; owner transfer requires owner identity |

## Verifying the Owner

Read the current chat state and confirm `owner_id` before an owner-level write. Creation provenance may be used only to keep dependent discovery reads on the same identity; it is not proof of current ownership and cannot select the write actor by itself. If the current owner cannot be established, stop and ask instead of trying both identities.

### When the Owner Is Neither the Current User Nor the Bot

If the query shows that the owner is a third-party user (`owner_id` is neither the currently authorized user nor the bot), the current identity does not have owner privileges. In that case:

- **Permission/setting changes:** if the bot is an admin of the group, `--as bot` can still perform admin-level operations such as renaming the group or changing permissions.
- **Owner-only actions such as owner transfer:** require the actual owner to complete UAT authorization via `lark-cli auth login`, then perform the action as that owner.
- Explain the limitation clearly to the user instead of retrying blindly.

## Common Pitfalls

### Inviting Members During Group Creation

If a bot creates a group and `--users` includes users who are mutually invisible to the bot, the entire request fails with 232043. Use two steps instead:

1. Generate a UUID once with a UUID library or tool, then create the group with the bot first, excluding invisible users: `lark-cli im +chat-create --name "Group Name" --idempotency-key <generated_uuid>`
2. Add users later with a user-identity member-management flow

### Insufficient Privileges

- **232016 / 232002 / 232017:** read current owner/admin state; if the requested actor lacks authority, stop before writing
- **232011:** read current membership; do not retry the write as another actor
- **232024:** the bot and target user are mutually invisible; stop the current write and explain the visibility constraint instead of switching identities implicitly

## References

- [lark-im](../SKILL.md) - all IM commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
