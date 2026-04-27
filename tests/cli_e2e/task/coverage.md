# Task CLI E2E Coverage

## Metrics
- Denominator: 29 leaf commands
- Covered: 14
- Coverage: 48.3%

## Summary
- TestTask_StatusWorkflow: creates a task via `task +create`, then proves `task +complete`, `task tasks get`, and `task +reopen` through `complete`, `get completed task`, `reopen`, and `get reopened task`; asserts `status` flips between `done` and `todo` and `completed_at` is set then cleared.
- TestTask_ReminderWorkflow: creates a task with a due time via `task +create`, then proves `task +reminder` and `task tasks get` through `set reminder`, `get task with reminder`, `remove reminder`, and `get task without reminder`; asserts `relative_fire_minute=30`, reminder id presence, and reminder removal.
- TestTask_CommentWorkflow: creates a task via `task +create`, runs `comment`, and asserts the returned comment id is non-empty; this is the direct proof for `task +comment`.
- TestTask_UpdateWorkflow: creates a task as `--as user`, proves `task +update` and `task tasks patch` through repeated read-after-write `task tasks get`, then completes it via `task +complete` and verifies the completed task state directly.
- TestTask_TasklistWorkflowAsBot: runs `create tasklist with task`, then `get tasklist`, `list tasklist tasks`, and `get task`; proves `task +tasklist-create`, `task tasklists get`, `task tasklists tasks`, and `task tasks get` with seeded task payload and task-to-tasklist linkage.
- TestTask_TasklistWorkflowAsUser: creates a tasklist as `--as user`, patches its name through `task tasklists patch`, then proves both `task tasklists get` and `task tasklists list` return the patched tasklist.
- TestTask_TasklistAddTaskWorkflow: creates a standalone tasklist and task, runs `add task to tasklist`, then `list tasklist tasks` and `get task with tasklist link`; proves `task +tasklist-task-add`, `task tasklists tasks`, and `task tasks get`, including no failed tasks in the add response.
- Cleanup path note: workflow-created tasks and tasklists are deleted through direct `task tasks delete` / `task tasklists delete` cleanup paths in `helpers_test.go::createTask`, `helpers_test.go::createTasklist`, `tasklist_workflow_test.go::TestTask_TasklistWorkflowAsBot`, and `tasklist_workflow_test.go::TestTask_TasklistWorkflowAsUser`, but those cleanup-only executions are not counted as command coverage because no testcase asserts delete behavior as the primary proof surface.
- Blocked area: assignee, follower, and tasklist member mutations still require stable real-user `open_id` fixtures; the current suite is bot-safe only.
- Blocked area: `task +get-my-tasks` and `task tasks list` did not return the workflow-created user task deterministically in UAT, so they are left uncovered instead of being counted from flaky list visibility.
- Blocked area: the remaining user-oriented shortcuts still need deterministic user-owned fixtures or collaborator fixtures beyond the self-owned task created inside the testcase.
- Gap pattern: direct `tasks create/delete/list/patch`, `tasklists create/delete/list/patch`, `members *`, and `subtasks *` APIs still lack deterministic direct-call workflows, so shortcut coverage does not count for those leaf commands.

## Command Table

| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✕ | task +assign | shortcut |  | none | requires real assignee open_id fixtures; shortcut defaults to `--as user` |
| ✓ | task +comment | shortcut | task_comment_workflow_test.go::TestTask_CommentWorkflow/comment | `--task-id`; `--content` | |
| ✓ | task +complete | shortcut | task_status_workflow_test.go::TestTask_StatusWorkflow/complete | `--task-id` | |
| ✓ | task +create | shortcut | task_status_workflow_test.go::TestTask_StatusWorkflow; task_comment_workflow_test.go::TestTask_CommentWorkflow; task_reminder_workflow_test.go::TestTask_ReminderWorkflow; tasklist_add_task_workflow_test.go::TestTask_TasklistAddTaskWorkflow | `summary` + `description`; `due.timestamp` + `due.is_all_day` | |
| ✕ | task +followers | shortcut |  | none | requires real follower open_id fixtures; shortcut defaults to `--as user` |
| ✕ | task +get-my-tasks | shortcut |  | none | UAT did not return the workflow-created user task deterministically in my-tasks views |
| ✓ | task +reminder | shortcut | task_reminder_workflow_test.go::TestTask_ReminderWorkflow/set reminder; task_reminder_workflow_test.go::TestTask_ReminderWorkflow/remove reminder | `--task-id --set 30m`; `--task-id --remove` | |
| ✓ | task +reopen | shortcut | task_status_workflow_test.go::TestTask_StatusWorkflow/reopen | `--task-id` | |
| ✓ | task +tasklist-create | shortcut | tasklist_workflow_test.go::TestTask_TasklistWorkflowAsBot/create tasklist with task as bot; tasklist_workflow_test.go::TestTask_TasklistWorkflowAsUser/create tasklist as user; tasklist_add_task_workflow_test.go::TestTask_TasklistAddTaskWorkflow | `--name` only; `--name` plus task array in `--data` | |
| ✕ | task +tasklist-members | shortcut |  | none | requires real member open_id fixtures to add, remove, or set tasklist members |
| ✓ | task +tasklist-task-add | shortcut | tasklist_add_task_workflow_test.go::TestTask_TasklistAddTaskWorkflow/add task to tasklist | `--tasklist-id`; `--task-id` | |
| ✓ | task +update | shortcut | task_update_workflow_test.go::TestTask_UpdateWorkflow/update task with shortcut as user | `--task-id`; `--summary`; `--description` | verified by follow-up `task tasks get` |
| ✕ | task members add | api |  | none | requires stable member fixtures and explicit direct API-body assertions |
| ✕ | task members remove | api |  | none | requires stable member fixtures and explicit direct API-body assertions |
| ✕ | task subtasks create | api |  | none | needs a parent-task workflow plus direct subtask payload assertions |
| ✕ | task subtasks list | api |  | none | needs deterministic subtask fixtures created in the same workflow |
| ✕ | task tasklists add_members | api |  | none | requires real member open_id fixtures and direct API coverage |
| ✕ | task tasklists create | api |  | none | only covered indirectly through `task +tasklist-create`; no direct API invocation yet |
| ✕ | task tasklists delete | api |  | none | only exercised in parent cleanup; no testcase asserts delete behavior or post-delete state as the primary proof |
| ✓ | task tasklists get | api | tasklist_workflow_test.go::TestTask_TasklistWorkflowAsBot/get tasklist as bot; tasklist_workflow_test.go::TestTask_TasklistWorkflowAsUser/get patched tasklist as user | `tasklist_guid` in `--params` | |
| ✓ | task tasklists list | api | tasklist_workflow_test.go::TestTask_TasklistWorkflowAsUser/list tasklists and find patched tasklist as user | `page_size` | asserts the workflow-created tasklist appears with patched name |
| ✓ | task tasklists patch | api | tasklist_workflow_test.go::TestTask_TasklistWorkflowAsUser/patch tasklist as user | `tasklist_guid` in `--params`; `name` in `--data` | verified by follow-up `task tasklists get` and `task tasklists list` |
| ✕ | task tasklists remove_members | api |  | none | requires real member open_id fixtures and direct API coverage |
| ✓ | task tasklists tasks | api | tasklist_workflow_test.go::TestTask_TasklistWorkflowAsBot/list tasklist tasks as bot; tasklist_add_task_workflow_test.go::TestTask_TasklistAddTaskWorkflow/list tasklist tasks | `tasklist_guid`; `page_size` | |
| ✕ | task tasks create | api |  | none | only covered indirectly through `task +create`; no direct API invocation yet |
| ✕ | task tasks delete | api |  | none | only exercised in parent cleanup; no testcase asserts delete behavior or post-delete state as the primary proof |
| ✓ | task tasks get | api | task_status_workflow_test.go::TestTask_StatusWorkflow/get completed task; task_status_workflow_test.go::TestTask_StatusWorkflow/get reopened task; task_reminder_workflow_test.go::TestTask_ReminderWorkflow/get task with reminder; task_reminder_workflow_test.go::TestTask_ReminderWorkflow/get task without reminder; tasklist_workflow_test.go::TestTask_TasklistWorkflowAsBot/get task as bot; tasklist_add_task_workflow_test.go::TestTask_TasklistAddTaskWorkflow/get task with tasklist link; task_update_workflow_test.go::TestTask_UpdateWorkflow/get created task as user; task_update_workflow_test.go::TestTask_UpdateWorkflow/get task updated by shortcut as user; task_update_workflow_test.go::TestTask_UpdateWorkflow/get task patched by api as user; task_update_workflow_test.go::TestTask_UpdateWorkflow/get completed task as user | `task_guid` in `--params`; assert status, reminders, summary, description, and tasklist link | |
| ✕ | task tasks list | api |  | none | UAT did not return the workflow-created user task deterministically in list views |
| ✓ | task tasks patch | api | task_update_workflow_test.go::TestTask_UpdateWorkflow/patch task with api as user | `task_guid` in `--params`; `summary` + `description` in `--data` | verified by follow-up `task tasks get` |
