// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

// TaskUpdateUserAccessV2Data is the Task v2 update event payload under the
// standard Lark V2 event envelope.
type TaskUpdateUserAccessV2Data struct {
	EventTypes []string `json:"event_types,omitempty" desc:"Task commit types included in this event" enum:"task_create,task_deleted,task_summary_update,task_desc_update,task_assignees_update,task_followers_update,task_reminders_update,task_start_due_update,task_completed_update"`
	TaskGUID   string   `json:"task_guid,omitempty" desc:"Task GUID that changed" kind:"task_guid"`
}

var taskUpdateUserAccessCommitTypes = []string{
	"task_create",
	"task_deleted",
	"task_summary_update",
	"task_desc_update",
	"task_assignees_update",
	"task_followers_update",
	"task_reminders_update",
	"task_start_due_update",
	"task_completed_update",
}
