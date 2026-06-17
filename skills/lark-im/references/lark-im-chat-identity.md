# Group Chat Identity Rules

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Concept doc (no direct command). These cross-cutting identity/ownership rules apply to [`+chat-create`](lark-im-chat-create.md), [`+chat-update`](lark-im-chat-update.md), and the member-management flow (`im chat.members ...`). **The most common cause of group-operation failure is choosing the wrong identity** — decide it before acting.

## Gotchas

- **If the user names an identity, use it verbatim** (`--as user` / `--as bot`) — do not second-guess. Only infer when the user is silent; never just take the default.
- **Adding members → prefer `--as user`.** Bot visibility is limited and fails when the target is mutually invisible to the bot (**232024** for member-add, **232043** during create). See the create-then-add recipe in [`+chat-create`](lark-im-chat-create.md); do not retry the bot blindly.
- **Owner-level actions need owner/admin identity** (rename/permissions: **232016 / 232002 / 232017** if under-privileged; **232011** if not in the group).

## Inferring the owner (when an owner-level action is needed and the owner is unknown)

1. Bot created the group, `--owner` **unset** → owner is the bot (`--as bot`).
2. Bot created the group, `--owner ou_xxx` **set** → owner is that user (`--as user`).
3. User created the group, `--owner` **unset** → owner is the current user (`--as user`).
4. Still unclear → **ask** before any owner-level change; don't guess.

## When the owner is a third party (neither current user nor bot)

The current identity has no owner privileges. Then:

- **Rename / permission / setting changes:** if the bot is a group **admin**, `--as bot` can still perform these admin-level operations.
- **Owner-only actions (e.g. owner transfer):** the actual owner must complete UAT auth via `lark-cli auth login` first, then act as that owner. There is no bot workaround.
- Explain the limitation to the user instead of retrying blindly.
