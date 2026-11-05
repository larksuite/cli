// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Problem is the RFC 7807-aligned shared shape embedded by every typed error.
//
// Message is REQUIRED. Producers must populate it; an empty Message will make
// Error() return "" — a known Go footgun for fmt.Errorf("...: %v", err).
//
// Wire-format notes:
//   - No Component field. Service / shortcut component is metric-only
//     enrichment derived by the dispatcher from the cobra command path; it
//     never appears on the wire.
//   - No DocURL field. PermissionError carries the same intent via its typed
//     ConsoleURL extension; other typed errors do not link out.
//   - Troubleshooter is the upstream Lark API's diagnostic URL (resp.error.
//     troubleshooter). Carried universally so any classified error can surface
//     it; populated by errclass.BuildAPIError when the upstream response
//     includes it, otherwise absent.
//   - Retryable uses omitempty so only `true` is emitted; consumers treat
//     absence as false.
type Problem struct {
	Category       Category `json:"type"`
	Subtype        Subtype  `json:"subtype,omitempty"`
	Code           int      `json:"code,omitempty"`
	Message        string   `json:"message"`
	Hint           string   `json:"hint,omitempty"`
	LogID          string   `json:"log_id,omitempty"`
	Troubleshooter string   `json:"troubleshooter,omitempty"`
	Retryable      bool     `json:"retryable,omitempty"`
}

// Error satisfies the standard `error` interface. A nil receiver is treated
// as the empty string so a stray nil *Problem stored in an error interface
// cannot panic the dispatcher.
func (p *Problem) Error() string {
	if p == nil {
		return ""
	}
	return p.Message
}
func (p *Problem) ProblemDetail() *Problem { return p }
