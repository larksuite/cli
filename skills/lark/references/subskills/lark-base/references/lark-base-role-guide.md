# Base advanced permission and role guide

This guide is the entry point for Base advanced permissions and roles. Use it to choose commands and understand safety boundaries. For the permission JSON itself, use [role-config.md](role-config.md) as the SSOT.

## Command selection

| Goal | Command | Notes |
|------|---------|-------|
| Enable advanced permissions | `+advperm-enable` | Required before creating or updating roles. Caller must be a Base admin. |
| Disable advanced permissions | `+advperm-disable` | High-risk write. Disabling invalidates existing custom roles. |
| Locate roles | `+role-list` | Returns role summaries. Use `+role-get` for full config. |
| Inspect one role | `+role-get` | Use before updating a role or deciding whether a role can be deleted. |
| Create a custom role | `+role-create` | Supports `custom_role` only. Read [role-config.md](role-config.md) before constructing `--json`. |
| Update a role | `+role-update` | Delta merge. Read current config first, then send only intended changes. |
| Delete a role | `+role-delete` | Custom roles only. System roles cannot be deleted. |

## Safety boundaries

- Role operations require advanced permissions to be enabled and the caller to be a Base admin.
- `+role-create` creates custom roles only.
- `+role-delete` is only for custom roles. System roles such as editor/reader can be configured within supported limits, but cannot be deleted.
- `+role-update` uses delta merge: omitted fields remain unchanged, but identity fields such as `role_name` and `role_type` should match the current target role.
- `+advperm-disable` invalidates existing custom roles; confirm the target Base and user intent before passing `--yes`.

## Common Fewshots

Use these fewshots for simple role changes. For table, field, record, dashboard, docx, or filter permission details, switch to [role-config.md](role-config.md).

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

Use [role-config.md](role-config.md) for:

- `AdvPermBaseRoleConfig` top-level structure.
- `base_rule_map`, `table_rule_map`, `dashboard_rule_map`, and `docx_rule_map`.
- Table, view, field, record, dashboard, and docx permission values.
- Filter permission JSON.
- Default permission strategy and risk rules.
