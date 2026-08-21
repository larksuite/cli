// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
)

func TestReadCompletenessMatrix(t *testing.T) {
	apiErr := errs.NewAPIError(errs.SubtypeServerError, "later page failed")
	networkErr := errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithRetryable()
	invalidErr := errs.NewInternalError(errs.SubtypeInvalidResponse, "bad pagination")
	tests := []struct {
		name       string
		fullRead   bool
		status     client.PaginationStatus
		wantOK     bool
		wantDone   bool
		wantExit   int
		wantReason client.StopReason
		wantError  bool
		wantHint   string
	}{
		{"single exhausted", false, client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonExhausted}, true, true, 0, client.StopReasonExhausted, false, ""},
		{"single has more", false, client.PaginationStatus{PagesFetched: 1, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonSinglePage}, true, false, 0, client.StopReasonSinglePage, false, "Result is incomplete. Re-run with --page-all --page-limit 0 when exhaustive output is required."},
		{"all exhausted", true, client.PaginationStatus{PagesFetched: 2, StopReason: client.StopReasonExhausted}, true, true, 0, client.StopReasonExhausted, false, ""},
		{"page limit", true, client.PaginationStatus{PagesFetched: 2, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonPageLimit}, true, false, 0, client.StopReasonPageLimit, false, "Result is incomplete because --page-limit was reached. Use --page-limit 0 only when exhaustive output is required."},
		{"start token", false, client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonStartPageToken}, true, false, 0, client.StopReasonStartPageToken, false, hintStartPage},
		{"api error", true, client.PaginationStatus{PagesFetched: 1, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonAPIError, Cause: apiErr}, false, false, output.ExitAPI, client.StopReasonAPIError, true, "The read is incomplete. Retry the read; do not infer that missing items do not exist."},
		{"transport error", true, client.PaginationStatus{PagesFetched: 1, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonTransportError, Cause: networkErr}, false, false, output.ExitNetwork, client.StopReasonTransportError, true, "The read is incomplete. Retry the read; do not infer that missing items do not exist."},
		{"missing token", true, client.PaginationStatus{PagesFetched: 1, HasMore: true, StopReason: client.StopReasonMissingToken, Cause: invalidErr}, false, false, output.ExitInternal, client.StopReasonMissingToken, true, "The server did not provide a usable next page token. Report the result as incomplete."},
		{"repeated token", true, client.PaginationStatus{PagesFetched: 2, HasMore: true, StopReason: client.StopReasonRepeatedToken, Cause: invalidErr}, false, false, output.ExitInternal, client.StopReasonRepeatedToken, true, "The server did not provide a usable next page token. Report the result as incomplete."},
		{"single truncation", false, client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonServerTruncation}, true, false, 0, client.StopReasonServerTruncation, false, "The server truncated the result. Narrow the query range before retrying."},
		{"full truncation", true, client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonServerTruncation}, false, false, output.ExitAPI, client.StopReasonServerTruncation, false, "The server truncated the result. Narrow the query range before retrying."},
	}
	contract := mustReadContract(t, "im +chat-list")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewReadSession(contract, ReadOptions{FullRead: tt.fullRead})
			if err != nil {
				t.Fatal(err)
			}
			session.ObservePagination(tt.status)
			got, err := session.Finalize(map[string]any{"items": []any{"a"}})
			if err != nil {
				t.Fatal(err)
			}
			if got.OK != tt.wantOK || got.ExitCode != tt.wantExit {
				t.Fatalf("result OK/exit = %v/%d, want %v/%d", got.OK, got.ExitCode, tt.wantOK, tt.wantExit)
			}
			if got.Meta == nil || got.Meta.Pagination == nil || got.Meta.Pagination.Complete != tt.wantDone {
				t.Fatalf("complete = %#v, want %v", got.Meta, tt.wantDone)
			}
			readNotice, _ := got.Notice["im_read"].(map[string]interface{})
			if readNotice["stop_reason"] != string(tt.wantReason) {
				t.Fatalf("stop reason = %#v, want %q", readNotice["stop_reason"], tt.wantReason)
			}
			if (got.Error != nil) != tt.wantError {
				t.Fatalf("error present = %v, want %v", got.Error != nil, tt.wantError)
			}
			if got.Hint != tt.wantHint {
				t.Fatalf("hint = %q, want %q", got.Hint, tt.wantHint)
			}
		})
	}
}

