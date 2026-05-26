// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Service-specific Subtype declarations. Per-service files follow the
// naming pattern subtypes_service_<name>.go so the framework's closed
// Subtype enum stays readable while service taxonomies remain visible.

// Task service subtypes — consumed by internal/errclass/codemeta_task.go.
const (
	SubtypeTaskInvalidParams       Subtype = "task_invalid_params"
	SubtypeTaskPermissionDenied    Subtype = "task_permission_denied"
	SubtypeTaskNotFound            Subtype = "task_not_found"
	SubtypeTaskConflict            Subtype = "task_conflict"
	SubtypeTaskServerError         Subtype = "task_server_error"
	SubtypeTaskAssigneeLimit       Subtype = "task_assignee_limit"
	SubtypeTaskFollowerLimit       Subtype = "task_follower_limit"
	SubtypeTaskTasklistMemberLimit Subtype = "task_tasklist_member_limit"
	SubtypeTaskReminderExists      Subtype = "task_reminder_exists"
)
