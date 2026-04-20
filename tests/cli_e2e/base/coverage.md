# Base CLI E2E Coverage

## Metrics
- Denominator: 73 leaf commands
- Covered: 10
- Coverage: 13.7%

## Summary
- TestBase_BasicWorkflow: proves `+base-create`, `+base-get`, `+table-create`, `+table-get`, and `+table-list`; key `t.Run(...)` proof points are `get base as bot`, `get table as bot`, and `list tables and find created table as bot`.
- TestBase_RoleWorkflow: proves `+advperm-enable`, `+role-create`, `+role-list`, `+role-get`, and `+role-update`; key `t.Run(...)` proof points are `list as bot`, `get as bot`, and `update as bot`.
- Cleanup note: `+table-delete` and `+role-delete` only run in cleanup and are intentionally left uncovered.
- Blocked area: dashboard, field, form, record, view, and workflow operations still lack deterministic create/read/update workflows in this suite.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | base +advperm-disable | shortcut |  | none | no disable workflow yet |
| ✓ | base +advperm-enable | shortcut | base_role_workflow_test.go::TestBase_RoleWorkflow | `--base-token` | |
| ✕ | base +base-copy | shortcut |  | none | no copy workflow yet |
| ✓ | base +base-create | shortcut | base/helpers_test.go::createBaseWithRetry | `--name`; `--time-zone` | helper asserts created base token |
| ✓ | base +base-get | shortcut | base_basic_workflow_test.go::TestBase_BasicWorkflow/get base as bot | `--base-token` | |
| ✕ | base +dashboard-arrange | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-create | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-delete | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-get | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-list | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-block-update | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-create | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-delete | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-get | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-list | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +dashboard-update | shortcut |  | none | dashboard workflows not covered |
| ✕ | base +data-query | shortcut |  | none | no data-query assertions yet |
| ✕ | base +field-create | shortcut |  | none | field workflows not covered |
| ✕ | base +field-delete | shortcut |  | none | field workflows not covered |
| ✕ | base +field-get | shortcut |  | none | field workflows not covered |
| ✕ | base +field-list | shortcut |  | none | field workflows not covered |
| ✕ | base +field-search-options | shortcut |  | none | field workflows not covered |
| ✕ | base +field-update | shortcut |  | none | field workflows not covered |
| ✕ | base +form-create | shortcut |  | none | form workflows not covered |
| ✕ | base +form-delete | shortcut |  | none | form workflows not covered |
| ✕ | base +form-get | shortcut |  | none | form workflows not covered |
| ✕ | base +form-list | shortcut |  | none | form workflows not covered |
| ✕ | base +form-questions-create | shortcut |  | none | form workflows not covered |
| ✕ | base +form-questions-delete | shortcut |  | none | form workflows not covered |
| ✕ | base +form-questions-list | shortcut |  | none | form workflows not covered |
| ✕ | base +form-questions-update | shortcut |  | none | form workflows not covered |
| ✕ | base +form-update | shortcut |  | none | form workflows not covered |
| ✕ | base +record-batch-create | shortcut |  | none | record workflows not covered |
| ✕ | base +record-batch-update | shortcut |  | none | record workflows not covered |
| ✕ | base +record-delete | shortcut |  | none | record workflows not covered |
| ✕ | base +record-get | shortcut |  | none | record workflows not covered |
| ✕ | base +record-history-list | shortcut |  | none | record workflows not covered |
| ✕ | base +record-list | shortcut |  | none | record workflows not covered |
| ✕ | base +record-search | shortcut |  | none | record workflows not covered |
| ✕ | base +record-upload-attachment | shortcut |  | none | record workflows not covered |
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
