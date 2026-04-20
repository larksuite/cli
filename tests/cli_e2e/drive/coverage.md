# Drive CLI E2E Coverage

## Metrics
- Denominator: 28 leaf commands
- Covered: 1
- Coverage: 3.6%

## Summary
- TestDrive_FilesCreateFolderWorkflow: proves `drive files create_folder` in `create_folder as bot`; helper asserts the returned folder token and registers best-effort cleanup via `drive files delete`.
- Cleanup note: `drive files delete` is only exercised in cleanup and is intentionally left uncovered.
- Blocked area: upload, export, comment, permission, subscription, and reply flows still need deterministic remote fixtures and filesystem setup.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | drive +add-comment | shortcut |  | none | no comment workflow yet |
| ✕ | drive +delete | shortcut |  | none | no primary delete workflow yet |
| ✕ | drive +download | shortcut |  | none | no file fixture workflow yet |
| ✕ | drive +export | shortcut |  | none | no export workflow yet |
| ✕ | drive +export-download | shortcut |  | none | no export-download workflow yet |
| ✕ | drive +import | shortcut |  | none | no import workflow yet |
| ✕ | drive +move | shortcut |  | none | no move workflow yet |
| ✕ | drive +task_result | shortcut |  | none | no async task-result workflow yet |
| ✕ | drive +upload | shortcut |  | none | no upload workflow yet |
| ✕ | drive file.comment.replys create | api |  | none | no reply workflow yet |
| ✕ | drive file.comment.replys delete | api |  | none | no reply workflow yet |
| ✕ | drive file.comment.replys list | api |  | none | no reply workflow yet |
| ✕ | drive file.comment.replys update | api |  | none | no reply workflow yet |
| ✕ | drive file.comments create_v2 | api |  | none | no file comment workflow yet |
| ✕ | drive file.comments list | api |  | none | no file comment workflow yet |
| ✕ | drive file.comments patch | api |  | none | no file comment workflow yet |
| ✕ | drive file.statistics get | api |  | none | no statistics workflow yet |
| ✕ | drive file.view_records list | api |  | none | no view-record workflow yet |
| ✕ | drive files copy | api |  | none | no file copy workflow yet |
| ✓ | drive files create_folder | api | drive_files_workflow_test.go::TestDrive_FilesCreateFolderWorkflow/create_folder as bot | `name`; empty `folder_token` in `--data` | |
| ✕ | drive files list | api |  | none | no list workflow yet |
| ✕ | drive metas batch_query | api |  | none | no metadata workflow yet |
| ✕ | drive permission.members auth | api |  | none | permission workflows not covered |
| ✕ | drive permission.members create | api |  | none | permission workflows not covered |
| ✕ | drive permission.members transfer_owner | api |  | none | permission workflows not covered |
| ✕ | drive user remove_subscription | api |  | none | subscription workflows not covered |
| ✕ | drive user subscription | api |  | none | subscription workflows not covered |
| ✕ | drive user subscription_status | api |  | none | subscription workflows not covered |
