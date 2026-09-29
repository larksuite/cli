# Base CLI E2E Coverage

## Metrics
- Denominator: 142 leaf commands (136 from `main` plus six permission shortcuts in this MR)
- Covered: 95
- Coverage: 66.9%

## Summary
- TestBase_BasicWorkflow: proves `+base-create`, `+base-get`, `+table-create`, `+table-get`, and `+table-list`; key `t.Run(...)` proof points are `get base as bot`, `get table as bot`, and `list tables and find created table as bot`.
- TestBaseBlockDryRun: proves the five `+base-block-*` shortcuts request shapes without touching live data.
- TestBaseAppWorkflow: live user workflow in an isolated fixture Workspace covering Workspace entity listing, BaseApp create/get, Page create/list/get/update/delete, text Block create/list/get/update, and App cleanup through Drive delete (`type=bitable`). Set `LARK_CLI_E2E_BASEAPP_WORKSPACE_TOKEN` to enable it.
- TestBaseWorkspaceDryRun / TestBaseappDryRun / TestBaseappPageDryRun / TestAppBlockDryRun / TestAppBlockGetDataDryRun: existing request-shape coverage for Workspace and BaseApp commands, plus local app URL resolution. These previously omitted rows are included in the refreshed command inventory; this does not represent new live verification.
- TestBaseButtonRuleDryRun: existing request-shape coverage for button field binding, reading and unbinding by ID/name, including field resolution and the final workflow ID body.
- TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat / TestBaseDashboardBlockUpdateDryRun_Position: prove `+dashboard-block-create` / `+dashboard-block-update` dry-run request shape carries the optional top-level `position` sibling and statistics `data_config.number_format`; TestBaseDashboardBlockCreateDryRun_InvalidNumberFormat proves the number_format enum validation rejects a bad formatName before any request.
- TestBaseFieldCreateDryRunArrayCompat: proves `+field-create` dry-run request shape for the internal JSON-array compatibility path.
- TestBaseFieldExtensionDryRun: proves `+field-extension-get`, `+field-extension-update`, and `+field-extension-update-cells` request shapes, including row-vs-column update bodies; TestBaseFieldExtensionUpdateCellsDryRunRejectsRowWithoutRecordID proves row updates require an explicit record.
- TestBaseFormQuestionsCreateDryRun: proves `+form-questions-create` preserves its POST body and renders the existing-question guard in command help.
- TestBaseFormDetailDryRun / TestBaseFormSubmitDryRun: prove shared-form detail and submission request shapes.
- TestBaseFormConfigDryRun: proves all eight form submission-settings, notifications, submit-actions, and lottery GET/write shortcuts use the expected method, endpoint, and representative request body without touching live data.
- TestBaseFormConfigWorkflow: deployment-gated isolated Base/table/form workflow that executes all eight form configuration shortcuts, reads back submission/notification/submit-action state, and relies on Base cleanup to remove the temporary configuration state. Set `LARK_CLI_E2E_FORM_CONFIG_READY=1` after the endpoints are deployed.
- TestBaseFormQueryPageDryRun / TestBaseFormDisplaySettingsDryRun / TestBaseFormShareQRCodeDryRun: prove the six form sub-resource shortcuts' method, URL and body — empty create body vs. `query_view_id` restore, `revision` sent only when passed (including `0`), the shared share-qrcode GET, and local rejection of a blank `--query-view-id`, an unknown `--display-mode` and an out-of-allowlist `--output` before any request. TestBaseFormSubmitTimezoneDryRun proves `--timezone` becomes `client_timezone` only when passed and that non-IANA names are rejected locally.
- Blocked area: live coverage for query-page / display-settings / share-qrcode waits on a confirmed deployment signal for those OpenAPIs; see the note at the top of base_form_subresource_dryrun_test.go.
- TestBaseDashboardBlockGetDataDryRun: proves dashboard block data request shapes and identifier handling.
- TestBaseDashboardViewDryRun: proves `view` block grid/gantt/gallery inline and file creation, filter/group/sort/visible-field configuration, partial/empty/null updates, unchanged view types, layout, GET routing, and help discovery. Create/update reject top-level `timebar` and `card` objects, empty objects, and nulls for inline/file inputs; `--no-validate` preserves them while still requiring a JSON object. This adds parameter coverage only.
- TestBaseDashboardViewWorkflow: gated by `LARK_CLI_E2E_VIEW_READY=1`; creates an isolated Base/table/dashboard, checks grid/gantt/gallery references with string `viewType`, and uses the returned view ID with `+view-get-filter/group/sort/visible-fields` to verify configuration updates, clearing, and preservation of untouched properties. Readback uses the view-property protocols rather than comparing dashboard references with writable input. The four getter rows now have deployment-gated test coverage; this is not new live verification. Local runs skip until the deployment gate is enabled. A previous live attempt was blocked before fixture creation because the test bot app lacked `base:app:create`; component endpoint deployment, clearing behavior, and page rendering remain unverified. Unit tests use separate request/reference fixtures and still verify that GET preserves stored `timebar/card` responses.
- TestBaseDashboardLinkageDryRun / TestBaseDashboardLinkageDryRunRejectsInvalidInput: prove top-level `--linkage` inline/file requests, replacement/clear/omission semantics, command help, and typed validation even with `--no-validate`. These parameter checks add no new leaf commands to the denominator.
- TestBaseDashboardLinkageWorkflow: deployment-gated by `LARK_CLI_E2E_LINKAGE_READY=1`; creates a temporary Base, two tables, a dashboard and four charts, checks same-table set/replace/clear/readback, preserves another source's configuration, and rejects a different table without changing the previous linkage. Not yet live-verified: requires bot credentials and deployed strict datasource validation. Multi-source equality and page click/cancel behavior need separate backend tests and UAT.
- TestBaseDashboardPivotDryRun_CreateValuesOnlyMultipleMetrics / UpdateExplicitSortClear / InvalidReferenceIsTyped / RejectsMissingAndUnsupportedConfig / UpdateRejectsInvalidReferenceInReplacedArea: prove pivot create defaults and multi-metric request shape, exact empty-sort update replacement, required/allowed configuration fields, and typed invalid-reference failures without touching live data.
- TestBaseDashboardPivotWorkflow: tenant-gated isolated Base/table/dashboard workflow covering create-time FIELD asc initialization, clearing dimensions and sort while preserving two metrics, Get/Get-data consistency, values-only cardinality, and cleanup. It is intentionally not executed during base coding; QA or the task owner must run it with tenant credentials after deployment.
- TestBaseDashboardBlockLayoutPrecisionWorkflow: creates a temporary Base/table/dashboard, creates a statistics block with `position` and omitted `number_format`, asserts the server default, updates to a custom format, then verifies a precision-only update preserves `formatName`, and cleans up the block/dashboard/base. `+dashboard-create`, `+dashboard-delete`, `+dashboard-block-get` and `+dashboard-block-delete` have no dry-run coverage and rest on this test alone. This workflow was executed successfully against a live tenant on 2026-08-20 while validating PR #2118.
- TestBaseDashboardBlockRankingCreateDryRun / TestBaseDashboardBlockRankingUpdateDryRunPreservesPatch / TestBaseDashboardBlockRankingDryRunRejectsInvalidConfig: prove ranking create defaults, top-level patch preservation, and typed validation failures for unsupported fields and malformed filters.
- TestBaseShareDryRun: proves dashboard/form share GET and PATCH routes, one-field update requests, explicit false preservation, and nested form settings without touching live data.
- TestBaseShareWorkflow: deployment-gated by `LARK_CLI_E2E_BASE_SHARE_READY=1`; creates a Base, table, form, and dashboard, updates each share field in a separate request, verifies get round trips for both resources, disables sharing, and cleans up the Base.
- TestBaseRecordBatchUpdatePerRecordDryRun: proves `+record-batch-update` preserves the per-record `update_records` request shape.
- TestBaseRecordBatchUpdatePerRecordWorkflow: creates two records, generates their share links using mixed `--record-id` / `--record-ids` input, updates different field types in one request, asserts the minimal response contract, reads both records back, verifies a missing record ID is not prevalidated, and cleans up the temporary Base.
- TestBaseRecordShareLinkCreateDryRunAcceptsSingularAndPluralRecordIDFlags: proves singular and plural record ID flags compose into one deduplicated share-link request.
- TestBaseRecordHistoryListDryRunUsesExplicitRecordID / TestBaseRecordHistoryListDryRunRejectsNonPositiveMaxVersion: prove the history request keeps the explicit record ID and rejects explicitly non-positive cursors with a typed validation error.
- TestBasePermissionShortcutsDryRun: proves the request shapes of the six permission shortcuts (`+role-member-list/add/remove`, `+advperm-share-get/update`, `+advperm-apply`) without touching live data.
- TestBase_RoleWorkflow: proves `+advperm-enable`, `+role-create`, `+role-list`, `+role-get`, and `+role-update`; key `t.Run(...)` proof points are `list as bot`, `get as bot`, and `update as bot`.
- TestBaseFormListDryRun_UsesBaseAndTableIdentifiers: proves `+form-list` dry-run request shape uses Base and table identifiers in the endpoint.
- TestBaseFormQuestionsCreateVisibleRuleDryRun / TestBaseFormQuestionsUpdateVisibleRuleDryRun: prove `+form-questions-create` / `+form-questions-update` dry-run request shape and that the optional `visible_rule` display condition is transcribed verbatim into the request body.
- TestBaseTableCopyDryRun: proves `+table-copy` and `+table-copy-status` request shapes, including the schema-safe default, table-name path escaping, explicit all+wait orchestration, Cobra duration parsing, and the opaque task ID body.
- TestBaseTableCopyWorkflow: feature-gated by `LARK_CLI_E2E_BASE_TABLE_COPY_READY=1` until the OpenAPI is deployed; creates a source table and record, proves schema-only copy, all no-wait plus status, all wait, record inclusion, and cleanup.
- TestBaseTemplateCenterDryRun: proves `+template-categories`, `+template-list`, and `+template-search` request shapes; the list case covers category, limit, and offset parameters.
- Cleanup note: `+table-delete` and `+role-delete` only run in cleanup and are intentionally left uncovered.
- Blocked area: table-copy live integration remains deployment-gated; remaining dashboard, field, most record operations, most form operations, view, and workflow operations still lack deterministic create/read/update workflows in this suite.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | base +advperm-apply | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token`; `--message` | dry-run request shape |
| ✕ | base +advperm-disable | shortcut |  | none | no disable workflow yet |
| ✓ | base +advperm-enable | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow | `--base-token` | |
| ✓ | base +advperm-share-get | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token` | dry-run request shape |
| ✓ | base +advperm-share-update | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token`; `--enable-share` | dry-run request shape |
| ✕ | base +base-copy | shortcut |  | none | no copy workflow yet |
| ✓ | base +base-create | shortcut | base/helpers_test.go::createBaseWithRetry | `--name`; `--time-zone` | helper asserts created base token |
| ✓ | base +base-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get base as bot | `--base-token` | |
| ✕ | base +base-update | shortcut |  | none | timezone update has unit coverage only |
| ✓ | base +base-block-create | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/create | `--base-token`; `--type`; `--name`; `--parent-id`; dry-run only | request shape only |
| ✓ | base +base-block-delete | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/delete | `--base-token`; `--block-id`; dry-run only | request shape only |
| ✓ | base +base-block-list | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/list all,list folder | `--base-token`; optional `--parent-id`; optional `--type`; dry-run only | request shape only |
| ✓ | base +base-block-move | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/move root,move after | `--base-token`; `--block-id`; optional `--parent-id`; `--after-id`; dry-run only | request shape only |
| ✓ | base +base-block-rename | shortcut | base_block_dryrun_test.go::TestBaseBlockDryRun/rename | `--base-token`; `--block-id`; `--name`; dry-run only | request shape only |
| ✓ | base +template-categories | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/categories | dry-run only | request shape only |
| ✓ | base +template-list | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/list | `--category-key`; `--limit`/`--page-size`; `--offset`; dry-run only | request shape only |
| ✓ | base +template-search | shortcut | base_template_center_dryrun_test.go::TestBaseTemplateCenterDryRun/search | `--keyword`; `--limit`; dry-run only | request shape only; blank keyword validation covered |
| ✕ | base +dashboard-arrange | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-block-create | shortcut | base_dashboard_block_layout_precision_dryrun_test.go::TestBaseDashboardBlockCreateDryRun_PositionAndNumberFormat; base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow; base_dashboard_block_ranking_dryrun_test.go::TestBaseDashboardBlockRankingCreateDryRun; TestBaseDashboardBlockRankingDryRunRejectsInvalidConfig; base_dashboard_view_dryrun_test.go::TestBaseDashboardViewDryRun; base_dashboard_pivot_dryrun_test.go::TestBaseDashboardPivotDryRun_CreateValuesOnlyMultipleMetrics; TestBaseDashboardPivotDryRun_RejectsMissingAndUnsupportedConfig; base_dashboard_pivot_workflow_test.go::TestBaseDashboardPivotWorkflow | `--position` top-level; `--data-config.number_format`; ranking defaults and invalid child/top-level/filter fields; dry-run + live; pivot values-only/multi-metric/default sort/required fields; tenant-gated live | request shape, typed validation envelope, plus live statistics block creation; view configuration dry-run covered, live blocked by fixture creation scope; pivot create/get workflow is tenant-gated |
| ✓ | base +dashboard-block-delete | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow (cleanup) | `--base-token`; `--dashboard-id`; `--block-id`; `--yes`; live cleanup | deletes the temporary block |
| ✓ | base +dashboard-block-get | shortcut | base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow; base_dashboard_linkage_workflow_test.go::TestBaseDashboardLinkageWorkflow; base_dashboard_view_dryrun_test.go::TestBaseDashboardViewDryRun; base_dashboard_pivot_workflow_test.go::TestBaseDashboardPivotWorkflow | `--base-token`; `--dashboard-id`; `--block-id`; live + dry-run + tenant-gated live | reads back number_format; linkage persistence test is deployment-gated and not yet live-verified; coordinates are not asserted (get is not contracted to echo position this iteration); view configuration dry-run covered, live blocked by fixture creation scope; pivot workflow reads back rows/columns/values/sort |
| ✓ | base +dashboard-block-get-data | shortcut | base_dashboard_block_get_data_dryrun_test.go; base_dashboard_pivot_workflow_test.go::TestBaseDashboardPivotWorkflow | `--base-token`; `--dashboard-id`; `--block-id`; dry-run + tenant-gated live | request shape and identifier handling; pivot workflow verifies values-only cardinality and measures |
| ✕ | base +dashboard-block-list | shortcut |  | none | dashboard workflows not covered |
| ✓ | base +dashboard-block-update | shortcut | base_dashboard_block_layout_precision_dryrun_test.go::TestBaseDashboardBlockUpdateDryRun_Position; base_dashboard_block_layout_precision_workflow_test.go::TestBaseDashboardBlockLayoutPrecisionWorkflow; base_dashboard_block_ranking_dryrun_test.go::TestBaseDashboardBlockRankingUpdateDryRunPreservesPatch; base_dashboard_linkage_dryrun_test.go::TestBaseDashboardLinkageDryRun; TestBaseDashboardLinkageDryRunRejectsInvalidInput; base_dashboard_linkage_workflow_test.go::TestBaseDashboardLinkageWorkflow; base_dashboard_view_dryrun_test.go::TestBaseDashboardViewDryRun; base_dashboard_pivot_dryrun_test.go::TestBaseDashboardPivotDryRun_UpdateExplicitSortClear; TestBaseDashboardPivotDryRun_UpdateRejectsInvalidReferenceInReplacedArea; base_dashboard_pivot_workflow_test.go::TestBaseDashboardPivotWorkflow | `--position` top-level dry-run; `--data-config.number_format` live; ranking `limit_size` patch dry-run; `--linkage.target_block_ids` inline/file/empty/omitted and invalid input dry-run; exact `data_config.sort:[]`; pivot replacement reference validation; tenant-gated live | position/ranking/linkage request shape covered; linkage persistence is deployment-gated and not yet live-verified; existing live workflow verifies number_format update and read-back, while position read-back remains unverified; view configuration dry-run covered, live blocked by fixture creation scope; pivot workflow verifies sort/dimension clearing |
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
| ✓ | base +field-extension-get | shortcut | base_field_extension_dryrun_test.go::TestBaseFieldExtensionDryRun/get | `--base-token`; `--table-id`; `--field-id`; dry-run only | request shape only |
| ✓ | base +field-extension-update | shortcut | base_field_extension_dryrun_test.go::TestBaseFieldExtensionDryRun/update | `--json.extension_id`; `--json.inputs.prompt`; `--yes`; dry-run only | request shape only |
| ✓ | base +field-extension-update-cells | shortcut | base_field_extension_dryrun_test.go::TestBaseFieldExtensionDryRun/update cells row,update cells column; TestBaseFieldExtensionUpdateCellsDryRunRejectsRowWithoutRecordID | `--type=row` + repeated `--record-id`; `--type=column` + optional `--view-id`; `--yes`; dry-run only | request shape and row validation |
| ✕ | base +field-get | shortcut |  | none | field workflows not covered |
| ✕ | base +field-list | shortcut |  | none | field workflows not covered |
| ✕ | base +field-search-options | shortcut |  | none | field workflows not covered |
| ✕ | base +field-update | shortcut |  | none | field workflows not covered |
| ✕ | base +form-create | shortcut |  | none | form workflows not covered |
| ✕ | base +form-delete | shortcut |  | none | form workflows not covered |
| ✓ | base +form-display-settings-get | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormDisplaySettingsDryRun/get | `--base-token`; `--table-id`; `--form-id`; dry-run only | request shape only; live blocked on deployment signal |
| ✓ | base +form-display-settings-update | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormDisplaySettingsDryRun | `--display-mode`; optional `--revision` (incl. `0`); dry-run only | PATCH body, revision omission, enum rejection; live blocked on deployment signal |
| ✓ | base +form-detail | shortcut | base_form_detail_dryrun_test.go::TestBaseFormDetailDryRun | `--share-token`; dry-run only | shared-form request shape |
| ✕ | base +form-get | shortcut |  | none | form workflows not covered |
| ✓ | base +form-query-page-create | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormQueryPageDryRun | no `--query-view-id` (body `{}`); `--query-view-id`; blank id; dry-run only | create vs. restore body and blank-id rejection; live blocked on deployment signal |
| ✓ | base +form-query-page-get | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormQueryPageDryRun/get | `--base-token`; `--table-id`; `--form-id`; dry-run only | request shape only; live blocked on deployment signal |
| ✓ | base +form-list | shortcut | base_form_detail_dryrun_test.go::TestBaseFormListDryRun_UsesBaseAndTableIdentifiers | `--base-token`; `--table-id`; dry-run only | request shape only |
| ✓ | base +form-submission-settings-get | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/submission settings get; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | identifiers; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_FORM_CONFIG_READY=1` |
| ✓ | base +form-submission-settings-update | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/submission settings update; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | `allow_modify_submission=true`; dry-run + deployment-gated live | representative single-group update |
| ✓ | base +form-notifications-get | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/notifications get; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | identifiers; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_FORM_CONFIG_READY=1` |
| ✓ | base +form-notifications-update | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/notifications update; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | disable on-submission notification; dry-run + deployment-gated live | non-destructive temporary-form update |
| ✓ | base +form-submit-actions-get | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/submit actions get; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | identifiers; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_FORM_CONFIG_READY=1` |
| ✓ | base +form-submit-actions-update | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/submit actions update; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | disable redirect; dry-run + deployment-gated live | non-destructive temporary-form update |
| ✓ | base +form-lottery-get | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/lottery get; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | identifiers; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_FORM_CONFIG_READY=1` |
| ✓ | base +form-lottery-action | shortcut | base_form_config_dryrun_test.go::TestBaseFormConfigDryRun/lottery action; base_form_config_workflow_test.go::TestBaseFormConfigWorkflow | disable action; dry-run + deployment-gated live | non-destructive temporary-form action |
| ✓ | base +form-share-get | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/form get; base_share_workflow_test.go::TestBaseShareWorkflow/form share update and get | `--base-token`; `--table-id`; `--form-id`; dry-run + deployment-gated live | live requires `LARK_CLI_E2E_BASE_SHARE_READY=1` |
| ✓ | base +form-share-qrcode-download | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormShareQRCodeDryRun | `--output`; `--overwrite`; out-of-allowlist `--output`; dry-run only | shared GET and dry-run `--output` rejection; write path covered by shortcuts/base unit tests |
| ✓ | base +form-share-qrcode-get | shortcut | base_form_subresource_dryrun_test.go::TestBaseFormShareQRCodeDryRun/get | `--base-token`; `--table-id`; `--form-id`; dry-run only | request shape only; live needs share enabled and a deployment signal |
| ✓ | base +form-share-update | shortcut | base_share_dryrun_test.go::TestBaseShareDryRun/form settings update; base_share_workflow_test.go::TestBaseShareWorkflow/form share update and get | one of share enablement; `access-scope=invite`; anonymous/login settings per request | single-field updates, login-plus-anonymous across separate requests, explicit false, and live read-back covered |
| ✓ | base +form-questions-create | shortcut | TestBaseFormQuestionsCreateVisibleRuleDryRun; base_form_questions_create_dryrun_test.go | questions[].visible_rule; dry-run | request body, visible_rule passthrough, and help guard covered |
| ✓ | base +form-questions-delete | shortcut | base_form_questions_dryrun_test.go::TestBaseFormQuestionsDeleteDefaultDryRun; TestBaseFormQuestionsDeleteKeepFieldDryRun | `question_ids`; optional `keep_field=true` | default destructive body and opt-in field-preserving body covered |
| ✕ | base +form-questions-list | shortcut |  | none | form workflows not covered |
| ✓ | base +form-questions-update | shortcut | TestBaseFormQuestionsUpdateVisibleRuleDryRun | questions[].visible_rule | dry-run: request shape + visible_rule body passthrough |
| ✓ | base +form-submit | shortcut | base_form_submit_dryrun_test.go::TestBaseFormSubmitDryRun; base_form_subresource_dryrun_test.go::TestBaseFormSubmitTimezoneDryRun | `--share-token`; `--json`; optional `--timezone`; dry-run only | submission request shape; `client_timezone` only when passed; invalid IANA name rejected |
| ✕ | base +form-update | shortcut |  | none | form workflows not covered |
| ✓ | base +record-batch-create | shortcut | base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--base-token`; `--table-id`; `--json.create_records` | seeds heterogeneous live workflow records |
| ✓ | base +record-batch-update | shortcut | base_record_batch_update_dryrun_test.go::TestBaseRecordBatchUpdatePerRecordDryRun; base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--base-token`; `--table-id`; `--json.update_records`; dry-run + live | heterogeneous select/number update with write-back verification |
| ✕ | base +record-delete | shortcut |  | none | record workflows not covered |
| ✓ | base +record-get | shortcut | base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | `--record-id`; repeated `--field-id`; `--format json` | reads back select and number values after batch update |
| ✓ | base +record-history-list | shortcut | base_record_history_dryrun_test.go::TestBaseRecordHistoryListDryRunUsesExplicitRecordID; TestBaseRecordHistoryListDryRunRejectsNonPositiveMaxVersion | `--base-token`; `--table-id`; `--record-id`; `--page-size`; `--max-version`; dry-run | request shape and typed cursor validation |
| ✕ | base +record-list | shortcut |  | none | record workflows not covered |
| ✕ | base +record-search | shortcut |  | none | record workflows not covered |
| ✓ | base +record-share-link-create | shortcut | base_record_share_link_dryrun_test.go::TestBaseRecordShareLinkCreateDryRunAcceptsSingularAndPluralRecordIDFlags; base_record_batch_update_workflow_test.go::TestBaseRecordBatchUpdatePerRecordWorkflow | canonical `--record-id`; hidden compatibility alias `--record-ids`; dry-run + live | singular and plural flags compose into one request |
| ✓ | base +record-upload-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/upload | dry-run only | request shape only |
| ✓ | base +record-download-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/download | dry-run only | request shape only |
| ✓ | base +record-remove-attachment | shortcut | base_attachment_dryrun_test.go::TestBase_AttachmentDryRun/remove | dry-run only | request shape only |
| ✕ | base +record-upsert | shortcut |  | none | record workflows not covered |
| ✓ | base +role-create | shortcut | base/helpers_test.go::createRole | `--base-token`; `--json` | helper asserts created role id |
| ✕ | base +role-delete | shortcut |  | none | cleanup only |
| ✓ | base +role-get | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/get as bot | `--base-token`; `--role-id` | |
| ✓ | base +role-list | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/list as bot | `--base-token` | |
| ✓ | base +role-member-add | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token`; `--role-id`; `--user-id`/`--department-id`/`--chat-id` | dry-run request shape |
| ✓ | base +role-member-list | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token`; `--role-id`; `--page-size` | dry-run request shape |
| ✓ | base +role-member-remove | shortcut | base_permission_dryrun_test.go::TestBasePermissionShortcutsDryRun | `--base-token`; `--role-id`; `--chat-id` | dry-run request shape |
| ✓ | base +role-update | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/update as bot | `--base-token`; `--role-id`; `--json` | |
| ✕ | base +sql-catalog | shortcut |  | none | catalog has unit coverage only |
| ✓ | base +sql-query | shortcut | base_sql_dryrun_test.go::TestBaseSQLQueryDryRun | `--base-token`; `--sql`; dry-run | POST request shape and verbatim SQL preservation |
| ✓ | base +table-create | shortcut | base/helpers_test.go::createTableWithRetry | `--base-token`; `--name`; `--fields`; optional `--view` | helper asserts table id |
| ✓ | base +table-copy | shortcut | base_table_copy_dryrun_test.go::TestBaseTableCopyDryRun; base_table_copy_workflow_test.go::TestBaseTableCopyWorkflow | `--table-id` ID/name; default `--range schema`; explicit `--range all --wait --timeout` | dry-run covered; live workflow is deployment-gated and not yet verified |
| ✓ | base +table-copy-status | shortcut | base_table_copy_dryrun_test.go::TestBaseTableCopyDryRun/status; base_table_copy_workflow_test.go::TestBaseTableCopyWorkflow/all no-wait and status | opaque `--task-id` | dry-run covered; live polling is deployment-gated and not yet verified |
| ✕ | base +table-delete | shortcut |  | none | cleanup only |
| ✓ | base +table-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get table as bot | `--base-token`; `--table-id` | |
| ✓ | base +table-list | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/list tables and find created table as bot | `--base-token` | |
| ✕ | base +table-update | shortcut |  | none | no rename workflow yet |
| ✕ | base +title-resolve | shortcut |  | none | resolver workflow not covered |
| ✓ | base +url-resolve | shortcut | base_app_dryrun_test.go::TestBaseappDryRun/resolve_app_URL_locally | app URL with workspace and page query; dry-run | local resolution output; remote resolver not covered |
| ✕ | base +view-create | shortcut |  | none | view workflows not covered |
| ✕ | base +view-delete | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get | shortcut |  | none | view workflows not covered |
| ✕ | base +view-get-card | shortcut |  | none | view workflows not covered |
| ✓ | base +view-get-filter | shortcut | base_dashboard_view_workflow_test.go::TestBaseDashboardViewWorkflow/grid | `--base-token`; `--table-id`; `--view-id`; deployment-gated live | verifies the referenced view's text equality filter and clearing; not yet live-verified |
| ✓ | base +view-get-group | shortcut | base_dashboard_view_workflow_test.go::TestBaseDashboardViewWorkflow/grid | `--base-token`; `--table-id`; `--view-id`; deployment-gated live | verifies grouping field, direction, preservation and clearing; not yet live-verified |
| ✓ | base +view-get-sort | shortcut | base_dashboard_view_workflow_test.go::TestBaseDashboardViewWorkflow/grid | `--base-token`; `--table-id`; `--view-id`; deployment-gated live | verifies sort field, direction, preservation and clearing; not yet live-verified |
| ✕ | base +view-get-timebar | shortcut |  | none | view workflows not covered |
| ✓ | base +view-get-visible-fields | shortcut | base_dashboard_view_workflow_test.go::TestBaseDashboardViewWorkflow/grid | `--base-token`; `--table-id`; `--view-id`; deployment-gated live | verifies ordered field references after create and partial updates; not yet live-verified |
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
| ✓ | base +app-block-create | shortcut | base_app_dryrun_test.go::TestAppBlockDryRun/create_chart | `--app-token`; `--page-id`; `--type`; `--data-config`; dry-run | request shape and data-source normalization |
| ✓ | base +app-block-get | shortcut | base_app_dryrun_test.go::TestAppBlockDryRun/get | `--app-token`; `--page-id`; `--block-id`; dry-run | request shape |
| ✓ | base +app-block-get-data | shortcut | base_app_dryrun_test.go::TestAppBlockGetDataDryRun | `--app-token`; `--base-token`; `--block-id`; dry-run | request shape and identifier validation |
| ✓ | base +app-block-list | shortcut | base_app_dryrun_test.go::TestAppBlockDryRun/list | `--app-token`; `--page-id`; dry-run | request shape and page size |
| ✓ | base +app-block-update | shortcut | base_app_dryrun_test.go::TestAppBlockDryRun/update | `--app-token`; `--page-id`; `--block-id`; dry-run | request shape and required update fields |
| ✓ | base +app-create | shortcut | base_app_dryrun_test.go::TestBaseappDryRun/create | `--workspace-token`; `--name`; `--theme-style`; dry-run | BaseApp create endpoint |
| ✓ | base +app-get | shortcut | base_app_dryrun_test.go::TestBaseappDryRun/get | `--app-token`; dry-run | request shape |
| ✓ | base +app-page-create | shortcut | base_app_dryrun_test.go::TestBaseappPageDryRun/create | `--app-token`; `--name`; dry-run | request shape |
| ✓ | base +app-page-delete | shortcut | base_app_dryrun_test.go::TestBaseappPageDryRun/delete | `--app-token`; `--page-id`; dry-run | DELETE request shape |
| ✓ | base +app-page-get | shortcut | base_app_dryrun_test.go::TestBaseappPageDryRun/get | `--app-token`; `--page-id`; dry-run | request shape |
| ✓ | base +app-page-list | shortcut | base_app_dryrun_test.go::TestBaseappPageDryRun/list | `--app-token`; dry-run | request shape |
| ✓ | base +app-page-update | shortcut | base_app_dryrun_test.go::TestBaseappPageDryRun/rename | `--app-token`; `--page-id`; `--name`; dry-run | PATCH request shape |
| ✓ | base +button-rule-bind | shortcut | base_button_rule_dryrun_test.go::TestBaseButtonRuleDryRun/bind | `--field-id` ID/name; `--workflow-id`; dry-run | field resolution and PUT body |
| ✓ | base +button-rule-get | shortcut | base_button_rule_dryrun_test.go::TestBaseButtonRuleDryRun/get | `--field-id` ID/name; dry-run | field resolution and GET request |
| ✓ | base +button-rule-unbind | shortcut | base_button_rule_dryrun_test.go::TestBaseButtonRuleDryRun/unbind | `--field-id` ID/name; dry-run | explicit empty workflow ID |
| ✓ | base +contact-field-enrichment-create | shortcut | base_contact_field_enrichment_dryrun_test.go::TestBaseContactFieldEnrichmentDryRun/create | `--base-token`; `--table-id`; `--json`; dry-run | POST request shape and source-field/info-type body |
| ✓ | base +contact-field-enrichment-list | shortcut | base_contact_field_enrichment_dryrun_test.go::TestBaseContactFieldEnrichmentDryRun/list | `--base-token`; `--table-id`; dry-run | GET request shape |
| ✓ | base +workspace-create | shortcut | base_app_dryrun_test.go::TestBaseWorkspaceDryRun/create | `--name`; dry-run | POST request shape |
| ✓ | base +workspace-entity-list | shortcut | base_app_dryrun_test.go::TestBaseWorkspaceDryRun/entity-list | `--workspace-token`; `--type`; dry-run | GET request shape |
| ✓ | base +workspace-move-in | shortcut | base_app_dryrun_test.go::TestBaseWorkspaceDryRun/move-in | `--workspace-token`; `--entity-token`; dry-run | move-in request shape |