func TestIncompleteReadRemovesContradictoryPageFields(t *testing.T) {
	contract := mustReadContract(t, "im +chat-list")
	for _, stopReason := range []client.StopReason{
		client.StopReasonStartPageToken,
		client.StopReasonServerTruncation,
	} {
		t.Run(string(stopReason), func(t *testing.T) {
			session, err := NewReadSession(contract, ReadOptions{FullRead: false})
			if err != nil {
				t.Fatal(err)
			}
			session.ObservePagination(client.PaginationStatus{
				PagesFetched: 1,
				StopReason:   stopReason,
			})
			input := map[string]any{
				"items":           []any{"kept"},
				"has_more":        false,
				"page_token":      "",
				"next_page_token": "",
			}
			result, err := session.Finalize(input)
			if err != nil {
				t.Fatal(err)
			}
			data := result.Data.(map[string]any)
			for _, field := range []string{"has_more", "page_token", "next_page_token"} {
				if _, exists := data[field]; exists {
					t.Fatalf("incomplete result retained page-relative field %q: %#v", field, data)
				}
				if _, exists := input[field]; !exists {
					t.Fatalf("Finalize mutated caller data while removing %q: %#v", field, input)
				}
			}
			if result.Meta == nil || result.Meta.Pagination == nil || result.Meta.Pagination.Complete {
				t.Fatalf("pagination = %#v, want canonical incomplete result", result.Meta)
			}
		})
	}
}

func TestCanonicalReadDataPreservesCompatiblePageFields(t *testing.T) {
	contract := mustReadContract(t, "im +chat-list")
	for _, tt := range []struct {
		name   string
		status client.PaginationStatus
		data   map[string]any
	}{
		{
			name:   "incomplete with resume cursor",
			status: client.PaginationStatus{PagesFetched: 1, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonPageLimit},
			data:   map[string]any{"items": []any{"kept"}, "has_more": true, "page_token": "next"},
		},
		{
			name:   "exhausted from collection start",
			status: client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonExhausted},
			data:   map[string]any{"items": []any{"kept"}, "has_more": false, "page_token": ""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewReadSession(contract, ReadOptions{FullRead: true})
			if err != nil {
				t.Fatal(err)
			}
			session.ObservePagination(tt.status)
			result, err := session.Finalize(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			data := result.Data.(map[string]any)
			for _, field := range []string{"has_more", "page_token"} {
				if _, exists := data[field]; !exists {
					t.Fatalf("compatible result removed %q: %#v", field, data)
				}
			}
		})
	}
}

func TestReadFailureErrorWireShapeDoesNotSerializeCause(t *testing.T) {
	contract := mustReadContract(t, "im +chat-list")
	session, err := NewReadSession(contract, ReadOptions{FullRead: true})
	if err != nil {
		t.Fatal(err)
	}
	secret := "raw-server-cause-must-not-leak"
	cause := errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").
		WithRetryable().
		WithCause(assertionError(secret))
	session.ObservePagination(client.PaginationStatus{
		PagesFetched:  1,
		HasMore:       true,
		NextPageToken: "opaque-token",
		StopReason:    client.StopReasonTransportError,
		Cause:         cause,
	})
	result, err := session.Finalize(map[string]any{"items": []any{"kept"}})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(result.Error)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) == "" || containsAny(string(wire), secret, "opaque-token") {
		t.Fatalf("unsafe error wire: %s", wire)
	}
}

func TestReadFinalizeErrorRetryMatrix(t *testing.T) {
	contract := mustReadContract(t, "im +messages-mget")
	tests := []struct {
		name          string
		err           error
		wantRetryable bool
	}{
		{
			name:          "transport",
			err:           errs.NewNetworkError(errs.SubtypeNetworkTransport, "connection reset"),
			wantRetryable: true,
		},
		{
			name:          "server error",
			err:           errs.NewAPIError(errs.SubtypeServerError, "upstream failed"),
			wantRetryable: true,
		},
		{
			name:          "rate limit is not authorized",
			err:           errs.NewAPIError(errs.SubtypeRateLimit, "too many requests").WithRetryable(),
			wantRetryable: false,
		},
		{
			name:          "permission",
			err:           errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope"),
			wantRetryable: false,
		},
		{
			name:          "not found",
			err:           errs.NewAPIError(errs.SubtypeNotFound, "missing"),
			wantRetryable: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewReadSession(contract, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got := session.FinalizeError(tt.err)
			problem, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("FinalizeError returned untyped error %T: %v", got, got)
			}
			if problem.Retryable != tt.wantRetryable {
				t.Fatalf("Retryable = %v, want %v: %#v", problem.Retryable, tt.wantRetryable, problem)
			}
		})
	}
}

