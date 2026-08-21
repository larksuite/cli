// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import "github.com/larksuite/cli/errs"

// NormalizeHTTPError makes HTTP status authoritative for contract-managed IM
// responses. It prevents a JSON body with code 0 or an unknown business code
// from hiding an HTTP failure. Non-IM callers do not opt into this behavior.
func NormalizeHTTPError(status int, logID string, err error) error {
	if status < 400 {
		return err
	}
	if status >= 500 {
		normalized := errs.NewNetworkError(
			errs.SubtypeNetworkServer,
			"HTTP %d server error",
			status,
		).WithCode(status).WithRetryable()
		if logID != "" {
			normalized.WithLogID(logID)
		}
		return normalized
	}
	if status == 429 {
		normalized := errs.NewAPIError(errs.SubtypeRateLimit, "HTTP 429 rate limit").WithCode(status)
		if logID != "" {
			normalized.WithLogID(logID)
		}
		return normalized
	}
	if err != nil {
		return err
	}
	subtype := errs.SubtypeUnknown
	if status == 404 {
		subtype = errs.SubtypeNotFound
	}
	normalized := errs.NewAPIError(subtype, "HTTP %d request failed", status).WithCode(status)
	if logID != "" {
		normalized.WithLogID(logID)
	}
	return normalized
}
