// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

type taskOutputField string

const (
	taskOutputSummary taskOutputField = "summary"
	taskOutputMembers taskOutputField = "members"
	taskOutputStart   taskOutputField = "start"
	taskOutputDue     taskOutputField = "due"
	taskOutputStatus  taskOutputField = "status"
)

var standardTaskOutputFields = []taskOutputField{
	taskOutputSummary,
	taskOutputMembers,
	taskOutputStart,
	taskOutputDue,
	taskOutputStatus,
}

func projectTaskFields(dst, task map[string]interface{}, fields ...taskOutputField) {
	for _, field := range fields {
		key := string(field)
		if value, ok := task[key]; ok {
			dst[key] = value
		}
	}
}
