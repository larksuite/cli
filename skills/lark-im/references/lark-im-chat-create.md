# im +chat-create

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

**Run `lark-cli im +chat-create --help` for the authoritative flags, defaults, limits, and enums.** This file covers only what `--help` cannot.

## `--idempotency-key` is required

`--help` lists the flag but does not mark it required — it is. Omitting it fails with
`--idempotency-key is required`. Generate one UUID with a library or tool, pass its **literal value**, and
reuse that same literal unchanged when retrying the same logical creation. Do not inline a generator
expression (`$(uuidgen)`) into the command: a fresh value on each retry defeats the protection entirely.

## AI Usage Guidance

### When using `--as bot`

A bot may fail to invite users who are mutually invisible to it during group creation (**error 232043**). `--help` gives no hint of this, and passing other users in `--users` is the natural-looking move that triggers it. Use the two-step flow instead:

1. **Resolve the current user's open_id** — `lark-cli contact +search-user --query "<name or email>"`.
2. **Create the group with only that user:**

   ```bash
   lark-cli im +chat-create --name "<group name>" \
     --idempotency-key <generated_uuid> \
     --users "<current user open_id>" --as bot
   ```

   **Default behavior:** always add the current user, unless they explicitly say "do not add me" or ask for a bot-only group — only then omit `--users`.

3. **Add the remaining members with user identity** (requires the current user to already be in the group):

   ```bash
   lark-cli im chat.members create \
     --params '{"chat_id":"<chat_id from step 2>","member_id_type":"open_id","succeed_type":1}' \
     --data '{"id_list":["ou_aaa","ou_bbb"]}' \
     --as user
   ```

   `succeed_type=1` makes reachable users succeed while unreachable ones come back in `invalid_id_list` instead of failing the whole request.

4. **Check `invalid_id_list`** in the response. If it is non-empty, report which members could not be added — silence here reads as success to the user.

### When using `--as user`

User identity has no visibility limitation, so creation and invitation happen in one step, and the authorized user is automatically creator and member. Prefer this path whenever the task does not specifically need bot ownership.

## Gotchas

- **`owner_id` can come back empty.** When a bot creates the group and `--owner` is not passed, the response's `owner_id` may be blank even though `--help` says the owner defaults to the bot. Do not treat an empty `owner_id` as a failure, and do not feed it into a follow-up call unchecked.
- **`share_link` is omitted when its retrieval fails**, not set to an empty string. A `jq` path that assumes the key exists will break.

## Common Errors and Troubleshooting

Only errors whose cause or fix is not evident from `--help`:

| Symptom | Root cause | Solution |
|---------|---------|---------|
| Permission denied (99991672) | App lacks `im:chat:create` (bot) or `im:chat:create_by_user` (user) | Enable that permission for the app in the Open Platform console |
| `--idempotency-key is required` | No replay protection supplied | See the section above — generate one UUID, pass the literal, reuse it on retry |
| `bot is invisible to user` (232043) | Bot and target users are mutually invisible | Use the two-step flow above; do not pass other users in `--users` at creation time |

## References

- [lark-im](../SKILL.md) — all IM commands
