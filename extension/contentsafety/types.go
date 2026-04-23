// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"io"
)

// Provider scans parsed response data for content-safety issues.
// Implementations must be safe for concurrent use.
type Provider interface {
	Name() string
	Scan(ctx context.Context, req ScanRequest) (*Alert, error)
}

// ScanRequest carries the data to scan.
type ScanRequest struct {
	Path   string    // normalized command path (e.g. "im.messages_search")
	Data   any       // parsed response data (generic JSON shape)
	ErrOut io.Writer // stderr for provider-level notices (e.g. lazy-config creation)
}

// Alert holds the result of a content-safety scan that detected issues.
type Alert struct {
	Provider     string   `json:"provider"`
	MatchedRules []string `json:"matched_rules"`
}
