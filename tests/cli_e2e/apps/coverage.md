# Apps CLI E2E Coverage

## Metrics
- Denominator: 6 leaf commands (all shortcuts)
- Covered: 6 (dry-run only)
- Coverage: 100% (dry-run); 0% (live)

## Summary
- `TestAppsCreateDryRun`: happy path with `--app-type HTML`, all-fields shape, three rejection paths (missing name, missing app-type, invalid app-type, case-sensitive lowercase rejection).
- `TestAppsUpdateDryRun`: partial-field PATCH semantics; `--app-id` and at-least-one-field validation.
- `TestAppsListDryRun`: default `page_size=20`; empty `--page-token` omitted; negative size passed through to server (no client-side bound check).
- `TestAppsAccessScopeSetDryRun`: CLI input `specific`/`public`/`tenant` -> server enum `Range`/`All`/`Tenant`; `apply_config.approvers` shape; four mutex rejection paths.
- `TestAppsAccessScopeGetDryRun`: URL shape; no body/params on GET; `--app-id` required.
- `TestAppsHTMLPublishDryRun`: walker manifest for directory + single file; hidden files intentionally included (design decision); empty dir / missing `index.html` rejection; both required-flag rejections.

Blocked: Live E2E intentionally not implemented yet. Apps has no `+delete` endpoint (OAPI doc explicitly defers archive/delete), so a create-and-cleanup workflow would leak tenant state. Revisit when the server exposes `DELETE /apps/{appId}`.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | apps +create | shortcut | apps_create_dryrun_test.go::TestAppsCreateDryRun | `--name`, `--app-type` (required, case-sensitive, `HTML` only), `--description`, `--icon-url` | live blocked: no +delete to clean up |
| ✓ | apps +update | shortcut | apps_update_dryrun_test.go::TestAppsUpdateDryRun | `--app-id`; at least one of `--name`/`--description` | live blocked: no +delete |
| ✓ | apps +list | shortcut | apps_list_dryrun_test.go::TestAppsListDryRun | `--page-size` default 20; `--page-token` cursor | live blocked: needs tenant fixtures |
| ✓ | apps +access-scope-set | shortcut | apps_access_scope_set_dryrun_test.go::TestAppsAccessScopeSetDryRun | `--scope specific/public/tenant`; `--targets` JSON; `--apply-enabled --approver`; `--require-login` | live blocked: needs real open_ids |
| ✓ | apps +access-scope-get | shortcut | apps_access_scope_get_dryrun_test.go::TestAppsAccessScopeGetDryRun | `--app-id` | live blocked: depends on +access-scope-set state |
| ✓ | apps +html-publish | shortcut | apps_html_publish_dryrun_test.go::TestAppsHTMLPublishDryRun | `--app-id`, `--path` (file or directory containing `index.html`) | live blocked: real upload has side effects; no rollback API |

