// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "github.com/larksuite/cli/errs"

// validateTypedResultProtocol validates the only V1 runner-owned receipt:
// pagination metadata. Partial outcomes, count metadata, and artifact-schema
// receipts had no public producer and deliberately do not exist in this model.
func validateTypedResultProtocol(command *compiledCommand, result compiledResult) error {
	if result.outcome != typedOutcomeSuccess {
		return nil
	}
	return validateTypedResultMeta(command.output.Meta, result.meta)
}

func validateTypedResultMeta(definition typedResultMetaDefinition, meta *typedResultMeta) error {
	if meta == nil {
		return nil
	}
	if meta.Pagination == nil {
		return resultProtocolError("typed Result Meta is empty")
	}
	if !definition.Pagination {
		return resultProtocolError("typed Result returned undeclared meta.pagination")
	}
	pagination := meta.Pagination
	if pagination.Pages < 1 {
		return resultProtocolError("typed Result meta.pagination.pages must be at least 1")
	}
	if pagination.Items < 0 {
		return resultProtocolError("typed Result meta.pagination.items must be non-negative")
	}
	if pagination.Complete && pagination.NextToken != "" {
		return resultProtocolError("typed Result complete pagination must not include next_token")
	}
	if !pagination.Complete && pagination.NextToken == "" {
		return resultProtocolError("typed Result incomplete pagination must include next_token")
	}
	return nil
}

func resultProtocolError(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeUnknown, format, args...)
}
