# Base CLI E2E Coverage

## Metrics
- Denominator: 93 leaf commands
- Covered: 41
- Coverage: 44.1%

## Summary
- TestBase_BasicWorkflow: proves `+base-create`, `+base-get`, `+table-create`, `+table-get`, and `+table-list`; key `t.Run(...)` proof points are `get base as bot`, `get table as bot`, and `list tables and find created table as bot`.
- TestBaseBlockDryRun: proves the five `+base-block-*` shortcuts request shapes without touching live data.
- TestBaseAppWorkflow: live user workflow in an isolated fixture Workspace covering Workspace entity listing, BaseApp create/get, Page create/list/get/update/delete, text Block create/list/get/update, and App cleanup through Drive delete (`type=bitable`). Set `LARK_CLI_E2E_BASEAPP_WORKSPACE_TOKEN` to enable it.
- TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat / TestBaseDashboardBlockUpdateDryRun_Position: prove `+dashboard-block-create` / `+dashboard-block-update` dry-run request shape carries the optional top-level `position` sibling and statistics `data_config.number_format`; TestBaseDashboardBlockCreateDryRun_InvalidNumberFormat proves the number_format enum validation rejects a bad formatName before any request.
- TestBaseFieldCreateDryRunArrayCompat: proves `+field-create` dry-run request shape for the internal JSON-array compatibility path.
- TestBaseFormQuestionsCreateDryRun: proves `+form-questions-create` preserves its POST body and renders the existing-question guard in command help.
- TestBaseFormDetailDryRun / TestBaseFormSubmitDryRun: prove shared-form detail and submission request shapes.
- TestBaseDashboardBlockGetDataDryRun: proves dashboard block data request shapes and identifier handling.
- TestBaseDashboardBlockLayoutPrecisionWorkflow: creates a temporary Base/table/dashboard, creates a statistics block with `position` and omitted `number_format`, asserts the server default, updates to a custom format, then verifies a precision-only update preserves `formatName`, and cleans up the block/dashboard/base. `+dashboard-create`, `+dashboard-delete`, `+dashboard-block-get` and `+dashboard-block-delete` have no dry-run coverage and rest on this test alone. This workflow was executed successfully against a live tenant on 2026-08-20 while validating PR #2118.
- TestBaseShareDryRun: proves dashboard/form share GET and PATCH routes, one-field update requests, explicit false preservation, and nested form settings without touching live data.
- TestBaseShareWorkflow: deployment-gated by `LARK_CLI_E2E_BASE_SHARE_READY=1`; creates a Base, table, form, and dashboard, updates each share field in a separate request, verifies get round trips for both resources, disables sharing, and cleans up the Base.
- TestBaseRecordBatchUpdatePerRecordDryRun: proves `+record-batch-update` preserves the per-record `update_records` request shape.
- TestBaseRecordBatchUpdatePerRecordWorkflow: creates two records, updates different field types in one request, asserts the minimal response contract, reads both records back, verifies a missing record ID is not prevalidated, and cleans up the temporary Base.
- TestBaseRecordHistoryListDryRunUsesExplicitRecordID / TestBaseRecordHistoryListDryRunRejectsNonPositiveMaxVersion: prove the history request keeps the explicit record ID and rejects explicitly non-positive cursors with a typed validation error.
- TestBase_RoleWorkflow: proves `+advperm-enable`, `+role-create`, `+role-list`, `+role-get`, and `+role-update`; key `t.Run(...)` proof points are `list as bot`, `get as bot`, and `update as bot`.
- TestBaseFormListDryRun_UsesBaseAndTableIdentifiers: proves `+form-list` dry-run request shape uses Base and table identifiers in the endpoint.
- TestBaseFormQuestionsCreateVisibleRuleDryRun / TestBaseFormQuestionsUpdateVisibleRuleDryRun: prove `+form-questions-create` / `+form-questions-update` dry-run request shape and that the optional `visible_rule` display condition is transcribed verbatim into the request body.
- TestBaseTableCopyDryRun: proves `+table-copy` and `+table-copy-status` request shapes, including the schema-safe default, table-name path escaping, explicit all+wait orchestration, Cobra duration parsing, and the opaque task ID body.
- TestBaseTableCopyWorkflow: feature-gated by `LARK_CLI_E2E_BASE_TABLE_COPY_READY=1` until the OpenAPI is deployed; creates a source table and record, proves schema-only copy, all no-wait plus status, all wait, record inclusion, and cleanup.
- TestBaseTemplateCenterDryRun: proves `+template-categories`, `+template-list`, and `+template-search` request shapes; the list case covers category, limit, and offset parameters.
- Cleanup note: `+table-delete` and `+role-delete` only run in cleanup and are intentionally left uncovered.
- Blocked area: table-copy live integration remains deployment-gated; dashboard, field, most record operations, most form operations, view, and workflow operations still lack deterministic create/read/update workflows in this suite.

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
| ✓ | base +template-categories | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/categories | dry-run only | request shape only |
| ✓ | base +template-list | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/list | `--category-key`; `--limit`/`--page-size`; `--offset`; dry-run only | request shape only |
| ✓ | base +template-search | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/search | `--keyword`; `--limit`; dry-run only | request shape only; blank keyword validation covered |
| ✕ | base +dashboard-arrange | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-block-create | shortcut | base_dashboard_block_layout_precision_dryrun_test.go::TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat; base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow | `--position` top-level; `--data-config.number_format`; dry-run + live | request shape plus live statistics block creation |
| ✓ | base +dashboard-block-delete | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow (cleanup) | `--base-token`; `--dashboard-id`; `--block-id`; `--yes`; live cleanup | deletes the temporary block |
| ✓ | base +dashboard-block-get | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow | `--base-token`; `--dashboard-id`; `--block-id`; live | reads back number_format; coordinates are not asserted (get is not contracted to echo position this iteration) |
| ✓ | base +dashboard-block-get-data | shortcut | base_dashboard_block_get_data_dryrun_test.go | `--base-token`; `--dashboard-id`; `--block-id`; dry-run only | request shape and identifier handling |
| ✕ | base +dashboard-block-list | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-block-update | shortcut | base_dashboard_block_layout_precision_dryrun_test.go::TestBaseDashboardBlockUpdateDryRun_Position; base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow | `--position` top-level dry-run; `--data-config.number_format` live | request shape for position; live workflow verifies number_format update and read-back, while position read-back remains unverified |
| ✓ | base +dashboard-create | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow | `--base-token`; `--name`; live | creates the temporary dashboard |
| ✓ | base +dashboard-delete | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow (cleanup) | `--base-token`; `--dashboard-id`; `--yes`; live cleanup | deletes the temporary dashboard |
| ✕ | base +dashboard-get | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-list | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-share-get | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/dashboard get; base_share_workflow_test.go::TestBaseShareWorkflow/dashboard share update and get | `--base-token`; `--dashboard-id`; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_BASE_SHARE_READY=1` |
| ✓ | base +dashboard-share-update | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/dashboard partial update, dashboard auto analysis is not exposed; base_share_workflow_test.go::TestBaseShareWorkflow/dashboard share update and get | one of `--enabled`; `--access-scope=invite`; `--show-source` per request; unsupported auto-analysis flag | single-field updates, explicit false, invite-only scope, live read-back, and backend-gated setting exclusion covered |
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
| ✓ | base +form-share-get | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/form get; base_share_workflow_test.go::TestBaseShareWorkflow/form share update and get | `--base-token`; `--table-id`; `--form-id`; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_BASE_SHARE_READY=1` |
| ✓ | base +form-share-update | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/form settings update; base_share_workflow_test.go::TestBaseShareWorkflow/form share update and get | one of share enablement; `access-scope=invite`; anonymous/login settings per request | single-field updates, login-plus-anonymous across separate requests, explicit false, and live read-back covered |
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
| ✓ | base +record-history-list | shortcut | base_record_history_dryrun_test.go::TestBaseRecordHistoryListDryRunUsesExplicitRecordID; TestBaseRecordHistoryListDryRunRejectsNonPositiveMaxVersion | `--base-token`; `--table-id`; `--record-id`; `--page-size`; `--max-version`; dry-run | request shape and typed cursor validation |
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
| ✓ | base +table-create | shortcut | base/helpers_test.go::createTableWithRetry | `--base-token`; `--name`; `--fields`; optional `--view` | helper asserts table id |
| ✓ | base +table-copy | shortcut | base_table_copy_dryrun_test.go::TestBaseTableCopyDryRun; base_table_copy_workflow_test.go::TestBaseTableCopyWorkflow | `--table-id` ID/name; default `--range schema`; explicit `--range all --wait --timeout` | dry-run covered; live workflow is deployment-gated and not yet verified |
| ✓ | base +table-copy-status | shortcut | base_table_copy_dryrun_test.go::TestBaseTableCopyDryRun/status; base_table_copy_workflow_test.go::TestBaseTableCopyWorkflow/all no-wait and status | opaque `--task-id` | dry-run covered; live polling is deployment-gated and not yet verified |
| ✕ | base +table-delete | shortcut |  | none | cleanup only |
| ✓ | base +table-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get table as bot | `--base-token`; `--table-id` | |
| ✓ | base +table-list | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/list tables and find created table as bot | `--base-token` | |
| ✕ | base +table-update | shortcut |  | none | no rename workflow yet |
| ✕ | base +title-resolve | shortcut |  | none | resolver workflow not covered |
| ✕ | base +url-resolve | shortcut |  | none | resolver workflow not covered |
| ✕ | base +view-create | shortcut |  | none | view workflows not covered |
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
