// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// taskCodeMeta holds the task-service-specific Lark code classifications.
// 1470403 permission_denied is CategoryAuthorization (exit 3); the other task
// codes route to CategoryAPI / CategoryValidation. BuildAPIError consumes this
// map via mergeCodeMeta + LookupCodeMeta.
var taskCodeMeta = map[int]CodeMeta{
	1470400: {errs.CategoryValidation, errs.SubtypeTaskInvalidParams, false},
	1470403: {errs.CategoryAuthorization, errs.SubtypeTaskPermissionDenied, false}, // permission_denied
	1470404: {errs.CategoryAPI, errs.SubtypeTaskNotFound, false},
	1470422: {errs.CategoryAPI, errs.SubtypeTaskConflict, true},
	1470500: {errs.CategoryAPI, errs.SubtypeTaskServerError, true},
	1470610: {errs.CategoryAPI, errs.SubtypeTaskAssigneeLimit, false},
	1470611: {errs.CategoryAPI, errs.SubtypeTaskFollowerLimit, false},
	1470612: {errs.CategoryAPI, errs.SubtypeTaskTasklistMemberLimit, false},
	1470613: {errs.CategoryAPI, errs.SubtypeTaskReminderExists, false},
}

func init() { mergeCodeMeta(taskCodeMeta, "task") }
