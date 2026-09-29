# Base advanced permissions and roles

This is the module entry point for Base advanced permissions and roles. Use it to choose commands and understand safety boundaries. For the permission JSON itself, use [Role Permission Schema](lark-base-role-config.md).

Use `lark-drive` for ordinary Base collaborators and document link-access settings. These requests alone do not require enabling advanced permissions or creating roles. Use the role commands below for advanced role configuration or custom-role membership, and `+advperm-share-*` for sharing authorization of system roles.

Whether dashboard statistics follow the visitor's permissions is controlled by
`is_adv_perm_enable`. Use [仪表盘跟随访问者权限统计开关](lark-base-dashboard-advperm.md)
for that setting. `+advperm-enable/disable` controls advanced permissions for the containing Base, while
`dashboard_rule_map` controls a role's access to dashboards.

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
| Manage system Editor/Reader members | `drive +member-list/add/remove` | Drive/CCM sharing; see the system-member workflow below. |
| Manage custom-role members | `+role-member-list/add/remove` | Users, departments and chats; requires Base admin access. |
| Read or change sharing authorization | `+advperm-share-get/update` | Requires advanced permissions with system roles and Base admin access. |
| Request access as a non-admin | `+advperm-apply` | User identity only; submission does not grant access. |

## Required order

The admin preflight below applies to advanced-role configuration and custom-role member management. System-role collaborator operations follow the Drive path below. An access request uses the applicant's identity directly; do not require an admin-only role read before submitting it.

At the start of a role workflow, before the first `+role-list`, `+role-get`, `+role-create`, `+role-update`, or `+role-delete` call:

1. Run `lark-cli base +base-get --base-token <base_token>` and inspect `data.base.is_advanced`.
2. If `is_advanced` is `false`, report that state for a read-only query. Enable it only for an authorized advanced-permission configuration task before proceeding with role commands. A request to inspect settings or members does not authorize enabling advanced permissions. If a write request does not authorize enabling them, stop and explain that advanced permissions must be enabled first.
3. Run the requested role commands only after `is_advanced` is `true` or `+advperm-enable` succeeds. Reuse that confirmed status for later role calls in the same workflow.

Do not probe with `+advperm-get`: that command is not supported. Do not use an empty `+role-list` response to infer the advanced permission status; a disabled Base can also return an empty list.

## Safety boundaries

- Advanced-role configuration and custom-role member operations require advanced permissions to be enabled and the caller to be a Base admin. System-role collaborator operations use Drive sharing permissions.
- `+role-create` creates custom roles only.
- `+role-delete` is only for custom roles. System roles such as editor/reader can be configured within supported limits, but cannot be deleted.
- `+role-update` uses delta merge: omitted fields remain unchanged, but identity fields such as `role_name` and `role_type` should match the current target role.
- `+advperm-disable` invalidates existing custom roles; confirm the target Base and user intent before passing `--yes`.
- `+role-member-remove` is a high-risk write that requires `--yes`: it revokes the members' role access. Confirm the role and members first.

## Role scope and verification

Use the returned `role_type` to distinguish custom roles from system roles, rather than inferring the type from a display name. Treat an unspecified "administrator" as a custom business role; configure system roles only when the user explicitly requests system administrators, system Editor/Reader roles, or sharing identities.

- For advanced-role configuration or custom-role membership, list roles and resolve each requested target to its returned ID and type. Reuse an existing matching role, including an explicitly named default or system role, within the command's supported limits. When operating on an existing role, clarify a missing or ambiguous target; a member-only request does not authorize creating a role or substituting a similarly named one.
- Create custom roles only for the requested business configuration. Check the actual roles after enabling advanced permissions; an "普通用户" (ordinary user) or other default role need not exist and should not be added automatically.
- An "普通用户" role with `role_type=custom_role` is within the business-role review scope. For explicit exclusivity requirements such as "only Finance can view" or "only team leads can edit", inspect all relevant custom roles, including existing ordinary-user roles, and narrow only the resources and capabilities that violate the requirement. Management and custom administrator roles receive no implicit exemption.
- Preserve system Editor/Reader roles unless the user explicitly requests changes to system roles or sharing authorization. Configuring business roles does not itself authorize clearing system-role permissions or disabling sharing.
- A role's existence does not prove that a particular user belongs to it. Report configuration readback separately from effective access. Deletion requires explicit intent; the name "普通用户" alone is not a reason to delete a role.

