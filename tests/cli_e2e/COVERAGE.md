# CLI E2E Coverage

This document shows command-level coverage for the CLI E2E tests under `tests/cli_e2e/`.
It lists all available CLI commands and shows whether each command is covered by tests.

## Summary

| Domain | Total Commands | Covered | Not Covered | Coverage | Tests                 |
| ------ | -------------: | ------: | ----------: | -------: | --------------------- |
| `base` |             68 |      68 |           0 |     100% | `tests/cli_e2e/base/` |
| `task` |             24 |      12 |          12 |      50% | `tests/cli_e2e/task/` |
| `wiki` |              6 |       6 |           0 |     100% | `tests/cli_e2e/wiki/` |

## `base`

### Shortcuts

| Command                          | Covered |
| -------------------------------- | ------- |
| `base +advperm-enable`           | ✓       |
| `base +advperm-disable`          | ✓       |
| `base +base-create`              | ✓       |
| `base +base-get`                 | ✓       |
| `base +base-copy`                | ✓       |
| `base +dashboard-create`         | ✓       |
| `base +dashboard-list`           | ✓       |
| `base +dashboard-get`            | ✓       |
| `base +dashboard-update`         | ✓       |
| `base +dashboard-delete`         | ✓       |
| `base +dashboard-block-create`   | ✓       |
| `base +dashboard-block-list`     | ✓       |
| `base +dashboard-block-get`      | ✓       |
| `base +dashboard-block-update`   | ✓       |
| `base +dashboard-block-delete`   | ✓       |
| `base +data-query`               | ✓       |
| `base +field-create`             | ✓       |
| `base +field-list`               | ✓       |
| `base +field-get`                | ✓       |
| `base +field-update`             | ✓       |
| `base +field-search-options`     | ✓       |
| `base +field-delete`             | ✓       |
| `base +form-create`              | ✓       |
| `base +form-get`                 | ✓       |
| `base +form-list`                | ✓       |
| `base +form-update`              | ✓       |
| `base +form-delete`              | ✓       |
| `base +form-questions-create`    | ✓       |
| `base +form-questions-list`      | ✓       |
| `base +form-questions-update`    | ✓       |
| `base +form-questions-delete`    | ✓       |
| `base +record-upsert`            | ✓       |
| `base +record-list`              | ✓       |
| `base +record-get`               | ✓       |
| `base +record-history-list`      | ✓       |
| `base +record-upload-attachment` | ✓       |
| `base +record-delete`            | ✓       |
| `base +role-create`              | ✓       |
| `base +role-list`                | ✓       |
| `base +role-get`                 | ✓       |
| `base +role-update`              | ✓       |
| `base +role-delete`              | ✓       |
| `base +table-create`             | ✓       |
| `base +table-list`               | ✓       |
| `base +table-get`                | ✓       |
| `base +table-update`             | ✓       |
| `base +table-delete`             | ✓       |
| `base +view-create`              | ✓       |
| `base +view-list`                | ✓       |
| `base +view-get`                 | ✓       |
| `base +view-rename`              | ✓       |
| `base +view-set-filter`          | ✓       |
| `base +view-get-filter`          | ✓       |
| `base +view-set-group`           | ✓       |
| `base +view-get-group`           | ✓       |
| `base +view-set-sort`            | ✓       |
| `base +view-get-sort`            | ✓       |
| `base +view-set-timebar`         | ✓       |
| `base +view-get-timebar`         | ✓       |
| `base +view-set-card`            | ✓       |
| `base +view-get-card`            | ✓       |
| `base +view-delete`              | ✓       |
| `base +workflow-create`          | ✓       |
| `base +workflow-list`            | ✓       |
| `base +workflow-get`             | ✓       |
| `base +workflow-update`          | ✓       |
| `base +workflow-enable`          | ✓       |
| `base +workflow-disable`         | ✓       |

## `wiki`

### Resource Commands

#### `spaces`

| Command | Covered |
| ------- | ------- |
| `wiki spaces get` | ✓ |
| `wiki spaces get_node` | ✓ |
| `wiki spaces list` | ✓ |

#### `nodes`

| Command | Covered |
| ------- | ------- |
| `wiki nodes copy` | ✓ |
| `wiki nodes create` | ✓ |
| `wiki nodes list` | ✓ |

## `task`

### Shortcuts

| Command                   | Covered |
| ------------------------- | ------- |
| `task +assign`            | -       |
| `task +comment`           | ✓       |
| `task +complete`          | ✓       |
| `task +create`            | ✓       |
| `task +followers`         | -       |
| `task +get-my-tasks`      | -       |
| `task +reminder`          | ✓       |
| `task +reopen`            | ✓       |
| `task +tasklist-create`   | ✓       |
| `task +tasklist-members`  | -       |
| `task +tasklist-task-add` | ✓       |
| `task +update`            | -       |

### Resource Commands

#### `tasks`

| Command | Covered |
| ------- | ------- |
| `task tasks create` | - |
| `task tasks delete` | ✓ |
| `task tasks get` | ✓ |
| `task tasks list` | - |
| `task tasks patch` | - |

#### `tasklists`

| Command | Covered |
| ------- | ------- |
| `task tasklists add_members` | - |
| `task tasklists create` | - |
| `task tasklists delete` | ✓ |
| `task tasklists get` | ✓ |
| `task tasklists list` | - |
| `task tasklists patch` | - |
| `task tasklists tasks` | ✓ |
