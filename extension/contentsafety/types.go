// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import "context"

// Provider scans parsed response data for content-safety issues.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Name returns a stable provider identifier. Used in Alert payloads
	// and diagnostic output.
	Name() string

	// Scan inspects req.Data and returns a non-nil Alert when any issue
	// is detected, or nil when the data is clean.
	//
	// Returning a non-nil error signals that the scan itself failed
	// (misconfiguration, transient I/O, internal panic). Callers are
	// expected to treat scan errors as fail-open.
	//
	// Scan must respect ctx cancellation and return promptly once
	// ctx.Err() becomes non-nil; callers may impose a deadline.
	Scan(ctx context.Context, req ScanRequest) (*Alert, error)
}

// ScanRequest carries the data to be scanned plus minimal context.
type ScanRequest struct {
	// CmdPath is the normalized command path (e.g. "im.messages_search").
	// Providers may use it for per-command logic; most can ignore it.
	CmdPath string

	// Data is the parsed response payload as it flows through the CLI's
	// output layer. It may be a map, slice, string, or a typed struct
	// depending on the originating command. Providers must not mutate it.
	// Providers that require a uniform shape should perform their own
	// normalization internally.
	Data any
}

// Alert describes content-safety issues discovered by a Provider.
// An Alert only exists when at least one issue was found; nil means clean.
type Alert struct {
	// Provider identifies which provider produced this alert.
	Provider string `json:"provider"`

	// Matches is the list of issues detected. Guaranteed non-empty
	// when the enclosing *Alert is non-nil.
	Matches []RuleMatch `json:"matches"`
}

// RuleMatch describes a single rule hit.
type RuleMatch struct {
	// Rule is the stable identifier of the matched rule
	// (e.g. "instruction_override", "role_injection").
	Rule string `json:"rule"`
}
