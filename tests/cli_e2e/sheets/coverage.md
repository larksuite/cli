# Sheets CLI E2E Coverage

## Metrics
- Denominator: 30 leaf commands
- Covered: 18
- Coverage: 60.0%

## Summary
- TestSheets_CRUDE2EWorkflow: proves `+create`, `+info`, `+write`, `+read`, `+append`, `+find`, and `+export`; key `t.Run(...)` proof points are `create spreadsheet with +create as bot`, `read data with +read as bot`, `find cells with +find as bot`, and `export spreadsheet with +export as bot`.
- TestSheets_CreateWorkflowAsUser: proves the UAT path for `sheets +create` and `sheets +info` through `create spreadsheet with +create as user` and `get spreadsheet info with +info as user`.
- TestSheets_SpreadsheetsResource: proves direct `spreadsheets create`, `spreadsheets get`, and `spreadsheets patch`.
- TestSheets_FilterWorkflow: proves `spreadsheet.sheet.filters create`, `get`, `update`, and `delete`, with supporting sheet setup through `+create`, `+info`, and `+write`.
- TestSheets_SheetShortcutsDryRun: proves request shapes for `+create-sheet`, `+copy-sheet`, `+delete-sheet`, and `+update-sheet` without hitting live APIs.
- TestSheets_SheetShortcutsWorkflow: proves live `+create-sheet`, `+copy-sheet`, `+update-sheet`, and `+delete-sheet` flows against a real spreadsheet, with verification through `+info`.
- Cleanup note: workflow-created spreadsheets are cleaned up via `drive +delete --type sheet`; those cleanup-only executions are not counted as command coverage because no testcase asserts delete behavior as the primary proof surface.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | sheets +add-dimension | shortcut |  | none | no dimension workflow yet |
| ✓ | sheets +append | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/append rows with +append as bot | `--spreadsheet-token`; `--sheet-id`; `--range`; `--values` | |
| ✕ | sheets +batch-set-style | shortcut |  | none | no style workflow yet |
| ✓ | sheets +create | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/create spreadsheet with +create as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create spreadsheet with initial data as bot; sheets_create_workflow_test.go::TestSheets_CreateWorkflowAsUser/create spreadsheet with +create as user | `--title` | |
| ✓ | sheets +create-sheet | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/create sheet with +create-sheet as bot | `--spreadsheet-token`; optional `--title`; optional `--index` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +copy-sheet | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/copy sheet with +copy-sheet as bot | `--spreadsheet-token`; `--sheet-id`; optional `--title`; optional `--index` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✕ | sheets +delete-dimension | shortcut |  | none | no dimension workflow yet |
| ✓ | sheets +delete-sheet | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/delete sheet with +delete-sheet as bot | `--spreadsheet-token`; `--sheet-id` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +export | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/export spreadsheet with +export as bot | `--spreadsheet-token`; `--file-extension` | |
| ✓ | sheets +find | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/find cells with +find as bot | `--spreadsheet-token`; `--sheet-id`; `--find`; `--range` | |
| ✓ | sheets +info | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/get spreadsheet info with +info as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get sheet info as bot; sheets_create_workflow_test.go::TestSheets_CreateWorkflowAsUser/get spreadsheet info with +info as user | `--spreadsheet-token` | |
| ✕ | sheets +insert-dimension | shortcut |  | none | no dimension workflow yet |
| ✕ | sheets +merge-cells | shortcut |  | none | no merge workflow yet |
| ✕ | sheets +move-dimension | shortcut |  | none | no dimension workflow yet |
| ✓ | sheets +read | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/read data with +read as bot | `--spreadsheet-token`; `--sheet-id`; `--range` | |
| ✕ | sheets +replace | shortcut |  | none | no replace workflow yet |
| ✕ | sheets +set-style | shortcut |  | none | no style workflow yet |
| ✕ | sheets +unmerge-cells | shortcut |  | none | no merge workflow yet |
| ✕ | sheets +update-dimension | shortcut |  | none | no dimension workflow yet |
| ✓ | sheets +update-sheet | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/update sheet with +update-sheet as bot | `--spreadsheet-token`; `--sheet-id`; scalar update flags; optional protect fields | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +write | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/write data with +write as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/write test data for filtering as bot | `--spreadsheet-token`; `--sheet-id`; `--range`; `--values` | |
| ✕ | sheets +write-image | shortcut |  | none | no image workflow yet |
| ✓ | sheets spreadsheet.sheet.filters create | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create filter with spreadsheet.sheet.filters create as bot | `spreadsheet_token`; `sheet_id` in `--params`; filter JSON in `--data` | |
| ✓ | sheets spreadsheet.sheet.filters delete | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/delete filter with spreadsheet.sheet.filters delete as bot | `spreadsheet_token`; `sheet_id` in `--params` | |
| ✓ | sheets spreadsheet.sheet.filters get | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get filter with spreadsheet.sheet.filters get as bot | `spreadsheet_token`; `sheet_id` in `--params` | |
| ✓ | sheets spreadsheet.sheet.filters update | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/update filter with spreadsheet.sheet.filters update as bot | `spreadsheet_token`; `sheet_id` in `--params`; filter JSON in `--data` | |
| ✕ | sheets spreadsheet.sheets find | api |  | none | no direct API workflow yet |
| ✓ | sheets spreadsheets create | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/create spreadsheet with spreadsheets create as bot | `title` in `--data` | |
| ✓ | sheets spreadsheets get | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/get spreadsheet with spreadsheets get as bot | `spreadsheet_token` in `--params` | |
| ✓ | sheets spreadsheets patch | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/patch spreadsheet with spreadsheets patch as bot | `spreadsheet_token` in `--params`; title patch in `--data` | |
