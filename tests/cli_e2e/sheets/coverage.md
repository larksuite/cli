# Sheets CLI E2E Coverage

## Metrics
- Denominator: 30 leaf commands
- Covered: 23
- Coverage: 76.7%
- Scope note: this table tracks the commands these E2E tests touch plus the gaps
  carried over from the pre-refactor surface. It is not a census of the whole
  sheets shortcut surface, which is far larger; re-auditing it against the
  current command list is a separate follow-up.

## Summary
- TestSheets_CRUDE2EWorkflow: proves `+workbook-create`, `+workbook-info`, `+cells-set`, `+cells-get`, `+cells-search`, and `+workbook-export`; key `t.Run(...)` proof points are `create spreadsheet with +workbook-create as bot`, `read data with +cells-get as bot`, `find cells with +cells-search as bot`, and `export spreadsheet with +workbook-export as bot`. `+cells-search` asserts both halves — one match at `A2` for a term on the sheet, zero for one that is not — so a search that silently matched everything would still fail.
- TestSheets_CreateWorkflowAsUser: proves the UAT path for `sheets +workbook-create` and `sheets +workbook-info` through `create spreadsheet with +workbook-create as user` and `get spreadsheet info with +workbook-info as user`.
- TestSheets_SpreadsheetsResource: proves direct `spreadsheets create`, `spreadsheets get`, and `spreadsheets patch`.
- TestSheets_FilterWorkflow: proves `spreadsheet.sheet.filters create`, `get`, `update`, and `delete`, with supporting sheet setup through `+workbook-create`, `+workbook-info`, and `+cells-set`.
- TestSheets_SheetShortcutsDryRun: proves request shapes for `+sheet-create`, `+sheet-copy`, `+sheet-delete`, `+sheet-rename`, and `+sheet-move` without hitting live APIs — all five reach the backend through `modify_workbook_structure`, so the pinned distinction is the `operation` and the fields packed beside it.
- TestSheets_SheetShortcutsWorkflow: proves live `+sheet-create`, `+sheet-copy`, `+sheet-rename`, `+sheet-hide`, `+dim-freeze`, and `+sheet-delete` flows against a real spreadsheet, verified by reading the workbook back through `+workbook-info` rather than by matching a field in each command's own response. Rename and hide answer with nothing but a revision counter, so the listing is the only place their effect can be observed (`sheet_name`, `is_hidden`); `+dim-freeze` reports `frozen_rows` / `frozen_columns` in its own result.
- TestSheets_ImageUploadDryRunParentType: dry-run coverage for the drive `parent_type` an image upload carries, across every surface a local file enters through — `+cells-set-image` and `+float-image-create`. A native spreadsheet must upload as `sheet_image` and one backed by an imported office file as `office_sheet_file`; a `/wiki/` ref must stay native, because a preview cannot resolve the node and so must not read the split out of the node_token. The negative half matters most: the backend does not validate `parent_node` against `parent_type`, so a wrong value uploads successfully and only surfaces later as an image that will not render.
- TestSheets_ImageUploadDryRunChunked: dry-run coverage for the 20 MB branch in `+cells-set-image`. Under the ceiling the preview shows one `upload_all`; one byte past it, the `upload_prepare` / `upload_part` / `upload_finish` trio Execute actually sends. Before that dispatch existed, an oversized image failed on `upload_all` with a bare `1061002` naming neither the size nor the limit.
- Cleanup note: workflow-created spreadsheets are cleaned up via `drive +delete --type sheet`; those cleanup-only executions are not counted as command coverage because no testcase asserts delete behavior as the primary proof surface.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | sheets +cells-get | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/read data with +cells-get as bot | `--spreadsheet-token`; `--sheet-id`; `--range` | values are collected out of the decoded payload, not matched against a fixed path: `get_cell_ranges`' response nesting is the backend's |
| ✕ | sheets +cells-merge | shortcut |  | none | no merge workflow yet |
| ✕ | sheets +cells-replace | shortcut |  | none | no replace workflow yet |
| ✓ | sheets +cells-search | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/find cells with +cells-search as bot | `--spreadsheet-token`; `--sheet-id`; `--find`; `--range` | asserts the hit (`total_matches` 1, address `A2`) and the miss (`total_matches` 0) |
| ✓ | sheets +cells-set | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/write data with +cells-set as bot; sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/append a row with +cells-set as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/write test data for filtering as bot | `--spreadsheet-token`; `--sheet-id`; `--range`; `--cells` | the append subtest writes past the existing block, which is the auto-expand path the pre-refactor `+append` relied on |
| ✕ | sheets +cells-set-image | shortcut |  | dry-run only | live image workflow still missing; parent_type shape covered by sheets_image_upload_dryrun_test.go |
| ✓ | sheets +cells-set-style | shortcut | sheets_call_compat_workflow_test.go::TestSheets_CallCompatWorkflow/style it with an openpyxl border weight | `--spreadsheet-token`; `--range`; `--border-styles` | |
| ✕ | sheets +cells-unmerge | shortcut |  | none | no merge workflow yet |
| ✓ | sheets +dim-delete | shortcut | sheets_dim_workflow_test.go::TestSheets_DimShortcutsWorkflow | `--spreadsheet-token`; `--sheet-id`; `--ranges` | |
| ✓ | sheets +dim-freeze | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/freeze rows and columns with +dim-freeze as bot | `--spreadsheet-token`; `--sheet-id`; `--rows`; `--cols` | asserts the reported `frozen_rows` / `frozen_columns` |
| ✓ | sheets +dim-insert | shortcut | sheets_dim_workflow_test.go::TestSheets_DimShortcutsWorkflow | `--spreadsheet-token`; `--sheet-id`; `--position`; `--count` | dry-run inherit-style shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✕ | sheets +dim-move | shortcut |  | none | no dimension move workflow yet |
| ✓ | sheets +sheet-copy | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/copy sheet with +sheet-copy as bot | `--spreadsheet-token`; `--sheet-id`; optional `--title`; optional `--index` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +sheet-create | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/create sheet with +sheet-create as bot; sheets_sheet_list_workflow_test.go::TestSheets_SheetListWorkflow | `--spreadsheet-token`; `--title`; optional `--index` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +sheet-delete | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/delete sheet with +sheet-delete as bot | `--spreadsheet-token`; `--sheet-id`; `--yes` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +sheet-hide | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/hide sheet with +sheet-hide as bot | `--spreadsheet-token`; `--sheet-id` | the command answers with a revision only, so `is_hidden` is read back from +workbook-info |
| ✓ | sheets +sheet-list | shortcut | sheets_sheet_list_workflow_test.go::TestSheets_SheetListWorkflow | `--spreadsheet-token` | pinned to forward +workbook-info's sheets entries verbatim |
| ✕ | sheets +sheet-move | shortcut |  | dry-run only | no live reordering workflow yet; request shape covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +sheet-rename | shortcut | sheets_sheet_shortcuts_workflow_test.go::TestSheets_SheetShortcutsWorkflow/rename sheet with +sheet-rename as bot | `--spreadsheet-token`; `--sheet-id`; `--title` | dry-run shape also covered by sheets_sheet_shortcuts_dryrun_test.go |
| ✓ | sheets +workbook-create | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/create spreadsheet with +workbook-create as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create spreadsheet with initial data as bot; sheets_create_workflow_test.go::TestSheets_CreateWorkflowAsUser/create spreadsheet with +workbook-create as user | `--title`; `--folder-token` | token read from `data.spreadsheet.spreadsheet_token` |
| ✓ | sheets +workbook-export | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/export spreadsheet with +workbook-export as bot | `--spreadsheet-token`; `--file-extension`; `--output-path` | |
| ✓ | sheets +workbook-info | shortcut | sheets_crud_workflow_test.go::TestSheets_CRUDE2EWorkflow/get spreadsheet info with +workbook-info as bot; sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get sheet info as bot; sheets_create_workflow_test.go::TestSheets_CreateWorkflowAsUser/get spreadsheet info with +workbook-info as user | `--spreadsheet-token` | returns the workbook structure only: sub-sheets under `data.sheets`, no spreadsheet token echo |
| ✓ | sheets spreadsheet.sheet.filters create | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/create filter with spreadsheet.sheet.filters create as bot | `spreadsheet_token`; `sheet_id` in `--params`; filter JSON in `--data` | |
| ✓ | sheets spreadsheet.sheet.filters delete | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/delete filter with spreadsheet.sheet.filters delete as bot | `spreadsheet_token`; `sheet_id` in `--params` | |
| ✓ | sheets spreadsheet.sheet.filters get | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/get filter with spreadsheet.sheet.filters get as bot | `spreadsheet_token`; `sheet_id` in `--params` | |
| ✓ | sheets spreadsheet.sheet.filters update | api | sheets_filter_workflow_test.go::TestSheets_FilterWorkflow/update filter with spreadsheet.sheet.filters update as bot | `spreadsheet_token`; `sheet_id` in `--params`; filter JSON in `--data` | |
| ✕ | sheets spreadsheet.sheets find | api |  | none | no direct API workflow yet |
| ✓ | sheets spreadsheets create | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/create spreadsheet with spreadsheets create as bot | `title` in `--data` | |
| ✓ | sheets spreadsheets get | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/get spreadsheet with spreadsheets get as bot | `spreadsheet_token` in `--params` | |
| ✓ | sheets spreadsheets patch | api | sheets_crud_workflow_test.go::TestSheets_SpreadsheetsResource/patch spreadsheet with spreadsheets patch as bot | `spreadsheet_token` in `--params`; title patch in `--data` | |
