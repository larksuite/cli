# Apps CLI E2E Coverage

## Metrics
- Denominator: 18 leaf commands in the selected apps E2E coverage set (not all 79 apps shortcuts)
- Selected command coverage: 100% (18/18)
- API dry-run coverage: 100% (16/16 API-backed commands)
- Local E2E coverage: 100% (2/2 local-only commands)
- Live coverage: tracked role workflows are intentionally fixture-gated and skipped by default CI. When run manually with dedicated fixtures, a transient-role lifecycle covers create/get/update, member add/list/`--all` clear, role-presence readback, delete, and target-ID absence readback; shared-fixture workflows separately cover explicit member removal and `+role-match-list`.

## Summary
- `TestAppsCreateDryRun`: happy path with `--app-type html`, all-fields shape, rejection paths (missing name, missing app-type, invalid app-type, legacy uppercase `HTML`). `--app-type` is a strict lowercase enum (`html`/`full_stack`); the CLI does not normalize case — legacy uppercase compatibility is a server concern.
- `TestAppsUpdateDryRun`: partial-field PATCH semantics; `--app-id` and at-least-one-field validation.
- `TestAppsListDryRun`: default `page_size=20`; empty `--page-token` omitted; negative size passed through to server (no client-side bound check); `--keyword`/`--ownership`/`--app-type` pass-through + empty-omission; invalid `--ownership` and legacy uppercase `--app-type` enum rejection.
- `TestAppsAccessScopeSetDryRun`: CLI input `specific`/`public`/`tenant` -> server enum `Range`/`All`/`Tenant`; `apply_config.approvers` shape; four mutex rejection paths.
- `TestAppsAccessScopeGetDryRun`: URL shape; no body/params on GET; `--app-id` required.
- `TestAppsHTMLPublishDryRun`: walker manifest for directory + single file; hidden files intentionally included (design decision); empty dir / missing `index.html` produce envelope `validation_error` field (dry-run exits 0 advisory, not blocking); both required-flag rejections.
- `TestAppsGitCredentialInitDryRun`: URL shape for issuing an app Git PAT; no body; `app_id` query metadata included.
- `TestAppsGitCredentialListLocalE2E`: local-only command scans every app storage directory and reports repository URL and status without exposing PAT or expiry details.
- `TestAppsGitCredentialRemoveLocalE2E`: local cleanup command removes app-scoped metadata under an isolated config dir.
- `TestAppsRoleManagementDryRun_RequestShapes`: role CRUD/member/match request shapes for all 9 role shortcuts. Request/response fields follow the API contract: `name`, `role_id`, `users`, `departments`, `chats`, `target_user_id`, and `roles`.
- `TestAppsRoleManagementValidation`: deterministic typed validation for invalid role ID, page bounds/token, missing update fields, missing member input, `--all` conflicts, and missing `--user-id`.
- `TestAppsRoleManagementLiveWorkflow`: fixture-gated live role/member workflow against the role provided by `LARK_CLI_E2E_APPS_ROLE_ID`. It refuses to run when the selected member already exists, mutates only that member, and removes only that member during cleanup; it never changes a shared role definition or clears unrelated members.
- `TestAppsRoleLifecycleLiveWorkflow`: creates a uniquely named transient role, independently reads it back, updates and re-reads it, adds a fixture member, clears all members and proves the role still exists, then deletes it and verifies the target `role_id` is absent. Cleanup is armed before creation and uses only environment-provided test identifiers.
- `TestAppsRoleMatchListLiveWorkflow`: separately fixture-gated live `+role-match-list` proof against the same isolated fixture role. It also requires the selected user to be absent at baseline and removes only the user it added.

