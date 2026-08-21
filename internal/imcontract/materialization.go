// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

// MaterializationStatus records the IM-only search-to-detail reconciliation.
// RequestedIDs and ResolvedIDs are internal evidence and are never serialized;
// only missing requested IDs may be exposed for targeted recovery.
type MaterializationStatus struct {
	RequestedIDs           []string `json:"-"`
	ResolvedIDs            []string `json:"-"`
	MissingMessageIDs      []string
	UnresolvedHitCount     int
	UnexpectedMessageCount int
	Cause                  error `json:"-"`
}

func (s MaterializationStatus) complete() bool {
	return s.Cause == nil &&
		len(s.MissingMessageIDs) == 0 &&
		s.UnresolvedHitCount == 0 &&
		s.UnexpectedMessageCount == 0 &&
		len(s.RequestedIDs) == len(s.ResolvedIDs)
}

func (s MaterializationStatus) ledger() map[string]any {
	status := "partial"
	if s.complete() {
		status = "complete"
	}
	missing := append([]string(nil), s.MissingMessageIDs...)
	if missing == nil {
		missing = []string{}
	}
	return map[string]any{
		"status":                   status,
		"requested_count":          len(s.RequestedIDs),
		"resolved_count":           len(s.ResolvedIDs),
		"missing_message_ids":      missing,
		"unresolved_hit_count":     s.UnresolvedHitCount,
		"unexpected_message_count": s.UnexpectedMessageCount,
	}
}
