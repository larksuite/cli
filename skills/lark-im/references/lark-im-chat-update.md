# im +chat-update

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +chat-update`. **Run `lark-cli im +chat-update --help` for the authoritative flags (`--chat-id` / `--name` / `--description` / `--as`), limits, and the "at least one field" rule.** This file covers only what `--help` cannot.

## Gotchas

- **Updating requires owner/admin privileges.** A non-owner/non-admin identity fails with **232016 / 232002 / 232017**; an identity that isn't even in the group fails with **232011**. `--help` won't tell you this — pick the identity *before* you call.
- **Infer the owner identity from context** rather than the default (per [Group Chat Identity Rules](lark-im-chat-identity.md)): if the user names an identity, use it; if a bot just created the group, the owner is the bot; otherwise query the group first and confirm `owner_id` before choosing `--as user` / `--as bot`.
- **A bot that is an admin (not owner) can still rename / change settings** under `--as bot` — owner-only actions (e.g. owner transfer) are not exposed here and need the real owner's UAT auth.
