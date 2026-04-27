# Docs CLI E2E Coverage

## Metrics
- Denominator: 8 leaf commands
- Covered: 3
- Coverage: 37.5%

## Summary
- TestDocs_CreateAndFetchWorkflow: proves `docs +create` and `docs +fetch`; key `t.Run(...)` proof points are `create as bot` and `fetch as bot`.
- TestDocs_CreateAndFetchWorkflowAsUser: proves the same shortcut pair with UAT injection via `create as user` and `fetch as user`; creates its own Drive folder fixture first, then reads back the created doc by token.
- TestDocs_UpdateWorkflow: proves `docs +update` via `update-title-and-content as bot`, then re-fetches the same doc in `verify as bot` to assert persisted title/content changes.
- Setup note: docs workflows create a Drive folder through `drive files create_folder` in `helpers_test.go`; that helper is external to the docs domain and is not counted here.
- Blocked area: media and search shortcuts still need deterministic fixtures and local file orchestration.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | docs +create | shortcut | docs/helpers_test.go::createDocWithRetry; docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflowAsUser/create as user | `--folder-token`; `--title`; `--markdown` | helper asserts returned doc id |
| ✓ | docs +fetch | shortcut | docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflow/fetch as bot; docs_update_test.go::TestDocs_UpdateWorkflow/verify as bot; docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflowAsUser/fetch as user | `--doc <docToken>` | |
| ✕ | docs +media-download | shortcut |  | none | no media fixture workflow yet |
| ✕ | docs +media-insert | shortcut |  | none | requires deterministic upload fixture and rollback assertions |
| ✕ | docs +media-preview | shortcut |  | none | requires deterministic media fixture |
| ✕ | docs +search | shortcut |  | none | search results are ambient and not yet stabilized for E2E |
| ✓ | docs +update | shortcut | docs_update_test.go::TestDocs_UpdateWorkflow/update-title-and-content as bot | `--doc`; `--mode overwrite`; `--markdown`; `--new-title` | |
| ✕ | docs +whiteboard-update | shortcut |  | none | requires whiteboard fixture and DSL-specific assertions |
