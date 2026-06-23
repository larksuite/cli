# Docs CLI E2E Coverage

## Metrics
- Denominator: 8 leaf commands
- Covered: 3
- Coverage: 37.5%

## Summary
- TestDocs_CreateAndFetchWorkflow: proves `docs +create` and `docs +fetch`; key `t.Run(...)` proof points are `create as bot` and `fetch as bot`.
- TestDocs_CreateAndFetchWorkflowAsUser: proves the same shortcut pair with UAT injection via `create as user` and `fetch as user`; creates its own Drive folder fixture first, then reads back the created doc by token.
- TestDocs_UpdateWorkflow: proves `docs +update` via `update-title-and-content as bot`, then re-fetches the same doc in `verify as bot` to assert persisted title/content changes.
- TestDocs_DryRunDefaultsToV2OpenAPI: proves `docs +create`, `docs +fetch`, and `docs +update` dry-run all emit `/open-apis/docs_ai/v1/...` requests without MCP or `--api-version` guidance; its fetch case asserts fetch sends the default `extra_param`, and its update case asserts `--reference-map` is sent as request body `reference_map`.
- TestDocs_CreateTitleDryRunPrependsContent: proves `docs +create --title` dry-run prepends an escaped `<title>...</title>` tag to request body `content`.
- Setup note: docs workflows create a Drive folder through `drive files create_folder` in `helpers_test.go`; that helper is external to the docs domain and is not counted here.
- Blocked area: media and search shortcuts still need deterministic fixtures and local file orchestration.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | docs +create | shortcut | docs/helpers_test.go::createDocWithRetry; docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflowAsUser/create as user; docs_update_dryrun_test.go::TestDocs_DryRunDefaultsToV2OpenAPI/create; docs_update_dryrun_test.go::TestDocs_CreateTitleDryRunPrependsContent | `--parent-token`; `--doc-format markdown`; `--content`; `--title` | helper asserts returned doc id from `data.document.document_id`; dry-run asserts title is prepended into request body content |
| ✓ | docs +fetch | shortcut | docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflow/fetch as bot; docs_update_test.go::TestDocs_UpdateWorkflow/verify as bot; docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflowAsUser/fetch as user; docs_update_dryrun_test.go::TestDocs_DryRunDefaultsToV2OpenAPI/fetch | `--doc <docToken>`; `--doc-format markdown`; default `extra_param.enable_user_cite_reference_map=true` | |
| ✕ | docs +media-download | shortcut |  | none | no media fixture workflow yet |
| ✕ | docs +media-insert | shortcut |  | none | requires deterministic upload fixture and rollback assertions |
| ✕ | docs +media-preview | shortcut |  | none | requires deterministic media fixture |
| ✕ | docs +search | shortcut |  | none | search results are ambient and not yet stabilized for E2E |
| ✓ | docs +update | shortcut | docs_update_test.go::TestDocs_UpdateWorkflow/update-title-and-content as bot; docs_update_dryrun_test.go::TestDocs_DryRunDefaultsToV2OpenAPI/update | `--doc`; `--command overwrite`; `--doc-format markdown`; `--content`; optional `--reference-map` -> body `reference_map` | |
| ✕ | docs +whiteboard-update | shortcut |  | none | requires whiteboard fixture and DSL-specific assertions |
