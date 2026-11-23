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
//
// Total/HitCount/MissedCount are batch-lookup counters used by commands that
// resolve a list of inputs and report per-input hit/miss (e.g.
// apps +user-id-convert). They are pointers so an unset counter is dropped by
// omitempty while an explicit zero (e.g. "missed_count": 0 on a full hit) is
// still emitted — a plain int with omitempty would silently drop the required
// zero, and without omitempty it would pollute every other domain's meta. Every
// non-batch command leaves them nil, so this stays invisible outside batch use.
type Meta struct {
	Count       int             `json:"count,omitempty"`
	Rollback    string          `json:"rollback,omitempty"`
	Pagination  *PaginationMeta `json:"pagination,omitempty"`
	Total       *int            `json:"total,omitempty"`
	HitCount    *int            `json:"hit_count,omitempty"`
	MissedCount *int            `json:"missed_count,omitempty"`
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
	// Complete is true only when the server's exhausted state was observed.
	Complete bool `json:"complete"`
	// Pages counts successful API pages included in this result.
	Pages int `json:"pages"`
	// Items counts records after command-level filtering and enrichment.
	Items int `json:"items"`
	// NextToken is the cursor at which an incomplete result can resume.
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