Blocked: General app create live E2E is intentionally not implemented yet. Apps has no `+delete` endpoint (OAPI doc explicitly defers archive/delete), so a create-and-cleanup workflow would leak tenant state. Selected role read/member/match live flows intentionally remain fixture-gated and skipped by default because they mutate app role members.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | apps +create | shortcut | apps_create_dryrun_test.go::TestAppsCreateDryRun | `--name`, `--app-type` (required, case-sensitive, `html`/`full_stack`), `--description`, `--icon-url` | live blocked: no +delete to clean up |
| ✓ | apps +update | shortcut | apps_update_dryrun_test.go::TestAppsUpdateDryRun | `--app-id`; at least one of `--name`/`--description` | live blocked: no +delete |
| ✓ | apps +list | shortcut | apps_list_dryrun_test.go::TestAppsListDryRun | `--keyword`; `--ownership` (enum all/mine/shared); `--app-type` (enum html/full_stack); `--page-size` default 20; `--page-token` cursor | live blocked: needs tenant fixtures |
| ✓ | apps +access-scope-set | shortcut | apps_access_scope_set_dryrun_test.go::TestAppsAccessScopeSetDryRun | `--scope specific/public/tenant`; `--targets` JSON; `--apply-enabled --approver`; `--require-login` | live blocked: needs real open_ids |
| ✓ | apps +access-scope-get | shortcut | apps_access_scope_get_dryrun_test.go::TestAppsAccessScopeGetDryRun | `--app-id` | live blocked: depends on +access-scope-set state |
| ✓ | apps +html-publish | shortcut | apps_html_publish_dryrun_test.go::TestAppsHTMLPublishDryRun | `--app-id`, `--path` (file or directory containing `index.html`) | live blocked: real upload has side effects; no rollback API |
| ✓ | apps +git-credential-init | shortcut | apps_git_credential_dryrun_test.go::TestAppsGitCredentialInitDryRun | `--app-id`; dry-run `GET /open-apis/spark/v1/apps/{app_id}/git_info` | live blocked: issues short-lived repository PAT |
| ✓ | apps +git-credential-list | shortcut | apps_git_credential_local_test.go::TestAppsGitCredentialListLocalE2E | no `--app-id`; scans all local app storage directories and reports `app_id`, repository URL, and status without PAT or expiry | local E2E only: no dry-run API because command is local read only |
| ✓ | apps +git-credential-remove | shortcut | apps_git_credential_local_test.go::TestAppsGitCredentialRemoveLocalE2E | `--app-id`; deletes local metadata, keychain PAT, and Git config | local E2E only: no dry-run API because command is local cleanup only |
| ✓ | apps +role-list | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes | `--app-id`; `--name` -> `name`; `--page-size` -> `limit`; `--page-token` -> `offset` | live depends on a role-capable app fixture |
| ✓ | apps +role-get | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes | `GET /roles/:role_id`; no body/query | live covered by fixture-gated role workflow |
| ✓ | apps +role-create | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes; apps_role_management_test.go::TestAppsRoleLifecycleLiveWorkflow | `POST /roles`; required `name`; optional `role_id` | transient live role is independently read back |
| ✓ | apps +role-update | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes; apps_role_management_test.go::TestAppsRoleLifecycleLiveWorkflow | `PATCH /roles/:role_id`; only changed fields | transient live role update is independently read back |
| ✓ | apps +role-delete | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes; apps_role_management_test.go::TestAppsRoleLifecycleLiveWorkflow | `DELETE /roles/:role_id`; high-risk confirmation | live flow verifies the target `role_id` is absent after deletion |
| ✓ | apps +role-member-list | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes | `GET /member_list`; optional `member_type=user/department/chat`; no pagination | live covered by fixture-gated role workflow for a provided chat member when available, otherwise a user member |
| ✓ | apps +role-member-add | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes | `POST /member_add`; body `users/departments/chats` open_id arrays | live covered by fixture-gated role workflow for a provided chat member when available, otherwise a user member |
| ✓ | apps +role-member-remove | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes; apps_role_management_test.go::TestAppsRoleManagementLiveWorkflow; apps_role_management_test.go::TestAppsRoleLifecycleLiveWorkflow | `POST /member_remove`; body `users/departments/chats` open_id arrays or `all=true`; high-risk confirmation | explicit removal and `--all` both have fixture-gated live readback coverage |
| ✓ | apps +role-match-list | shortcut | apps_role_management_test.go::TestAppsRoleManagementDryRun_RequestShapes; apps_role_management_test.go::TestAppsRoleMatchListLiveWorkflow | `POST /user_role_list`; body `target_user_id`; no `role_id`; response field is `roles` per the API contract | automated live runs only when `LARK_CLI_E2E_APPS_ROLE_MATCH_READY=1`; it reuses the role provided by `LARK_CLI_E2E_APPS_ROLE_ID` instead of creating a transient role |
