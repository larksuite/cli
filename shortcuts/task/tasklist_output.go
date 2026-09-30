// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

type tasklistOutputField string

const tasklistOutputMembers tasklistOutputField = "members"

var standardTasklistOutputFields = []tasklistOutputField{
	tasklistOutputMembers,
}

func projectTasklistFields(dst, tasklist map[string]interface{}, fields ...tasklistOutputField) {
	for _, field := range fields {
		key := string(field)
		if value, ok := tasklist[key]; ok {
			dst[key] = value
		}
	}
}
