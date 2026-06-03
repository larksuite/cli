# lark-wiki +member-remove

Remove a member from a wiki space. OpenAPI: `DELETE /open-apis/wiki/v2/spaces/:space_id/members/:member_id`. Unlike most DELETEs, this endpoint **requires a body** carrying `member_type` and `member_role` — the `:member_id` path segment alone is ambiguous without both.

> The underlying `members.delete` API is flagged `danger: true` in the schema browser, but the operation is recoverable — call [`+member-add`](lark-wiki-member-add.md) with the same `(member_id, member_type, member_role)` to restore. No `--yes` gate.

## Usage

```bash
lark-cli wiki +member-remove \
  --space-id <space_id> \
  --member-id <open_id|email|user_id|app_id|...> \
  --member-type <openid|email|userid|unionid|openchat|opendepartmentid|appid> \
  --member-role <admin|member> \
  [--as user|bot]

# Personal library (resolves my_library first)
lark-cli wiki +member-remove \
  --space-id my_library \
  --member-id ou_xxx --member-type openid --member-role member \
  --as user

# Preview the call chain without deleting
lark-cli wiki +member-remove \
  --space-id <id> --member-id <id> --member-type openid --member-role admin \
  --dry-run
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--space-id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `--member-id` | string | **Yes** | — | Member ID; interpretation is decided by `--member-type` |
| `--member-type` | enum | **Yes** | — | Must **match the original grant**: `openchat` / `userid` / `email` / `opendepartmentid` / `openid` / `unionid` / `appid` |
| `--member-role` | enum | **Yes** | — | Must **match the original grant**: `admin` / `member` |
| `--as` | enum | No | `auto` | Identity `user`/`bot`; wiki is user-centric → pass `--as user` |

## Output

```json
{
  "space_id": "7160145948494381236",
  "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
  "member_type": "openid",
  "member_role": "admin"
}
```

If the API ever omits the member echo, the CLI falls back to surfacing the caller-supplied `(member_id, member_type, member_role)` so scripts still see what was removed.

## Notes

- **`--member-type` and `--member-role` must match the original grant.** Revoking a non-existent `(member_id, type, role)` tuple is a no-op error from the API. If you do not know the current role, run [`+member-list`](lark-wiki-member-list.md) first.
- **Role switch is not a single update.** To move someone between `admin` and `member`, call `+member-remove` with the old role first, then [`+member-add`](lark-wiki-member-add.md) with the new one.
- **Bot + `my_library` is rejected upfront.** Pass an explicit `--space-id` when `--as bot`.
- `--dry-run` previews 2 steps when `--space-id my_library` (resolve → delete), 1 step otherwise.

## Required Scope

`wiki:member:update`
