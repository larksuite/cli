# Sheets CLI E2E Coverage

## Metrics

- Denominator: 15 leaf commands
- Covered: 15
- Coverage: 100%

## Summary

- TestSheets_CRUDE2EWorkflow: proves the shortcut CRUD path with `sheets +create`, `sheets +info`, `sheets +write`, `sheets +read`, `sheets +append`, `sheets +find`, and `sheets +export`; key proof points assert returned spreadsheet token, sheet id, read-back values, append persistence, find results, and export task success.
- TestSheets_SpreadsheetsResource: proves direct `sheets spreadsheets {create,get,patch}`; key proof points include a `spreadsheets get` read-after-create and a second `spreadsheets get` after `spreadsheets patch` to verify the new title.
- TestSheets_FilterWorkflow: proves `sheets spreadsheet.sheet.filters {create,get,update,delete}` after seeding sheet data with `sheets +create`, `sheets +info`, and `sheets +write`.
- TestSheets_FindWorkflow: proves `sheets spreadsheet.sheets find` after creating a spreadsheet and writing searchable values.
- Current scope note: the CLI currently exposes only 15 public sheets leaf commands, and all 15 are exercised with assertions in this suite.

## Command Table

| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | sheets +append | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/append values with +append | `--spreadsheet-token --sheet-id --values` | |
| ✓ | sheets +create | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/create spreadsheet with +create; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create spreadsheet with initial data; sheets_filter_workflow_test.go::TestSheets_FindWorkflow/create spreadsheet for find test | `--title` | |
| ✓ | sheets +export | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/export spreadsheet with +export | `--spreadsheet-token --type --output-dir` | |
| ✓ | sheets +find | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/find values with +find | `--spreadsheet-token --sheet-id --query` | |
| ✓ | sheets +info | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/get spreadsheet info with +info; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get sheet info; sheets_filter_workflow_test.go::TestSheets_FindWorkflow/get sheet info | `--spreadsheet-token` | |
| ✓ | sheets +read | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/read values with +read | `--spreadsheet-token --sheet-id --range` | |
| ✓ | sheets +write | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/write values with +write; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/write test data for filtering; sheets_filter_workflow_test.go::TestSheets_FindWorkflow/write searchable data | `--spreadsheet-token --sheet-id --values` | |
| ✓ | sheets spreadsheet.sheet.filters create | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create filter with spreadsheet.sheet.filters create | params with `spreadsheet_token` and `sheet_id`; request body with range and condition | |
| ✓ | sheets spreadsheet.sheet.filters delete | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/delete filter with spreadsheet.sheet.filters delete | params with `spreadsheet_token` and `sheet_id` | |
| ✓ | sheets spreadsheet.sheet.filters get | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get filter with spreadsheet.sheet.filters get | params with `spreadsheet_token` and `sheet_id` | |
| ✓ | sheets spreadsheet.sheet.filters update | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/update filter with spreadsheet.sheet.filters update | params with `spreadsheet_token` and `sheet_id`; request body with updated condition | |
| ✓ | sheets spreadsheet.sheets find | api | sheets_filter_workflow_test.go::TestSheets_FindWorkflow/find with spreadsheet.sheets find | params with `spreadsheet_token`; request body with range and search query | |
| ✓ | sheets spreadsheets create | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/create spreadsheet with spreadsheets create | request body with `title` | |
| ✓ | sheets spreadsheets get | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/get spreadsheet with spreadsheets get | params with `spreadsheet_token` | |
| ✓ | sheets spreadsheets patch | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/patch spreadsheet with spreadsheets patch | params with `spreadsheet_token`; request body with updated `title` | |
