# im +chat-create

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +chat-create`. **Run `lark-cli im +chat-create --help` for the authoritative flags (`--name` / `--description` / `--users` / `--bots` / `--owner` / `--type` / `--chat-mode` / `--set-bot-manager` / `--as`), limits, and enums.** This file covers only what `--help` cannot.

Identity choice follows [Group Chat Identity Rules](lark-im-chat-identity.md). Scope: `--as bot` needs `im:chat:create`; `--as user` needs `im:chat:create_by_user`.

## Gotchas

- **`--chat-mode topic` ≠ "normal group in topic-message mode".** `topic` makes the *entire* group a 话题群. A normal group (`chat_mode=group`) with `group_message_type=thread` is a different thing — and this CLI intentionally does NOT surface `group_message_type`. When the user asks for a topic chat, pass `--chat-mode topic` explicitly; never rely on the default.
- **`--type public` defaults to `private`** — only pass `public` when the user explicitly asks for a discoverable group.
- **`--set-bot-manager` is silently a no-op without `--as bot`** (effective only when the creating bot is also a member).

## The bot-invisible-members trap → create-then-add recipe

A bot **cannot** invite users who are mutually invisible to it during creation — passing them in `--users` fails the *whole* request with **232043**. Do NOT pass arbitrary users in `--users` under `--as bot`. Instead:

1. Resolve the **current user's** open_id (`lark-cli contact +search-user --query "<name|email>"`).
2. Create the group with the bot, including **only the current user** (omit `--users` entirely only if the user explicitly says "bot-only group" / "do not add me"):

   ```bash
   lark-cli im +chat-create --name "<name>" --users "<current user open_id>" --as bot
   ```

3. Add the remaining members with **user identity** (requires the current user to be in the group), tolerating unreachable IDs via `succeed_type=1`:

   ```bash
   lark-cli im chat.members create \
     --params '{"chat_id":"<chat_id from step 2>","member_id_type":"open_id","succeed_type":1}' \
     --data '{"id_list":["ou_aaa","ou_bbb"]}' --as user
   ```

4. **Check `invalid_id_list`** in the response; report any members that could not be added.

Under `--as user` there is no visibility limit, so create + invite happens in one call; the authorized user is automatically creator and member.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Output fields**: `chat_id` (oc_xxx) · `name` · `chat_type` (`private` / `public`) · `owner_id` (**may be empty** when a bot creates the group and `--owner` is unset) · `external` · `share_link` (**omitted if retrieval fails**).
