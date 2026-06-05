// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// taskCodeMeta holds task-service Lark code → CodeMeta mappings.
// All Subtypes are framework-shared (errs.SubtypeXxx) — task does not declare
// service-specific Subtypes because none of these codes carry semantics beyond
// the cross-service taxonomy (NotFound / QuotaExceeded / etc.).
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var taskCodeMeta = map[int]CodeMeta{
	1470400: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},            // invalid_params
	1470403: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},   // permission_denied (resource-level)
	1470404: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                     // not_found
	1470422: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict, Retryable: true},    // conflict (retryable)
	1470500: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // server_error (retryable)
	1470610: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},                // assignee_limit
	1470611: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},                // follower_limit
	1470612: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},                // tasklist_member_limit
	1470613: {Category: errs.CategoryAPI, Subtype: errs.SubtypeAlreadyExists},                // reminder_exists
}

func init() { mergeCodeMeta(taskCodeMeta, "task") }