func TestPagedReadNormalizesRateLimitToNonRetryable(t *testing.T) {
	contract := mustReadContract(t, "im +chat-list")
	session, err := NewReadSession(contract, ReadOptions{FullRead: true})
	if err != nil {
		t.Fatal(err)
	}
	rateLimit := errs.NewAPIError(errs.SubtypeRateLimit, "too many requests").WithRetryable()
	session.ObservePagination(client.PaginationStatus{
		PagesFetched: 1,
		HasMore:      true,
		StopReason:   client.StopReasonAPIError,
		Cause:        rateLimit,
	})
	result, err := session.Finalize(map[string]any{"items": []any{"kept"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Error == nil {
		t.Fatal("expected typed partial read error")
	}
	if result.Error.Retryable {
		t.Fatalf("429/rate_limit must not authorize retry: %#v", result.Error)
	}
}

func TestSearchMaterializationControlsFinalCompleteness(t *testing.T) {
	contract := mustReadContract(t, "im +messages-search")
	tests := []struct {
		name                        string
		status                      MaterializationStatus
		wantOK                      bool
		wantMaterializationComplete bool
		wantHint                    string
	}{
		{
			name: "complete",
			status: MaterializationStatus{
				RequestedIDs: []string{"om_a", "om_b"},
				ResolvedIDs:  []string{"om_a", "om_b"},
			},
			wantOK:                      true,
			wantMaterializationComplete: true,
			wantHint:                    "Results are ready to use. Use message_id/file_key directly; do not call messages-mget.",
		},
		{
			name: "missing details",
			status: MaterializationStatus{
				RequestedIDs:      []string{"om_a", "om_b"},
				ResolvedIDs:       []string{"om_a"},
				MissingMessageIDs: []string{"om_b"},
			},
			wantOK:                      false,
			wantMaterializationComplete: false,
			wantHint:                    "The search is incomplete. Query only materialization.missing_message_ids with im +messages-mget.",
		},
		{
			name: "unresolved hit",
			status: MaterializationStatus{
				RequestedIDs:       []string{"om_a"},
				ResolvedIDs:        []string{"om_a"},
				UnresolvedHitCount: 1,
			},
			wantOK:                      false,
			wantMaterializationComplete: false,
			wantHint:                    "The search is incomplete and cannot be safely recovered by message ID. Narrow the query before retrying.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewReadSession(contract, ReadOptions{FullRead: true})
			if err != nil {
				t.Fatal(err)
			}
			session.ObservePagination(client.PaginationStatus{PagesFetched: 2, StopReason: client.StopReasonExhausted})
			session.ObserveMaterialization(tt.status)
			result, err := session.Finalize(map[string]any{"messages": []any{map[string]any{"message_id": "om_a"}}})
			if err != nil {
				t.Fatal(err)
			}
			if result.OK != tt.wantOK || result.Meta == nil || result.Meta.Pagination == nil {
				t.Fatalf("result = %#v, want OK %v with pagination metadata", result, tt.wantOK)
			}
			if !result.Meta.Pagination.Complete {
				t.Fatalf("materialization must not overwrite exhausted pagination: %#v", result.Meta.Pagination)
			}
			if result.Hint != tt.wantHint {
				t.Fatalf("hint = %q, want %q", result.Hint, tt.wantHint)
			}
			data := result.Data.(map[string]any)
			ledger, ok := data["materialization"].(map[string]any)
			if !ok {
				t.Fatalf("materialization ledger missing: %#v", data)
			}
			wantStatus := "partial"
			if tt.wantMaterializationComplete {
				wantStatus = "complete"
			}
			if ledger["status"] != wantStatus {
				t.Fatalf("materialization status = %q, want %q", ledger["status"], wantStatus)
			}
		})
	}
}

func TestSearchMaterializationRequiredButUnobservedFailsClosed(t *testing.T) {
	contract := mustReadContract(t, "im +messages-search")
	session, err := NewReadSession(contract, ReadOptions{FullRead: true})
	if err != nil {
		t.Fatal(err)
	}
	session.ObservePagination(client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonExhausted})
	_, err = session.Finalize(map[string]any{"messages": []any{}})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want invalid_response", err, err)
	}
}

