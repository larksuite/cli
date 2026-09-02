# Base Advanced Permission 与 Role

This is the module entry point for Base advanced permissions and roles. Use it to choose commands and understand safety boundaries. For the permission JSON itself, use [Role Permission Schema](lark-base-role-config.md).

## Command selection

| Goal | Command | Notes |
|------|---------|-------|
| Check advanced permission status | `+base-get` | Read `data.base.is_advanced`. There is no `+advperm-get` command. |
| Enable advanced permissions | `+advperm-enable` | Required before creating or updating roles. Caller must be a Base admin. |
| Disable advanced permissions | `+advperm-disable` | High-risk write. Disabling invalidates existing custom roles. |
| Locate roles | `+role-list` | Returns role summaries. Use `+role-get` for full config. |
| Inspect one role | `+role-get` | Use before updating a role or deciding whether a role can be deleted. |
| Create a custom role | `+role-create` | Supports `custom_role` only. Read [Role Permission Schema](lark-base-role-config.md) before constructing `--json`. |
| Update a role | `+role-update` | Delta merge. Read current config first, then send only intended changes. |
| Delete a role | `+role-delete` | Custom roles only. System roles cannot be deleted. |

## Required order

At the start of a role workflow, before the first `+role-list`, `+role-get`, `+role-create`, `+role-update`, or `+role-delete` call:

1. Run `lark-cli base +base-get --base-token <base_token>` and inspect `data.base.is_advanced`.
2. If `is_advanced` is `false`, run `+advperm-enable` before the role command. If the user did not authorize enabling advanced permissions, stop and explain the required precondition.
3. Run the requested role commands only after `is_advanced` is `true` or `+advperm-enable` succeeds. Reuse that confirmed status for later role calls in the same workflow.

Do not probe with `+advperm-get`: that command is not supported. Do not use an empty `+role-list` response to infer the advanced permission status; a disabled Base can also return an empty list.

## Safety boundaries

- Role operations require advanced permissions to be enabled and the caller to be a Base admin.
- `+role-create` creates custom roles only.
- `+role-delete` is only for custom roles. System roles such as editor/reader can be configured within supported limits, but cannot be deleted.
- `+role-update` uses delta merge: omitted fields remain unchanged, but identity fields such as `role_name` and `role_type` should match the current target role.
- `+advperm-disable` invalidates existing custom roles; confirm the target Base and user intent before passing `--yes`.

## Common Fewshots

Use these fewshots for simple role changes. For table, field, record, dashboard, docx, or filter permission details, switch to [Role Permission Schema](lark-base-role-config.md).

Create a custom role that keeps copy/download disabled:

```bash
lark-cli base +role-create \
  --base-token <base_token> \
  --json '{"role_name":"Reviewer","role_type":"custom_role","base_rule_map":{"copy":false,"download":false}}'
```

Rename a role while preserving its type:

```bash
lark-cli base +role-update \
  --base-token <base_token> \
  --role-id <role_id> \
  --json '{"role_name":"Finance Reviewer","role_type":"custom_role"}' \
  --yes
```

Grant read-only access to one table:

```bash
lark-cli base +role-update \
  --base-token <base_token> \
  --role-id <role_id> \
  --json '{"role_name":"Finance Reviewer","role_type":"custom_role","table_rule_map":{"Orders":{"perm":"read_only"}}}' \
  --yes
```

## JSON SSOT

Use [Role Permission Schema](lark-base-role-config.md) for:

- `AdvPermBaseRoleConfig` top-level structure.
- `base_rule_map`, `table_rule_map`, `dashboard_rule_map`, and `docx_rule_map`.
- Table, view, field, record, dashboard, and docx permission values.
- Filter permission JSON.
- Default permission strategy and risk rules.
