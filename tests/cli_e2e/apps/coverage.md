# Apps CLI E2E Coverage

## Metrics
- Denominator: 9 leaf commands (all user-visible shortcuts)
- Command coverage: 100% (9/9)
- API dry-run coverage: 100% (7/7 API-backed commands)
- Local E2E coverage: 100% (2/2 local-only commands)
- Live coverage: 0%

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

Blocked: Live E2E intentionally not implemented yet. Apps has no `+delete` endpoint (OAPI doc explicitly defers archive/delete), so a create-and-cleanup workflow would leak tenant state. Revisit when the server exposes `DELETE /apps/{appId}`.

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
