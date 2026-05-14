# Drive CLI E2E Coverage

## Metrics
- Denominator: 29 leaf commands
- Covered: 7
- Coverage: 24.1%

## Summary
- TestDrive_FilesCreateFolderWorkflow: proves `drive files create_folder` in `create_folder as bot`; helper asserts the returned folder token and registers best-effort cleanup via `drive files delete`.
- TestDrive_StatusWorkflow: proves `drive +status` against a real Drive folder. Seeds the remote side via `drive +upload` (`unchanged.txt`, `modified.txt`, `remote-only.txt`), seeds local files with the matching/diverging contents, and asserts every output bucket (`unchanged`, `modified`, `new_local`, `new_remote`) holds exactly the expected `rel_path` and `file_token`. Cleans up uploaded files and the parent folder via best-effort cleanup hooks.
- TestDrive_UploadWorkflow: proves `drive +upload` against the real backend in both create and overwrite modes. First uploads a fresh file into a temporary Drive folder, then re-uploads new bytes with `--file-token` against the returned token, asserts the overwrite keeps the token stable, and finally downloads the file to confirm the remote content changed.
- TestDrive_DuplicateRemoteWorkflow: proves the duplicate-remote workflows against the real backend. One subtest uploads two same-name files into the same Drive folder and asserts `drive +status` and default `drive +pull` both fail with `duplicate_remote_path`, while `drive +pull --on-duplicate-remote=rename` succeeds, downloads both files, and writes a hashed renamed sibling locally. The other subtest uploads duplicate remote files, runs `drive +push --on-duplicate-remote=newest --if-exists=overwrite --delete-remote --yes`, and then re-runs `drive +status` to prove the mirror converged to a single unchanged `dup.txt`.
- TestDrive_ApplyPermissionDryRun / TestDrive_ApplyPermissionDryRunRejectsFullAccess: dry-run coverage for `drive +apply-permission`; asserts URL→type inference for docx/sheet/slides, explicit `--type` overriding URL inference when both a recognized URL and `--type` are supplied, bare-token + explicit `--type` path, request method/URL/type-query/perm/remark body shape, optional `remark` omission when unset, and client-side rejection of `--perm full_access`. Runs without hitting the live API.
- TestDriveExportDryRun_FileNameMetadata: dry-run coverage for `drive +export`; asserts export task request shape and local `--file-name` / `--output-dir` metadata without calling live APIs.
- TestDrive_PullDryRun / TestDrive_PullDryRunAcceptsDuplicateRemoteStrategies: dry-run coverage for `drive +pull`; asserts the list-files request shape, Validate-stage safety guards, and acceptance of `--on-duplicate-remote=rename|newest|oldest` by the real CLI binary.
- TestDrive_PushDryRun / TestDrive_PushDryRunAcceptsDuplicateRemoteStrategies: dry-run coverage for `drive +push`; asserts the list-files request shape, Validate-stage safety guards, conditional delete preflight, and acceptance of `--on-duplicate-remote=newest|oldest` by the real CLI binary.
- Cleanup note: `drive files delete` is only exercised in cleanup and is intentionally left uncovered.
- Blocked area: live export, comment, permission, subscription, and reply flows still need deterministic remote fixtures and filesystem setup.
- Dry-run note: `drive_upload_dryrun_test.go::TestDriveUploadDryRun_WikiTarget` and `TestDriveUploadDryRun_WithFileToken` cover the wiki-target and overwrite request shapes for `drive +upload`; live upload/status/duplicate workflows also use real `+upload` against the backend.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | drive +add-comment | shortcut |  | none | no comment workflow yet |
| ✓ | drive +apply-permission | shortcut | drive_apply_permission_dryrun_test.go::TestDrive_ApplyPermissionDryRun | `--token` URL vs bare; `--type` (enum) with URL inference; `--perm view\|edit`; `--remark` optional | dry-run only; no live-apply E2E because a real request pushes a card to the owner |
| ✕ | drive +delete | shortcut |  | none | no primary delete workflow yet |
| ✕ | drive +download | shortcut |  | none | no file fixture workflow yet |
| ✓ | drive +export | shortcut | drive_export_dryrun_test.go::TestDriveExportDryRun_FileNameMetadata | `--token`; `--doc-type`; `--file-extension`; `--file-name`; `--output-dir` | dry-run only; no live export workflow yet |
| ✕ | drive +export-download | shortcut |  | none | no export-download workflow yet |
| ✕ | drive +import | shortcut |  | none | no import workflow yet |
| ✕ | drive +move | shortcut |  | none | no move workflow yet |
| ✓ | drive +pull | shortcut | drive_pull_dryrun_test.go::TestDrive_PullDryRun + drive_duplicate_sync_workflow_test.go::TestDrive_DuplicateRemoteWorkflow | `--local-dir`; `--folder-token`; `--on-duplicate-remote=rename\|newest\|oldest`; `--delete-local --yes` guard | dry-run locks flag/validate shape; live workflow proves duplicate fail-fast and rename recovery |
| ✓ | drive +push | shortcut | drive_push_dryrun_test.go::TestDrive_PushDryRun + drive_duplicate_sync_workflow_test.go::TestDrive_DuplicateRemoteWorkflow | `--local-dir`; `--folder-token`; `--if-exists`; `--on-duplicate-remote=newest\|oldest`; `--delete-remote --yes` | dry-run locks flag/validate shape; live workflow proves overwrite + duplicate cleanup converges status |
| ✓ | drive +status | shortcut | drive_status_workflow_test.go::TestDrive_StatusWorkflow + drive_status_dryrun_test.go::TestDrive_StatusDryRun + drive_duplicate_sync_workflow_test.go::TestDrive_DuplicateRemoteWorkflow | `--local-dir`; `--folder-token`; bucketed `new_local` / `new_remote` / `modified` / `unchanged` outputs | dry-run pins request shape; live workflows cover both normal hashing buckets and duplicate-remote failure |
| ✕ | drive +task_result | shortcut |  | none | no async task-result workflow yet |
| ✓ | drive +upload | shortcut | drive_upload_dryrun_test.go::TestDriveUploadDryRun_WikiTarget + drive_upload_dryrun_test.go::TestDriveUploadDryRun_WithFileToken + drive_upload_workflow_test.go::TestDrive_UploadWorkflow + drive_status_workflow_test.go::TestDrive_StatusWorkflow + drive_duplicate_sync_workflow_test.go::TestDrive_DuplicateRemoteWorkflow | `--wiki-token`; `--file-token`; `parent_type=wiki`; `parent_node`; named uploads into Drive folders; in-place overwrite uploads | dry-run covers wiki-target and overwrite request shapes; live workflows assert returned file tokens, token-stable overwrite behavior, and that uploaded fixtures are consumable by downstream commands |
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
