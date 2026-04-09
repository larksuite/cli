# Base CLI E2E Coverage

## Metrics
- Denominator: 68 leaf commands
- Covered: 63
- Coverage: 92.6%

## Summary
- TestBase_CoreWorkflow: proves `base +base-create`, `base +base-get`, and `base +base-copy`; key proof point is `get base` asserting the created base is readable and `copy base` returning a different base token.
- TestBase_AdvpermWorkflow: proves `base +advperm-enable` and `base +advperm-disable`; key proof points are `enable` and `disable` both succeeding against the same created base.
- TestBase_TableFieldRecordViewWorkflow: proves the main table, field, record, view, and data-query path; key `t.Run(...)` proof points include `record update`, `record history list`, `record upload attachment`, `view set filter`, `view get filter`, `view set timebar`, `view get timebar`, and `data query`.
- TestBase_DashboardWorkflow: proves `dashboard` and `dashboard-block` lifecycle reads and mutations; key `t.Run(...)` proof points include `dashboard update`, `dashboard block get`, `dashboard block update`, `dashboard block delete`, and `dashboard delete`.
- TestBase_FormWorkflow: proves `form` and `form-questions` lifecycle reads and mutations; key `t.Run(...)` proof points include `form update`, `form questions create`, `form questions update`, `form questions delete`, and `form delete`.
- TestBase_RoleWorkflow: proves advanced-permission role management; key `t.Run(...)` proof points include `list`, `get`, and `update` after setup creates a custom role.
- TestBase_WorkflowLifecycle: proves workflow definition reads and mutations; key `t.Run(...)` proof points include `list`, `get`, `update`, `enable`, and `disable`.
- Gap pattern: `base +table-delete`, `base +field-delete`, `base +record-delete`, `base +view-delete`, and `base +role-delete` only run in `parentT.Cleanup(...)`; they are not counted as covered because no testcase makes deletion the primary proof surface.

