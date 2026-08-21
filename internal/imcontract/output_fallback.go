// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// BuildJQOutputFallback returns the self-contained result emitted when jq
// presentation fails after an IM write has already been finalized.
func BuildJQOutputFallback(result Result) (output.Envelope, error) {
	problem := errs.NewAPIError(
		errs.SubtypeUnknown,
		"Output failed after the IM write completed",
	)
	return buildOutputFallback(result, &problem.Problem), output.PartialFailure(output.ExitAPI)
}

// BuildContentSafetyOutputFallback returns the self-contained result emitted
// when content-safety blocks presentation after an IM write has already been
// finalized.
func BuildContentSafetyOutputFallback(result Result) (output.Envelope, error) {
	problem := errs.NewContentSafetyError(
		errs.SubtypeContentSafety,
		"Output blocked after the IM write completed",
	)
	return buildOutputFallback(result, &problem.Problem), output.PartialFailure(output.ExitContentSafety)
}

func buildOutputFallback(result Result, problem *errs.Problem) output.Envelope {
	return output.Envelope{
		OK: false,
		Data: map[string]any{
			"completion": allowlistedCompletion(result.Data),
		},
		Error: problem,
	}
}

func allowlistedCompletion(data any) map[string]any {
	summary := map[string]any{
		"status":      "complete",
		"retry_scope": "none",
	}
	root, ok := data.(map[string]any)
	if !ok {
		return summary
	}
	completionValue, hasCompletion := root["completion"]
	switch completion := completionValue.(type) {
	case Completion:
		copyCompletionStatus(summary, completion.Status)
		summary["requested_count"] = completion.RequestedCount
		summary["succeeded_count"] = completion.SucceededCount
		summary["failed_count"] = completion.FailedCount
		summary["pending_count"] = completion.PendingCount
		copyCompletionRetryScope(summary, completion.RetryScope)
		return summary
	case map[string]any:
		if value, ok := completion["status"].(string); ok {
			copyCompletionStatus(summary, value)
		}
		copyCompletionCount(summary, completion, "requested_count")
		copyCompletionCount(summary, completion, "succeeded_count")
		copyCompletionCount(summary, completion, "failed_count")
		copyCompletionCount(summary, completion, "pending_count")
		if value, exists := completion["final_state_verified"]; exists {
			if verified, valid := value.(bool); valid {
				summary["final_state_verified"] = verified
			}
		}
		if value, ok := completion["retry_scope"].(string); ok {
			copyCompletionRetryScope(summary, value)
		}
	}
	if !hasCompletion {
		if mention, ok := root["mention_result"].(MessageMentionResult); ok {
			copyCompletionStatus(summary, mention.Status)
			copyCompletionRetryScope(summary, mention.RetryScope)
		}
	}
	return summary
}

func copyCompletionStatus(dst map[string]any, value string) {
	switch value {
	case "complete", "partial", "accepted_unverified", "partial_unattributed":
		dst["status"] = value
	}
}

func copyCompletionRetryScope(dst map[string]any, value string) {
	switch value {
	case "none", "whole_request", "failed_items_only":
		dst["retry_scope"] = value
	}
}

func copyCompletionCount(dst, src map[string]any, key string) {
	switch value := src[key].(type) {
	case int:
		dst[key] = value
	case int32:
		dst[key] = value
	case int64:
		dst[key] = value
	case uint:
		dst[key] = value
	case uint32:
		dst[key] = value
	case uint64:
		dst[key] = value
	case float64:
		dst[key] = value
	}
}