### Translate requirements into a target configuration

1. Identify roles, resources, allowed capabilities, and explicit restrictions. Distinguish "only this role can edit" (role exclusivity) from "this role can edit only this table" (resource scope). Resolve ambiguity from context and state the interpretation; clarify first if an unresolved interpretation would revoke other roles' existing permissions.
2. Grant a new custom role access only within its authorized scope. Before updating an existing role, use `+role-get` and preserve unrelated settings. Follow the JSON SSOT for defaults governing table editing, record creation/deletion, and view management. "Disallow creation" excludes only `add`; it does not also remove deletion or view management.
3. For "view only my own records" without an editing requirement, set `perm=read_only`, configure the read filter, and disable record creation, deletion, and view editing. Restricting the visibility of a few fields does not make the entire table read-only; define read and edit scopes separately when the same table also has explicit editing requirements.
4. For "my records and their related details", identify every covered table using the same business ownership relationship. Determine ownership from the actual fields and relations, rather than substituting an approximate creator or applicant field. Detail tables do not inherit the parent table's row filter; configure and read back each table separately. See the JSON SSOT's current-user row-isolation example for cross-table configuration.
5. If column-level control is requested without specific field differences, propose a role/field configuration or clarify the boundaries. Uniformly inheriting table permissions does not complete a request for distinct column permissions. Include dashboards created for the role in the same task in verification, applying their purpose and explicit restrictions; dashboards must not reopen restricted fields.

### Read back each stage

After writing each permission stage, enumerate roles with `+role-list` and read this stage's target roles with `+role-get`. For explicit cross-role exclusivity, also read all relevant custom roles. Deduplicate by role ID and check that no required role was omitted.

Verify allowed capabilities, restrictions, `record_operations`, fields, record filters, and dashboard access. For row isolation, check field types, ownership relationships, `is_invalid=false`, and the absence of full-read settings such as `other_record_all_read=true`. Repair invalid or missing configuration and read it back again. Deleting roles or disabling advanced permissions later does not replace verification of an earlier stage. State incomplete or unsupported requirements explicitly instead of reporting full verification.

## Members, sharing authorization and access requests

These commands require the permission routes in the selected gateway and backend. Keep the host, environment, application and identity consistent across calls. Use read commands for inspection and diagnosis; `--dry-run` previews a request but does not verify deployment, permissions, or a completed write.

Check the command result before reporting success. An HTTP 404, non-JSON response, or timeout does not establish the Base's permission state or a successful change. Report the failed call and what remains unverified; check the selected route, environment, identity, and error details before retrying. For a write with an uncertain outcome, read the current state before retrying to avoid repeating a change that may already have taken effect.

### Custom-role members

Use `+role-list` to obtain the requested role ID and confirm `role_type=custom_role` before calling the custom-role member commands. For system roles, follow the system-member workflow below; do not create a custom-role substitute. Member changes preserve the role's permission rules. Resolve user, department and chat open IDs in the same application and environment; use contact/chat search to find candidates, confirm the intended identity, then pass its ID. If the requested identity is missing or ambiguous, clarify it before granting access; use a different person, department, or chat only when authorized.

`--user-id`, `--department-id` and `--chat-id` are repeatable and may be combined. Each request accepts 1–100 input members and deduplicates identical type/ID pairs. Use the user credential provided by the current environment; member management requires Base admin access.

```bash
lark-cli base +role-member-list --base-token <base_token> --role-id <role_id> --page-size 50 --as user
lark-cli base +role-member-add --base-token <base_token> --role-id <role_id> --user-id <user_open_id> --department-id <department_open_id> --chat-id <chat_open_id> --as user
lark-cli base +role-member-remove --base-token <base_token> --role-id <role_id> --chat-id <chat_open_id> --as user --yes
```

Results are decoded into `data`; inspect `items`, `has_more` and `page_token`. Pass the returned token via `--page-token` to read the next page. Stop if a cursor is missing or repeats while `has_more=true`.

