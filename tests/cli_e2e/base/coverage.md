# Base CLI E2E Coverage

## Metrics
- Denominator: 87 leaf commands
- Covered: 29
- Coverage: 33.3%

## Summary
- TestBase_BasicWorkflow: proves `+base-create`, `+base-get`, `+table-create`, `+table-get`, and `+table-list`; key `t.Run(...)` proof points are `get base as bot`, `get table as bot`, and `list tables and find created table as bot`.
- TestBaseBlockDryRun: proves the five `+base-block-*` shortcuts request shapes without touching live data.
- TestBaseFieldCreateDryRunArrayCompat: proves `+field-create` dry-run request shape for the internal JSON-array compatibility path.
- TestBaseViewCreateDryRun: proves `+view-create` request shape and empty-batch validation.
- TestBaseFormQuestionsCreateDryRun: proves `+form-questions-create` preserves its POST body and renders the existing-question guard in command help.
- TestBaseFormDetailDryRun / TestBaseFormSubmitDryRun: prove shared-form detail and submission request shapes.
- TestBaseDashboardBlockGetDataDryRun: proves dashboard block data request shapes and identifier handling.
- TestBaseRecordBatchUpdatePerRecordDryRun: proves `+record-batch-update` preserves the per-record `update_records` request shape.
- TestBaseRecordBatchUpdatePerRecordWorkflow: creates two records, updates different field types in one request, asserts the minimal response contract, reads both records back, verifies a missing record ID is not prevalidated, and cleans up the temporary Base.
- TestBase_RoleWorkflow: proves `+advperm-enable`, `+role-create`, `+role-list`, `+role-get`, and `+role-update`; key `t.Run(...)` proof points are `list as bot`, `get as bot`, and `update as bot`.
- TestBaseFormListDryRun_UsesBaseAndTableIdentifiers: proves `+form-list` dry-run request shape uses Base and table identifiers in the endpoint.
- TestBaseFormQuestionsCreateVisibleRuleDryRun / TestBaseFormQuestionsUpdateVisibleRuleDryRun: prove `+form-questions-create` / `+form-questions-update` dry-run request shape and that the optional `visible_rule` display condition is transcribed verbatim into the request body.
- Cleanup note: `+table-delete` and `+role-delete` only run in cleanup and are intentionally left uncovered.
- Blocked area: dashboard, field, most record operations, form, remaining view operations, and workflow operations still lack deterministic create/read/update workflows in this suite.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | base +advperm-disable | shortcut |  | none | no disable workflow yet |
| ✓ | base +advperm-enable | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow | `--base-token` | |
| ✕ | base +base-copy | shortcut |  | none | no copy workflow yet |
| ✓ | base +base-create | shortcut | base/helpers_test.go::createBaseWithRetry | `--name`; `--time-zone` | helper asserts created base token |
| ✓ | base +base-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get base as bot | `--base-token` | |
| ✓ | base +base-block-create | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/create | `--base-token`; `--type`; `--name`; `--parent-id`; dry-run only | request shape only |
| ✓ | base +base-block-delete | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/delete | `--base-token`; `--block-id`; dry-run only | request shape only |
| ✓ | base +base-block-list | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/list all,list folder | `--base-token`; optional `--parent-id`; optional `--type`; dry-run only | request shape only |
| ✓ | base +base-block-move | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/move root,move after | `--base-token`; `--block-id`; optional `--parent-id`; `--after-id`; dry-run only | request shape only |
| ✓ | base +base-block-rename | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/rename | `--base-token`; `--block-id`; `--name`; dry-run only | request shape only |
| ✕ | base +dashboard-arrange | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-create | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-delete | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-get | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-block-get-data | shortcut | base_dashboard_block_get_data_dryrun_test.go | `--base-token`; `--dashboard-id`; `--block-id`; dry-run only | request shape and identifier handling |
| ✕ | base +dashboard-block-list | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-update | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-create | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-delete | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-get | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-list | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-update | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +data-query | shortcut |  | none | no data-query assertions yet |
| ✓ | base +field-create | shortcut | base_field_dryrun_test.go::TestBaseFieldCreateDryRunArrayCompat | `--base-token`; `--table-id`; `--json`; dry-run only | request shape only |
| ✕ | base +field-delete | shortcut |  | none | field workflows not covered |
| ✕ | base +field-get | shortcut |  | none | field workflows not covered |
| ✕ | base +field-list | shortcut |  | none | field workflows not covered |
| ✕ | base +field-search-options | shortcut |  | none | field workflows not covered |
| ✕ | base +field-update | shortcut |  | none | field workflows not covered |
| ✕ | base +form-create | shortcut |  | none | form workflows not covered |
| ✕ | base +form-delete | shortcut |  | none | form workflows not covered |
| ✓ | base +form-detail | shortcut | base_form_detail_dryrun_test.go::TestBaseFormDetailDryRun | `--share-token`; dry-run only | shared-form request shape |
| ✕ | base +form-get | shortcut |  | none | form workflows not covered |
| ✓ | base +form-list | shortcut | base_form_detail_dryrun_test.go::TestBaseFormListDryRun_UsesBaseAndTableIdentifiers | `--base-token`; `--table-id`; dry-run only | request shape only |
| ✓ | base +form-questions-create | shortcut | TestBaseFormQuestionsCreateVisibleRuleDryRun; base_form_questions_create_dryrun_test.go | questions[].visible_rule; dry-run | request body, visible_rule passthrough, and help guard covered |
| ✕ | base +form-questions-delete | shortcut |  | none | form workflows not covered |
| ✕ | base +form-questions-list | shortcut |  | none | form workflows not covered |
| ✓ | base +form-questions-update | shortcut | TestBaseFormQuestionsUpdateVisibleRuleDryRun | questions[].visible_rule | dry-run: request shape + visible_rule body passthrough |
| ✓ | base +form-submit | shortcut | base_form_submit_dryrun_test.go::TestBaseFormSubmitDryRun | `--share-token`; `--json`; dry-run only | submission request shape |
| ✕ | base +form-update | shortcut |  | none | form workflows not covered |
| ✓ | base +record-batch-create | shortcut | base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--base-token`; `--table-id`; `--json.create_records` | seeds heterogeneous live workflow records |
| ✓ | base +record-batch-update | shortcut | base_record_batch_update_dryrun_test.go::TestBaseRecordBatchUpdatePerRecordDryRun; base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--base-token`; `--table-id`; `--json.update_records`; dry-run + live | heterogeneous select/number update with write-back verification |
| ✕ | base +record-delete | shortcut |  | none | record workflows not covered |
| ✓ | base +record-get | shortcut | base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--record-id`; repeated `--field-id`; `--format json` | reads back select and number values after batch update |
| ✕ | base +record-history-list | shortcut |  | none | record workflows not covered |
| ✕ | base +record-list | shortcut |  | none | record workflows not covered |
| ✕ | base +record-search | shortcut |  | none | record workflows not covered |
| ✕ | base +record-share-link-create | shortcut |  | none | record workflows not covered |
| ✓ | base +record-upload-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/upload | dry-run only | request shape only |
| ✓ | base +record-download-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/download | dry-run only | request shape only |
| ✓ | base +record-remove-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/remove | dry-run only | request shape only |
| ✕ | base +record-upsert | shortcut |  | none | record workflows not covered |
| ✓ | base +role-create | shortcut | base/helpers_test.go::createRole | `--base-token`; `--json` | helper asserts created role id |
| ✕ | base +role-delete | shortcut |  | none | cleanup only |
| ✓ | base +role-get | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/get as bot | `--base-token`; `--role-id` | |
| ✓ | base +role-list | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/list as bot | `--base-token` | |
| ✓ | base +role-update | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/update as bot | `--base-token`; `--role-id`; `--json` | |
| ✓ | base +table-create | shortcut | base/helpers_test.go::createTableWithRetry | `--base-token`; `--name`; optional `--fields`; optional `--view` | helper asserts table id |
| ✕ | base +table-delete | shortcut |  | none | cleanup only |
| ✓ | base +table-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get table as bot | `--base-token`; `--table-id` | |
| ✓ | base +table-list | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/list tables and find created table as bot | `--base-token` | |
| ✕ | base +table-update | shortcut |  | none | no rename workflow yet |
| ✕ | base +title-resolve | shortcut |  | none | resolver workflow not covered |
| ✕ | base +url-resolve | shortcut |  | none | resolver workflow not covered |
| ✓ | base +view-create | shortcut | base_view_create_dryrun_test.go::TestBaseViewCreateDryRun | `--base-token`; `--table-id`; `--json`; dry-run only | request shape and empty-batch validation |
| ✕ | base +view-delete | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-card | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-filter | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-group | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-sort | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-timebar | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-visible-fields | shortcut |  | none | view workflows not covered |
| ✕ | base +view-list | shortcut |  | none | view workflows not covered |
| ✕ | base +view-rename | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-card | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-filter | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-group | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-sort | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-timebar | shortcut |  | none | view workflows not covered |
| ✕ | base +view-set-visible-fields | shortcut |  | none | view workflows not covered |
| ✕ | base +workflow-create | shortcut |  | none | workflow CRUD not covered |
| ✕ | base +workflow-disable | shortcut |  | none | workflow CRUD not covered |
| ✕ | base +workflow-enable | shortcut |  | none | workflow CRUD not covered |
| ✕ | base +workflow-get | shortcut |  | none | workflow CRUD not covered |
| ✕ | base +workflow-list | shortcut |  | none | workflow CRUD not covered |
| ✕ | base +workflow-update | shortcut |  | none | workflow CRUD not covered |
