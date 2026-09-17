# Chat caller and targets

Use the configured Agent credential. `--as user` is valid; the Agent is the caller, while users or bots named as members and owners remain target entities.

Read the target chat's owner/admin/member fields before role-restricted changes. A permission failure requires reporting the missing role or resource access; do not change caller identity to bypass it. A requesting human is not automatically the creator, owner or member of a group created by the Agent. Explicitly include the intended people where the workflow requires membership.

## Inferring the owner

Owner-level actions — permission changes and owner transfer — require the Agent to be the owner or an admin of the target chat. When the owner is unknown, infer in this order:

1. The Agent created the group and `--owner` was **not** specified -> the Agent owns it.
2. The Agent created the group and `--owner ou_xxx` **was** specified -> that user owns it.
3. Someone else created the group -> that person, or whoever they transferred it to, owns it.
4. Still unclear -> read the chat's `owner_id` before attempting an owner-level change.

When `owner_id` is neither the Agent nor a role the Agent holds, the current principal has no owner privileges. Owner-only actions cannot be completed; report the limitation and the actual owner rather than retrying.

## Permission error codes

These are role and membership problems, not app scope problems. None of them is fixed by repeating the same request.

| Code | Meaning | What to do |
|---|---|---|
| `232016` / `232002` / `232017` | The caller is neither owner nor admin of the chat | Report which role is missing; ask the owner to grant it or to perform the action |
| `232011` | The caller is not a member of the chat | Have the Agent added to the chat where the workflow allows it, or ask a member to perform the action |

## References

- [lark-im](../SKILL.md) - all IM commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