## Command Table
| Status | Cmd | Type | Testcase | Key Parameter Shapes | Notes / Uncovered Reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | base +advperm-disable | shortcut | base_advperm_workflow_test.go::TestBase_AdvpermWorkflow/disable | `--base-token --yes` | |
| ✓ | base +advperm-enable | shortcut | base_advperm_workflow_test.go::TestBase_AdvpermWorkflow/enable | `--base-token` | |
| ✓ | base +base-copy | shortcut | base_core_workflow_test.go::TestBase_CoreWorkflow/copy base | `--base-token --name --without-content --time-zone` | |
| ✓ | base +base-create | shortcut | base_core_workflow_test.go::TestBase_CoreWorkflow; base_advperm_workflow_test.go::TestBase_AdvpermWorkflow; base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow; base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow; base_dashboard_form_workflow_test.go::TestBase_FormWorkflow; base_role_workflow_test.go::TestBase_RoleWorkflow; base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle | `--name --time-zone` | |
| ✓ | base +base-get | shortcut | base_core_workflow_test.go::TestBase_CoreWorkflow/get base | `--base-token` | |
| ✓ | base +dashboard-block-create | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow | `--dashboard-id --name --type --data-config` | Covered through setup assertions on returned `block_id`. |
| ✓ | base +dashboard-block-delete | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard block delete | `--dashboard-id --block-id --yes` | |
| ✓ | base +dashboard-block-get | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard block get | `--dashboard-id --block-id` | |
| ✓ | base +dashboard-block-list | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard block list | `--dashboard-id` | |
| ✓ | base +dashboard-block-update | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard block update | `--dashboard-id --block-id --name --data-config` | |
| ✓ | base +dashboard-create | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow | `--base-token --name` | Covered through setup assertions on returned `dashboard_id`. |
| ✓ | base +dashboard-delete | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard delete | `--dashboard-id --yes` | |
| ✓ | base +dashboard-get | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard get | `--dashboard-id` | |
| ✓ | base +dashboard-list | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard list | `--base-token` | |
| ✓ | base +dashboard-update | shortcut | base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow/dashboard update | `--dashboard-id --name --theme-style` | |
| ✓ | base +data-query | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/data query | `--dsl` with `datasource`, `dimensions`, `measures`, and `shaper` | |
| ✓ | base +field-create | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow | `--table-id --json` for select, text, attachment, and datetime fields | Covered through setup assertions on returned field ids. |
| ✕ | base +field-delete | shortcut |  | none | cleanup-only in `helpers_test.go::createField` |
| ✓ | base +field-get | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/field get | `--table-id --field-id` | |
| ✓ | base +field-list | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/field list | `--table-id` | |
| ✓ | base +field-search-options | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/field search options | `--table-id --field-id --query` | |
| ✓ | base +field-update | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/field update | `--table-id --field-id --json` | |
| ✓ | base +form-create | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow | `--table-id --name` | Covered through setup assertions on returned `form_id`. |
| ✓ | base +form-delete | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form delete | `--table-id --form-id --yes` | |
| ✓ | base +form-get | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form get | `--table-id --form-id` | |
| ✓ | base +form-list | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form list | `--table-id` | |
| ✓ | base +form-questions-create | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form questions create | `--table-id --form-id --questions` | |
| ✓ | base +form-questions-delete | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form questions delete | `--table-id --form-id --question-ids --yes` | |
| ✓ | base +form-questions-list | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form questions list | `--table-id --form-id` | |
| ✓ | base +form-questions-update | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form questions update | `--table-id --form-id --questions` | |
| ✓ | base +form-update | shortcut | base_dashboard_form_workflow_test.go::TestBase_FormWorkflow/form update | `--table-id --form-id --name --description` | |
| ✕ | base +record-delete | shortcut |  | none | cleanup-only in `helpers_test.go::createRecord` |
| ✓ | base +record-get | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/record get | `--table-id --record-id` | |
| ✓ | base +record-history-list | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/record history list | `--table-id --record-id --page-size` | |
| ✓ | base +record-list | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/record list | `--table-id` | |
| ✓ | base +record-upload-attachment | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/record upload attachment | `--table-id --record-id --field-id --file` | |
| ✓ | base +record-upsert | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow; base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/record update | create body with `Name`, `Status`, `Note`; update body with `record-id` and changed fields | |
| ✓ | base +role-create | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow | `--base-token --json` with `role_name` and `role_type` | Covered through setup assertions on returned or resolved `role_id`. |
| ✕ | base +role-delete | shortcut |  | none | cleanup-only in `helpers_test.go::createRole` |
| ✓ | base +role-get | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/get | `--base-token --role-id` | |
| ✓ | base +role-list | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/list | `--base-token` | |
| ✓ | base +role-update | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow/update | `--base-token --role-id --json` | |
| ✓ | base +table-create | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow; base_dashboard_form_workflow_test.go::TestBase_DashboardWorkflow; base_dashboard_form_workflow_test.go::TestBase_FormWorkflow; base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle | `--name`; optional `--fields`; optional `--view` | Covered through setup assertions on returned table, field, and view ids. |
| ✕ | base +table-delete | shortcut |  | none | cleanup-only in `helpers_test.go::createTable` |
| ✓ | base +table-get | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/table get | `--table-id` | |
| ✓ | base +table-list | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/table list | `--base-token` | |
| ✓ | base +table-update | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/table update | `--table-id --name` | |
| ✓ | base +view-create | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow | `--table-id --json` for grid, gallery, and calendar views | Covered through setup assertions on returned `view_id`. |
| ✕ | base +view-delete | shortcut |  | none | cleanup-only in `helpers_test.go::createView` |
| ✓ | base +view-get | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get | `--table-id --view-id` | |
| ✓ | base +view-get-card | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get card | `--table-id --view-id` | |
| ✓ | base +view-get-filter | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get filter | `--table-id --view-id` | |
| ✓ | base +view-get-group | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get group | `--table-id --view-id` | |
| ✓ | base +view-get-sort | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get sort | `--table-id --view-id` | |
| ✓ | base +view-get-timebar | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view get timebar | `--table-id --view-id` | |
| ✓ | base +view-list | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view list | `--table-id` | |
| ✓ | base +view-rename | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view rename | `--table-id --view-id --name` | |
| ✓ | base +view-set-card | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view set card | `--table-id --view-id --json` | |
| ✓ | base +view-set-filter | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view set filter | `--table-id --view-id --json` | |
| ✓ | base +view-set-group | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view set group | `--table-id --view-id --json` | |
| ✓ | base +view-set-sort | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view set sort | `--table-id --view-id --json` | |
| ✓ | base +view-set-timebar | shortcut | base_table_record_view_workflow_test.go::TestBase_TableFieldRecordViewWorkflow/view set timebar | `--table-id --view-id --json` | |
| ✓ | base +workflow-create | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle | `--base-token --json` with `client_token`, `title`, and `steps` | Covered through setup assertions on returned `workflow_id`. |
| ✓ | base +workflow-disable | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle/disable | `--base-token --workflow-id` | |
| ✓ | base +workflow-enable | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle/enable | `--base-token --workflow-id` | |
| ✓ | base +workflow-get | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle/get | `--base-token --workflow-id` | |
| ✓ | base +workflow-list | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle/list | `--base-token` | |
| ✓ | base +workflow-update | shortcut | base_workflow_lifecycle_test.go::TestBase_WorkflowLifecycle/update | `--base-token --workflow-id --json` | |
