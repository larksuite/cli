# Docs CLI E2E Coverage

## Metrics
- Denominator: 7 leaf commands
- Covered: 5
- Coverage: 71.4%

## Summary
- TestDocs_CreateAndFetchWorkflow: proves `docs +create` and `docs +fetch`; key proof points are `create` returning `doc_id` and `fetch` read-after-write title assertion.
- TestDocs_UpdateWorkflow: proves `docs +update` with read-after-write via `docs +fetch`; key proof points are `update-title-and-content` and `verify` title persistence.
- TestDocs_MediaWorkflow: proves `docs +media-insert` and `docs +media-download`; key proof points are `media-insert` returning `file_token` and `media-download` asserting downloaded file exists.
- Blocked area: `docs +search` depends on user login and user identity in current testcase path.
- Blocked area: `docs +whiteboard-update` is skipped because current E2E harness does not support stdin DSL input.

## Command Table
| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | docs +create | shortcut | docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflow/create; docs_update_test.go::TestDocs_UpdateWorkflow/create; docs_media_test.go::TestDocs_MediaWorkflow/create-document | `--title --markdown` | |
| ✓ | docs +fetch | shortcut | docs_create_fetch_test.go::TestDocs_CreateAndFetchWorkflow/fetch; docs_update_test.go::TestDocs_UpdateWorkflow/verify | `--doc` | |
| ✓ | docs +media-download | shortcut | docs_media_test.go::TestDocs_MediaWorkflow/media-download | `--token --output` | |
| ✓ | docs +media-insert | shortcut | docs_media_test.go::TestDocs_MediaWorkflow/media-insert | `--doc --file --type` | |
| ✕ | docs +search | shortcut |  | none | current workflow uses `--as user`; not deterministic in bot-only CI |
| ✓ | docs +update | shortcut | docs_update_test.go::TestDocs_UpdateWorkflow/update-title-and-content | `--doc --mode --markdown --new-title` | |
| ✕ | docs +whiteboard-update | shortcut |  | none | current testcase is skipped; stdin DSL input is not supported by harness |