A write can partially succeed. When a successful HTTP response includes nonempty `failed_members`, the command returns `ok:false` on stdout and a nonzero exit code while preserving the per-member results. Check each failed member's `result_code`, list current members, and resolve the cause before retrying only the unresolved ones; other members may already be applied. A refusal without `failed_members` returns an ordinary error. After any member write, read back with `+role-member-list` to confirm the final membership; `+role-get` does not show members. A successful write with nothing to return decodes to an empty `data` object. Member removal does not revoke all Drive/CCM access; other roles or sharing may still grant access. Verify effective access with the affected user.

### System Editor/Reader members

System Editor/Reader membership comes from Drive/CCM sharing. Use `drive +member-list/add/remove` for these grants; `base +role-member-*` only manages custom-role members. Use the returned `role_type` when resolving an advanced role; a display name alone does not determine its type. A request that explicitly targets a sharing identity can use Drive directly, without an admin-only role preflight. Drive checks the caller's sharing permissions; do not require Base admin access or enable advanced permissions just to manage collaborators.

| Requested system role | Drive grant |
|------|------|
| Editor | `--perm edit` |
| Reader | `--perm view` |

Read existing grants first and preserve unrelated access. Do not replace an existing `full_access` grant with edit/view unless the user explicitly requests that downgrade. Resolve the intended member, then use `--member-type openid` for users, `openchat` for chats, or `opendepartmentid` for departments. Department operations require user identity. Batch additions accept up to 10 IDs of the same type; removals take one member per call.

For a standalone Base:

```bash
lark-cli drive +member-list --token <base_token> --type bitable --as user
lark-cli drive +member-add --token <base_token> --type bitable --member-type openchat --member-id <chat_open_id> --perm view --as user --yes
lark-cli drive +member-list --token <base_token> --type bitable --as user
```

For an explicitly requested removal, confirm the member's current grant, then remove it and read back:

```bash
lark-cli drive +member-remove --token <base_token> --type bitable --member-type openchat --member-id <chat_open_id> --as user --yes
lark-cli drive +member-list --token <base_token> --type bitable --as user
```

For a Base in Wiki, use the Wiki node token with `--type wiki` and explicitly pass `--perm-type single_page` to list/add/remove when only that Base is in scope. The add/remove default `container` also affects child pages. Removing a Drive grant can remove the member's document access; it is not a role-only detach. Other grants, groups or inherited access may still apply.

Readback verifies `data.items` by member ID, member type, permission and Wiki scope. A successful removal response alone does not prove the grant previously existed. The Drive list shows collaborator grants, not the exact effective system-role roster: custom-role matches and inherited/group access affect role membership, and sharing authorization can be disabled. Keep those grants distinct from the Web role list. Do not report an empty `+role-member-list` result as an empty system role, or a Drive write as proof of effective Base access. If exact Web membership or effective access is required, verify with the affected identity; otherwise state what remains unverified. Changing sharing authorization or custom-role memberships requires its own user intent.

For command parameters and permission errors, see [Drive member list](../../lark-drive/references/lark-drive-member-list.md), [add](../../lark-drive/references/lark-drive-member-add.md), and [remove](../../lark-drive/references/lark-drive-member-remove.md).

### Sharing authorization

For a status query, use only `+advperm-share-get`. For an authorized change, read the current value, update it, then read it back. This setting controls whether sharing can authorize advanced-permission system roles; it does not disable advanced permissions or replace custom roles. The caller needs Base admin access and a Base with advanced permissions and system roles.

```bash
lark-cli base +advperm-share-get --base-token <base_token> --as user
lark-cli base +advperm-share-update --base-token <base_token> --enable-share=false --as user
lark-cli base +advperm-share-get --base-token <base_token> --as user
```

Set `--enable-share=true` to allow sharing authorization, or `--enable-share=false` to disallow it. Verify the stored value with a successful readback; a submitted request alone does not confirm the setting. Restore the original value after an explicitly authorized temporary test.

### Request access

Submit as the applicant; do not require an admin-only role read first or substitute an administrator's identity. This command accepts only user identity.

```bash
lark-cli base +advperm-apply --base-token <base_token> --message "Please grant access to review this Base" --as user
```

Success means the request was submitted. An administrator must handle it before access changes. On timeout, check whether the request or notification arrived before retrying.

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
