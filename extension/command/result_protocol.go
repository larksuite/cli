// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// ValidateHostResult checks one erased Execute result against the protocol the
// framework depends on. Both lark-cli's host adapter and commandtest call it,
// so a Result the real CLI refuses can no longer pass a business command's own
// tests -- the divergence that mattered was a zero-value Result reading as a
// successful call in commandtest while every real invocation failed.
func ValidateHostResult(definition HostDefinition, result HostResult) error {
	if err := validateHostOutcome(result.Outcome); err != nil {
		return err
	}
	declaresPagination := definition.Output.Meta.Pagination || definition.PageOutput
	return validateHostPagination(declaresPagination, result.Pagination)
}

// validateHostOutcome rejects any outcome the framework did not produce. The
// empty outcome is the case worth naming: it means Execute returned Result{}
// instead of going through Success, which is indistinguishable from a real
// result everywhere except here.
func validateHostOutcome(outcome string) error {
	switch outcome {
	case string(outcomeSuccess):
		return nil
	case "":
		return InternalErrorf("business Execute returned a Result without an outcome; return command.Success(data), or a non-nil error")
	default:
		return InternalErrorf("business Execute returned unsupported outcome %q", outcome)
	}
}

// validateHostPagination mirrors the receipt checks the host applies to
// pagination metadata, so a Page command cannot report a state the framework
// would reject when it renders the envelope.
func validateHostPagination(declared bool, pagination *HostPagination) error {
	if pagination == nil {
		return nil
	}
	if !declared {
		return InternalErrorf("business Execute returned pagination metadata for a command that declares no Page output")
	}
	if pagination.Pages < 1 {
		return InternalErrorf("business Execute returned pagination pages %d, want at least 1", pagination.Pages)
	}
	if pagination.Items < 0 {
		return InternalErrorf("business Execute returned negative pagination items %d", pagination.Items)
	}
	if pagination.Complete && pagination.NextToken != "" {
		return InternalErrorf("business Execute returned a complete page carrying a next token")
	}
	if !pagination.Complete && pagination.NextToken == "" {
		return InternalErrorf("business Execute returned an incomplete page without a next token")
	}
	return nil
}
