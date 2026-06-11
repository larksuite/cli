# lark-wiki +member-add

Add a member to a wiki space. OpenAPI: `POST /open-apis/wiki/v2/spaces/:space_id/members`. Shortcut over the raw `wiki members create` — adds enum hints, optional `--need-notification`, `my_library` resolution, and a flattened single-member output envelope.

> The underlying `members.create` API is flagged `danger: true` in the schema browser, but adding a member is **not** confirmation-gated (no `--yes`). To revert, call [`+member-remove`](lark-wiki-member-remove.md) with the same `(member_id, member_type, member_role)` tuple.

## Usage

```bash
# Add a user as a regular member
lark-cli wiki +member-add \
  --space-id <space_id> \
  --member-id <open_id|email|user_id|app_id|...> \
  --member-type <openid|email|userid|unionid|openchat|opendepartmentid|appid> \
  --member-role <admin|member> \
  [--need-notification] \
  [--as user|bot]

# Personal library (resolves my_library to the per-user real space first)
lark-cli wiki +member-add \
  --space-id my_library \
  --member-id ou_xxx --member-type openid --member-role member \
  --as user

# Preview the call chain without writing
lark-cli wiki +member-add \
  --space-id <space_id> --member-id <id> --member-type openid --member-role admin \
  --dry-run
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--space-id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `--member-id` | string | **Yes** | — | Member ID; interpretation is decided by `--member-type` |
| `--member-type` | enum | **Yes** | — | `openchat` / `userid` / `email` / `opendepartmentid` / `openid` / `unionid` / `appid` |
| `--member-role` | enum | **Yes** | — | `admin` (full space administration) / `member` (collaborator) |
| `--need-notification` | bool | No | unset | Send an in-app notification after the grant. **Omitting the flag sends no `need_notification` query at all** — passing `--need-notification=false` is the explicit opt-out |
| `--as` | enum | No | `auto` | Identity `user`/`bot`; wiki is user-centric → pass `--as user` |

## Output

```json
{
  "space_id": "7160145948494381236",
  "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
  "member_type": "openid",
  "member_role": "admin",
  "type": "user"
}
```

`type` is a read-only enum (`user` / `chat` / `department`) the server attaches; absent when the API omits it.

## Notes

- **Bot + `my_library` is rejected upfront** — `my_library` is a per-user alias with no meaning for a tenant token. Pass an explicit `--space-id` when `--as bot`.
- **Bot + `opendepartmentid` is a known unsupported path on the backend.** The CLI does not pre-block it (the API may evolve), but the call will fail. Use `--as user` for department adds.
- **App member uses `--member-type=appid`.** The corresponding `--member-id` is the app ID, commonly formatted as `cli_xxx`.
- Resolve `--member-id` **before** calling: `lark-cli contact +search-user` for users, `lark-cli im +chat-search` for groups, `lark-cli api POST /open-apis/contact/v3/departments/search` for departments. Do not call `+member-add` first and reverse-engineer the type from the error.
- The role switch (`admin` ⇄ `member`) is not a single update — call [`+member-remove`](lark-wiki-member-remove.md) for the old role first, then `+member-add` with the new one.
- `--dry-run` previews 2 steps when `--space-id my_library` (resolve → add), 1 step otherwise.

## Required Scope

`wiki:member:create`