func TestSearchMaterializationDoesNotOverwritePaginationFailure(t *testing.T) {
	contract := mustReadContract(t, "im +messages-search")
	session, err := NewReadSession(contract, ReadOptions{FullRead: true})
	if err != nil {
		t.Fatal(err)
	}
	pageErr := errs.NewNetworkError(errs.SubtypeNetworkTransport, "later page failed")
	session.ObservePagination(client.PaginationStatus{
		PagesFetched:  1,
		HasMore:       true,
		NextPageToken: "next",
		StopReason:    client.StopReasonTransportError,
		Cause:         pageErr,
	})
	session.ObserveMaterialization(MaterializationStatus{
		RequestedIDs:      []string{"om_a", "om_b"},
		ResolvedIDs:       []string{"om_a"},
		MissingMessageIDs: []string{"om_b"},
	})

	result, err := session.Finalize(map[string]any{"messages": []any{map[string]any{"message_id": "om_a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.ExitCode != output.ExitNetwork || result.Cause != pageErr {
		t.Fatalf("pagination failure was overwritten: %#v", result)
	}
	if result.Error == nil || !result.Error.Retryable {
		t.Fatalf("pagination problem was not preserved: %#v", result.Error)
	}
	for _, want := range []string{hintReadFailed, "materialization.missing_message_ids"} {
		if !strings.Contains(result.Hint, want) {
			t.Fatalf("combined hint = %q, want %q", result.Hint, want)
		}
	}
}

func TestSearchMaterializationDoesNotExposeUnexpectedIDs(t *testing.T) {
	status := MaterializationStatus{
		RequestedIDs:           []string{"om_requested"},
		ResolvedIDs:            []string{"om_requested"},
		UnexpectedMessageCount: 1,
	}
	wire, err := json.Marshal(status.ledger())
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(string(wire), "om_requested", "om_unknown_secret") {
		t.Fatalf("ledger leaked internal IDs: %s", wire)
	}
}

func TestSearchEmptyResultAddsNonExistenceHint(t *testing.T) {
	contract := mustReadContract(t, "im +chat-search")
	session, err := NewReadSession(contract, ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	session.ObservePagination(client.PaginationStatus{PagesFetched: 1, StopReason: client.StopReasonExhausted})
	result, err := session.Finalize(map[string]any{"chats": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Meta == nil || result.Meta.Pagination == nil || !result.Meta.Pagination.Complete {
		t.Fatalf("expected exhausted result to be complete: %#v", result.Meta)
	}
	const wantHint = "The search was exhausted, but an empty search result does not prove that the resource does not exist."
	if result.Hint != wantHint {
		t.Fatalf("hint = %q, want %q", result.Hint, wantHint)
	}
}

func TestEntityAndMaterializeDoNotInventPagination(t *testing.T) {
	for _, key := range []ContractKey{"im chat.nickname get", "im +messages-resources-download"} {
		t.Run(string(key), func(t *testing.T) {
			contract := mustReadContract(t, key)
			session, err := NewReadSession(contract, ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := session.Finalize(map[string]any{"nickname": ""})
			if err != nil {
				t.Fatal(err)
			}
			if !result.OK || result.Meta != nil || result.ExitCode != 0 {
				t.Fatalf("unexpected finite result: %#v", result)
			}
		})
	}
}

func TestUnknownReadStrategyFailsClosed(t *testing.T) {
	_, err := NewReadSession(Contract{
		Key:      "im future read",
		Strategy: Strategy{Kind: StrategyKind("future_read")},
	}, ReadOptions{})
	if err == nil || !errs.IsInternal(err) {
		t.Fatalf("expected typed internal error, got %v", err)
	}
}

func mustReadContract(t *testing.T, key ContractKey) Contract {
	t.Helper()
	contract, ok := Lookup(key)
	if !ok {
		t.Fatalf("missing contract %q", key)
	}
	return contract
}

type assertionError string

func (e assertionError) Error() string { return string(e) }

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if value != "" && stringContains(s, value) {
			return true
		}
	}
	return false
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
