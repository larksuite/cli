// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Envelope is the standard success response wrapper.
type Envelope struct {
	OK                 bool                   `json:"ok"`
	Identity           string                 `json:"identity,omitempty"`
	DryRun             bool                   `json:"dry_run,omitempty"`
	Data               interface{}            `json:"data,omitempty"`
	Meta               *Meta                  `json:"meta,omitempty"`
	ContentSafetyAlert interface{}            `json:"_content_safety_alert,omitempty"`
	Notice             map[string]interface{} `json:"_notice,omitempty"`
}

// Meta carries optional metadata in envelope responses.
type Meta struct {
	Count      int             `json:"count,omitempty"`
	Rollback   string          `json:"rollback,omitempty"`
	Pagination *PaginationMeta `json:"pagination,omitempty"`
}

// PaginationMeta reports how a paginated read ended.
//
// It lives in the envelope's meta rather than in the business data because a
// stop reason is not part of the resource: writing it into data both pollutes
// the payload and forces the caller to tell an API field apart from one the CLI
// synthesised. Complete plus NextToken is the whole story — a run either
// exhausted the endpoint or stopped at --page-limit with somewhere to resume —
// so there is no separate stop_reason string to keep in sync.
type PaginationMeta struct {
	Complete  bool   `json:"complete"`
	Pages     int    `json:"pages"`
	Items     int    `json:"items"`
	NextToken string `json:"next_token,omitempty"`
}

// PendingNotice, if set, returns system-level notices to inject as the
// "_notice" field in JSON output envelopes. Set by cmd/root.go.
// Returns nil when there is nothing to report.
var PendingNotice func() map[string]interface{}

// GetNotice returns the current pending notice for struct-based callers.
// Returns nil when there is nothing to report.
func GetNotice() map[string]interface{} {
	if PendingNotice == nil {
		return nil
	}
	return PendingNotice()
}
