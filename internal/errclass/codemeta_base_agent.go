// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// baseAgentCodeMeta holds Backend Core Base Agent / AI Ware errno codes that can
// surface through Base Agent OpenAPI endpoints. The response message still comes
// from the upstream "msg"; this table only classifies the numeric code.
var baseAgentCodeMeta = map[int]CodeMeta{
	800004000: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound}, // ErrNotFound

	800004011: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // ErrPermNotAllow, e.g. CLI AI entrance disabled
	800004780: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // ErrNoPermissionException

	800004135: {Category: errs.CategoryAPI, Subtype: errs.SubtypeRateLimit, Retryable: true}, // ErrRateLimit
	800004907: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict},                   // ErrAIInBusy, e.g. context already has a running task
	800007006: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict},                   // ErrAgentTurnFinished

	800004921: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // ErrAIQuotaLimit
	800004936: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // ErrAIWareTooManyTask
	800004961: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // ErrFeishuAITenantCreditZero
	800004962: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // ErrFeishuAICreditExhausted
	800004991: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded}, // ErrAIDecisionAgentAIQuotaLimit

	800004906: {Category: errs.CategoryPolicy, Subtype: errs.SubtypeContentSafety}, // ErrAITnsNotPass

	800004932: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrAIWareJobTimeout
	800004934: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrAIWareTaskTimeout
	800004946: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrAIWareStageTimeout
	800004988: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrAgentAbnormalStop
	800005009: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrCacheGet
	800008006: {Category: errs.CategoryAPI, Subtype: errs.SubtypeServerError, Retryable: true}, // ErrInternalServer

	800006002: {Category: errs.CategoryInternal, Subtype: errs.SubtypeInvalidResponse}, // ErrMarshal
	800006003: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},    // ErrUnmarshal
	800009999: {Category: errs.CategoryInternal, Subtype: errs.SubtypeSDKError},        // ErrImplementMe / dependency not initialized

	800004953: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound}, // ErrAIBaseAgentConversationDeleted
	800004989: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound}, // ErrAIBaseAgentConversationNotFound
}

func init() { mergeCodeMeta(baseAgentCodeMeta, "base_agent") }
