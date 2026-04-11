# Drive CLI E2E Coverage

## Metrics

- Denominator: 28 leaf commands
- Covered: 26
- Coverage: 92.9%

## Summary

- TestDrive_UploadDownloadWorkflow: proves `drive +upload` and `drive +download`; key proof points are `upload` asserting `file_token` and `download` asserting the downloaded file content matches the uploaded content.
- TestDrive_ExportWorkflow, TestDrive_ExportDownloadWorkflow, and TestDrive_ImportExportWorkflow: prove `drive +import`, `drive +export`, `drive +export-download`, and `drive +task_result`; key proof points cover import polling, export polling, exported `file_token`, and local file existence.
- TestDrive_MoveWorkflow: proves `drive +move` and the move polling path through `drive +task_result`.
- Comment workflows prove `drive +add-comment`, `drive file.comments {create_v2,list,patch,batch_query}`, and `drive file.comment.replys {create,update,list,delete}` against imported docs.
- File/resource workflows prove `drive files {list,create_folder,copy}`, `drive metas batch_query`, `drive file.statistics get`, `drive file.view_records list`, and `drive permission.members auth`.
- Subscription workflow proves `drive user subscription`, `drive user subscription_status`, and `drive user remove_subscription`.
- Blocked area: `drive permission.members create` and `drive permission.members transfer_owner` require stable real-user `open_id` fixtures and are skipped.

## Command Table

| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | drive +add-comment | shortcut | drive_add_comment_workflow_test.go::TestDrive_AddCommentWorkflow/add full-document comment; drive_permission_members_workflow_test.go::TestDrive_FileCommentReplysWorkflow/create local comment; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysUpdateWorkflow/create local comment; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysDeleteWorkflow/create local comment | `--doc`; `--full-comment` or `--selection-with-ellipsis`; `--content` | |
| ✓ | drive +download | shortcut | drive_upload_download_workflow_test.go::TestDrive_UploadDownloadWorkflow/download | `--file-token --output` | |
| ✓ | drive +export | shortcut | drive_export_download_workflow_test.go::TestDrive_ExportWorkflow/export - save to local file; drive_export_download_workflow_test.go::TestDrive_ExportDownloadWorkflow/export as PDF to get file_token; drive_import_export_workflow_test.go::TestDrive_ImportExportWorkflow/export | `--token --doc-type --file-extension`; optional `--output-dir --overwrite` | |
| ✓ | drive +export-download | shortcut | drive_export_download_workflow_test.go::TestDrive_ExportDownloadWorkflow/download exported file with export-download | `--file-token --output-dir --overwrite` | |
| ✓ | drive +import | shortcut | drive/helpers_test.go::importTestDoc | `--file --type docx` | Covered through helper assertions on returned `ticket`/`token`. |
| ✓ | drive +move | shortcut | drive_move_workflow_test.go::TestDrive_MoveWorkflow/move | `--file-token --type` | |
| ✓ | drive +task_result | shortcut | drive/helpers_test.go::importTestDoc; drive_export_download_workflow_test.go::TestDrive_ExportWorkflow/export - save to local file; drive_import_export_workflow_test.go::TestDrive_ImportExportWorkflow/export; drive_move_workflow_test.go::TestDrive_MoveWorkflow/move | `--ticket --scenario import/export`; `--task-id --scenario task_check` | |
| ✓ | drive +upload | shortcut | drive_upload_download_workflow_test.go::TestDrive_UploadDownloadWorkflow/upload; drive_files_copy_workflow_test.go::TestDrive_FilesCopyWorkflow/upload file; drive/helpers_test.go::uploadTestFile | `--file` with a relative local path | |
| ✓ | drive file.comment.replys create | api | drive_permission_members_workflow_test.go::TestDrive_FileCommentReplysWorkflow/create reply; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysUpdateWorkflow/add reply; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysDeleteWorkflow/add reply | params with `file_token`, `comment_id`, `file_type`; request body with reply elements | |
| ✓ | drive file.comment.replys delete | api | drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysDeleteWorkflow/delete reply | params with `file_token`, `comment_id`, `reply_id`, `file_type` | |
| ✓ | drive file.comment.replys list | api | drive_permission_members_workflow_test.go::TestDrive_FileCommentReplysWorkflow/list replies; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysUpdateWorkflow/list replies; drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysDeleteWorkflow/list replies after delete | params with `file_token`, `comment_id`, `file_type` | |
| ✓ | drive file.comment.replys update | api | drive_file_comment_replys_workflow_test.go::TestDrive_FileCommentReplysUpdateWorkflow/update reply | params with `file_token`, `comment_id`, `reply_id`, `file_type`; request body with updated reply elements | |
| ✓ | drive file.comments batch_query | api | drive_permission_members_workflow_test.go::TestDrive_FileCommentsBatchQueryWorkflow/batch query comments | request body with `file_token`, `file_type`, and `comment_ids` | |
| ✓ | drive file.comments create_v2 | api | drive_file_comments_workflow_test.go::TestDrive_FileCommentsWorkflow/create_v2 - add comment; drive_permission_members_workflow_test.go::TestDrive_FileCommentsBatchQueryWorkflow/create comment | params with `file_token`; request body with `file_type` and reply elements | |
| ✓ | drive file.comments list | api | drive_file_comments_workflow_test.go::TestDrive_FileCommentsWorkflow/list - get comments | params with `file_token`, `file_type`, and pagination | |
| ✓ | drive file.comments patch | api | drive_file_comments_workflow_test.go::TestDrive_FileCommentsWorkflow/patch - resolve comment | params with `file_token`, `comment_id`, `file_type`; request body with updated state | |
| ✓ | drive file.statistics get | api | drive_file_statistics_workflow_test.go::TestDrive_FileStatisticsWorkflow/get - get file statistics | params with `file_token` and `file_type` | |
| ✓ | drive file.view_records list | api | drive_file_statistics_workflow_test.go::TestDrive_FileViewRecordsWorkflow/list - get file view records | params with `file_token`, `file_type`, and `page_size` | |
| ✓ | drive files copy | api | drive_files_copy_workflow_test.go::TestDrive_FilesCopyWorkflow/copy file | request body with source token, target folder, and new name | |
| ✓ | drive files create_folder | api | drive_files_workflow_test.go::TestDrive_FilesCreateFolderWorkflow/create_folder | request body with `name` and optional parent token | |
| ✓ | drive files list | api | drive_files_workflow_test.go::TestDrive_FilesListWorkflow/list - get root folder files; drive_files_copy_workflow_test.go::TestDrive_FilesCopyWorkflow/get root folder token; drive_files_copy_workflow_test.go::TestDrive_FilesCopyWorkflow/list copied files | optional listing params | |
| ✓ | drive metas batch_query | api | drive_metas_workflow_test.go::TestDrive_MetasBatchQueryWorkflow/batch_query - get document metadata | request body with `request_docs` | |
| ✓ | drive permission.members auth | api | drive_permission_user_workflow_test.go::TestDrive_PermissionMembersAuthWorkflow/check view permission; drive_permission_user_workflow_test.go::TestDrive_PermissionMembersAuthWorkflow/check edit permission | params with `token`, `type`, and `action` | |
| ✕ | drive permission.members create | api |  | none | skipped in `drive_permission_members_workflow_test.go`; requires a real target user |
| ✕ | drive permission.members transfer_owner | api |  | none | skipped in `drive_permission_user_workflow_test.go`; requires a real target user |
| ✓ | drive user remove_subscription | api | drive_permission_user_workflow_test.go::TestDrive_UserSubscriptionWorkflow/remove subscription | params with `event_type` | |
| ✓ | drive user subscription | api | drive_permission_user_workflow_test.go::TestDrive_UserSubscriptionWorkflow/subscribe to comment events | request body with `event_type` | |
| ✓ | drive user subscription_status | api | drive_permission_user_workflow_test.go::TestDrive_UserSubscriptionWorkflow/check subscription status | params with `event_type` | |
