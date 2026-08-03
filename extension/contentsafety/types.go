// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"io"
)

// Provider scans parsed response data for content-safety issues.
// Implementations must be safe for concurrent use. Scan may be a best-effort
// scan with bounded string length or nesting depth.
type Provider interface {
	Name() string
	Scan(ctx context.Context, req ScanRequest) (*Alert, error)
}

// FullTextProvider is a Provider that guarantees a complete scan of Data with
// NO per-string or depth truncation. Block mode requires this capability so a
// match anywhere in the output cannot slip past a truncation boundary.
type FullTextProvider interface {
	Provider
	ScanFullText(ctx context.Context, req ScanRequest) (*Alert, error)
}

// ScanRequest carries the data to scan.
type ScanRequest struct {
	Path   string    // normalized command path (e.g. "im.messages_search")
	Data   any       // parsed response data (generic JSON shape)
	ErrOut io.Writer // stderr for provider-level notices (e.g. lazy-config creation)
	// FullText marks Data as one complete rendered-output string. It remains a
	// compatibility hint for Provider.Scan; block mode calls
	// FullTextProvider.ScanFullText to enforce complete scanning.
	FullText bool
}

// Alert holds the result of a content-safety scan that detected issues.
type Alert struct {
	Provider     string   `json:"provider"`
	MatchedRules []string `json:"matched_rules"`
}
