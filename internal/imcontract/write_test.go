// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestRequiredResult(t *testing.T) {
	c, _ := Lookup("im +messages-send")
	for _, data := range []map[string]any{{}, {"message_id": ""}} {
		s := NewSession(c)
		_, err := s.FinalizeSuccess(data)
		if err == nil {
			t.Fatalf("expected missing result error for %#v", data)
		}
		p, _ := errs.ProblemOf(err)
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("problem = %#v", p)
		}
		if output.ExitCodeOf(err) != output.ExitInternal {
			t.Fatalf("exit = %d", output.ExitCodeOf(err))
		}
	}
	s := NewSession(c)
	got, err := s.FinalizeSuccess(map[string]any{"message_id": "om_x"})
	if err != nil || !got.OK {
		t.Fatalf("valid result rejected: %#v %v", got, err)
	}
}

func TestBatchPartialLedger(t *testing.T) {
	c, _ := Lookup("im messages urgent_app")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"user_id_list": []any{"ou_a", "ou_b"}})
	got, err := s.FinalizeSuccess(map[string]any{"invalid_user_id_list": []any{"ou_b"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.OK || got.ExitCode != output.ExitAPI {
		t.Fatalf("result = %#v", got)
	}
	completion := got.Data.(map[string]any)["completion"].(Completion)
	if completion.Status != "partial" || completion.SucceededCount != 1 || completion.FailedCount != 1 {
		t.Fatalf("completion = %#v", completion)
	}
	if len(completion.FailedItems) != 1 || completion.FailedItems[0] != "ou_b" {
		t.Fatalf("failed items = %#v", completion.FailedItems)
	}
}

func TestBatchPendingIsNotCountedAsSucceeded(t *testing.T) {
	c, _ := Lookup("im chat.members create")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"id_list": []any{"ou_a", "ou_b"}})
	got, err := s.FinalizeSuccess(map[string]any{"pending_approval_id_list": []any{"ou_b"}})
	if err != nil {
		t.Fatal(err)
	}
	completion := got.Data.(map[string]any)["completion"].(Completion)
	if completion.SucceededCount != 1 || completion.PendingCount != 1 || completion.RetryScope != "none" {
		t.Fatalf("completion = %#v", completion)
	}
}

func TestResponsePendingCannotExpandRequestedLedger(t *testing.T) {
	c, _ := Lookup("im chat.members create")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{
		"id_list": []any{"ou_a", "ou_b"},
	})
	got, err := s.FinalizeSuccess(map[string]any{
		"pending_approval_id_list": []any{"ou_unknown"},
	})
	if err == nil {
		t.Fatalf("unknown response pending was accepted: %#v", got)
	}
	assertUnsafeEvidenceError(t, err)
}

func TestSyntheticFlagPendingExpandsLogicalRequest(t *testing.T) {
	c, _ := Lookup("im +flag-cancel")
	s := NewSession(c)
	s.RecordFact(Fact{Kind: FactFlagFeedLayerPending})
	got, err := s.FinalizeSuccess(map[string]any{"results": []any{
		map[string]any{"flag_type": "message", "status": "ok"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	completion := got.Data.(map[string]any)["completion"].(Completion)
	if completion.RequestedCount != 2 || completion.SucceededCount != 1 ||
		completion.FailedCount != 0 || completion.PendingCount != 1 ||
		len(completion.PendingItems) != 1 || completion.PendingItems[0] != "feed" {
		t.Fatalf("synthetic pending did not expand logical request: %#v", completion)
	}
}

func TestRequiredResultBatchPartialPrioritizesLedger(t *testing.T) {
	c, _ := Lookup("im messages merge_forward")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"message_id_list": []any{"om_a", "om_b"}})
	got, err := s.FinalizeSuccess(map[string]any{"invalid_message_id_list": []any{"om_b"}})
	if err != nil || got.OK || got.ExitCode != output.ExitAPI {
		t.Fatalf("partial result = %#v, err=%v", got, err)
	}

	s = NewSession(c)
	s.ObserveRequest(map[string]any{"message_id_list": []any{"om_a"}})
	_, err = s.FinalizeSuccess(map[string]any{})
	if err == nil {
		t.Fatal("missing merged message_id must fail when no partial result exists")
	}
}

func TestManagerResponseSetAssertions(t *testing.T) {
	for _, tc := range []struct {
		key      ContractKey
		response map[string]any
		wantOK   bool
	}{
		{"im chat.managers add_managers", map[string]any{"chat_managers": []any{"ou_a"}}, true},
		{"im chat.managers add_managers", map[string]any{"chat_managers": []any{}}, false},
		{"im chat.managers delete_managers", map[string]any{"chat_managers": []any{}}, true},
		{"im chat.managers delete_managers", map[string]any{"chat_managers": []any{"ou_a"}}, false},
	} {
		c, _ := Lookup(tc.key)
		s := NewSession(c)
		s.ObserveRequest(map[string]any{"manager_ids": []any{"ou_a"}})
		got, err := s.FinalizeSuccess(tc.response)
		if err != nil || got.OK != tc.wantOK {
			t.Errorf("%s response=%v: got %#v, err=%v", tc.key, tc.response, got, err)
		}
	}
}

func TestManagerResponseSetAssertionsRequirePresentEvidence(t *testing.T) {
	for _, key := range []ContractKey{
		"im chat.managers add_managers",
		"im chat.managers delete_managers",
	} {
		t.Run(string(key), func(t *testing.T) {
			c, _ := Lookup(key)
			s := NewSession(c)
			s.ObserveRequest(map[string]any{"manager_ids": []any{"ou_a"}})
			got, err := s.FinalizeSuccess(map[string]any{})
			if err == nil {
				t.Fatalf("missing response sets were accepted: %#v", got)
			}
			assertUnsafeEvidenceError(t, err)
		})
	}
}

func TestModerationAcceptedUnverified(t *testing.T) {
	c, _ := Lookup("im chat.moderation update")
	got, err := NewSession(c).FinalizeSuccess(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	completion := got.Data.(map[string]any)["completion"].(map[string]any)
	if completion["status"] != "accepted_unverified" || completion["final_state_verified"] != false {
		t.Fatalf("completion = %#v", completion)
	}
	if got.Hint != HelpAcceptanceOnly.Text() {
		t.Fatalf("hint = %q", got.Hint)
	}
}

func TestReplaySafety(t *testing.T) {
	unknown := errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithHint("untrusted upstream hint")
	c, _ := Lookup("im +messages-send")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"uuid": "stable-key"})
	s.RecordFact(Fact{Kind: FactWriteAttempted})
	got := s.FinalizeError(unknown)
	p, _ := errs.ProblemOf(got)
	if !p.Retryable || p.Hint != hintSameKey {
		t.Fatalf("same-key problem = %#v", p)
	}

	unknown = errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithHint("untrusted upstream hint")
	s = NewSession(c)
	s.ObserveRequest(map[string]any{"uuid": "stable-key"})
	s.RecordFact(Fact{Kind: FactWriteAttempted})
	s.RecordFact(Fact{Kind: FactMediaPreuploadPerformed})
	got = s.FinalizeError(unknown)
	p, _ = errs.ProblemOf(got)
	if p.Retryable || p.Hint != hintReplayForbidden {
		t.Fatalf("preupload problem = %#v", p)
	}

	validation := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad flag")
	got = NewSession(c).FinalizeError(validation)
	p, _ = errs.ProblemOf(got)
	if p.Retryable || p.Hint != "" {
		t.Fatalf("validation problem was broadened: %#v", p)
	}

	unknown = errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithHint("untrusted upstream hint")
	c, _ = Lookup("im +feed-shortcut-create")
	s = NewSession(c)
	s.RecordFact(Fact{Kind: FactWriteAttempted})
	got = s.FinalizeError(unknown)
	p, _ = errs.ProblemOf(got)
	if !p.Retryable || p.Hint != hintReplaySafe {
		t.Fatalf("safe replay problem = %#v", p)
	}

	preflight := errs.NewNetworkError(errs.SubtypeNetworkTransport, "lookup failed").
		WithRetryable().
		WithHint("specify --item-type explicitly")
	c, _ = Lookup("im +flag-create")
	got = NewSession(c).FinalizeError(preflight)
	p, _ = errs.ProblemOf(got)
	if !p.Retryable || p.Hint != "specify --item-type explicitly" {
		t.Fatalf("preflight problem was rewritten: %#v", p)
	}
}

func TestWriteRateLimitNeverAuthorizesReplay(t *testing.T) {
	for _, key := range []ContractKey{
		"im +feed-shortcut-create",
		"im +messages-send",
	} {
		t.Run(string(key), func(t *testing.T) {
			contract, _ := Lookup(key)
			session := NewSession(contract)
			session.ObserveRequest(map[string]any{"uuid": "stable-key"})
			session.RecordFact(Fact{Kind: FactWriteAttempted})
			rateLimit := errs.NewAPIError(errs.SubtypeRateLimit, "too many requests").
				WithRetryable().
				WithHint("retry later")

			got := session.FinalizeError(rateLimit)
			problem, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("FinalizeError returned untyped error %T: %v", got, got)
			}
			if problem.Retryable || problem.Hint != "" {
				t.Fatalf("rate limit authorized replay for %s: %#v", key, problem)
			}
		})
	}
}

func TestBatchPartialRecoveryMatrix(t *testing.T) {
	tests := []struct {
		name      string
		command   ContractKey
		request   map[string]any
		response  map[string]any
		fact      *Fact
		wantScope string
	}{
		{
			name:    "pending always forbids retry",
			command: "im +flag-cancel",
			response: map[string]any{"results": []any{
				map[string]any{"flag_type": "message", "status": "ok"},
			}},
			fact:      &Fact{Kind: FactFlagFeedLayerPending},
			wantScope: "none",
		},
		{
			name:    "whole request recovery",
			command: "im +feed-shortcut-create",
			request: map[string]any{"shortcuts": []any{
				map[string]any{"feed_card_id": "oc_a"},
			}},
			response: map[string]any{"failed_shortcuts": []any{
				map[string]any{"shortcut": map[string]any{"feed_card_id": "oc_a"}},
			}},
			wantScope: "whole_request",
		},
		{
			name:      "failed items only recovery",
			command:   "im messages urgent_app",
			request:   map[string]any{"user_id_list": []any{"ou_a", "ou_b"}},
			response:  map[string]any{"invalid_user_id_list": []any{"ou_b"}},
			wantScope: "failed_items_only",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contract, _ := Lookup(tc.command)
			session := NewSession(contract)
			if tc.request != nil {
				if err := session.ObserveRequest(tc.request); err != nil {
					t.Fatal(err)
				}
			}
			if tc.fact != nil {
				session.RecordFact(*tc.fact)
			}
			result, err := session.FinalizeSuccess(tc.response)
			if err != nil {
				t.Fatal(err)
			}
			completion := result.Data.(map[string]any)["completion"].(Completion)
			if completion.RetryScope != tc.wantScope || result.Hint != "" {
				t.Fatalf("completion=%#v hint=%q", completion, result.Hint)
			}
		})
	}
}

func TestBatchRejectsUnmappableFailureEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		command  ContractKey
		request  map[string]any
		response map[string]any
	}{
		{
			name:     "all IDs missing",
			command:  "im chat.members create",
			request:  map[string]any{"id_list": []any{"ou_a"}},
			response: map[string]any{"invalid_id_list": []any{map[string]any{"reason": "bad"}}},
		},
		{
			name:    "one ID missing",
			command: "im chat.members create",
			request: map[string]any{"id_list": []any{"ou_a", "ou_b"}},
			response: map[string]any{"invalid_id_list": []any{
				"ou_a", map[string]any{"reason": "bad"},
			}},
		},
		{
			name:     "stable ID outside request",
			command:  "im chat.members create",
			request:  map[string]any{"id_list": []any{"ou_a"}},
			response: map[string]any{"invalid_id_list": []any{"ou_unknown"}},
		},
		{
			name:    "compound feed ID missing",
			command: "im feed.groups batch_add_item",
			request: map[string]any{"items": []any{
				map[string]any{"feed_id": "oc_a", "feed_type": "chat"},
			}},
			response: map[string]any{"failed_items": []any{
				map[string]any{"item": map[string]any{"feed_type": "chat"}},
			}},
		},
		{
			name:    "compound feed type missing",
			command: "im feed.groups batch_add_item",
			request: map[string]any{"items": []any{
				map[string]any{"feed_id": "oc_a", "feed_type": "chat"},
			}},
			response: map[string]any{"failed_items": []any{
				map[string]any{"item": map[string]any{"feed_id": "oc_a"}},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := Lookup(tc.command)
			s := NewSession(c)
			s.ObserveRequest(tc.request)
			got, err := s.FinalizeSuccess(tc.response)
			if err == nil {
				t.Fatalf("unmappable response was accepted: %#v", got)
			}
			assertUnsafeEvidenceError(t, err)
		})
	}
}

func TestAssertionRejectsUnmappableResponseEvidence(t *testing.T) {
	c, _ := Lookup("im chat.managers add_managers")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"manager_ids": []any{"ou_a"}})
	got, err := s.FinalizeSuccess(map[string]any{
		"chat_managers": []any{map[string]any{"name": "missing ID"}},
	})
	if err == nil {
		t.Fatalf("unmappable assertion response was accepted: %#v", got)
	}
	assertUnsafeEvidenceError(t, err)
}

func TestRequestEvidenceFailsClosedOnUnsupportedShapes(t *testing.T) {
	c, _ := Lookup("im chat.members create")
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "non-map body reaches contract as nil", body: nil},
		{name: "missing collection", body: map[string]any{}},
		{name: "wrong collection type", body: map[string]any{"id_list": []string{"ou_a"}}},
		{name: "unmappable item", body: map[string]any{"id_list": []any{map[int]any{1: "ou_a"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := NewSession(c).ObserveRequest(tc.body)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation ||
				problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("request evidence error = %#v, ok=%v", problem, ok)
			}
		})
	}
}

func TestExtractionAccounting(t *testing.T) {
	got := extract(map[string]any{
		"ids": []any{"ou_a", map[string]any{"missing": "id"}, "ou_a"},
	}, stringsFrom("ids"))
	if !got.present || got.rawCount != 3 || got.selectedCount != 2 ||
		got.rejectedCount != 1 || len(got.items) != 1 {
		t.Fatalf("extraction = %#v", got)
	}

	got = extract(map[string]any{"ids": []string{"ou_a"}}, stringsFrom("ids"))
	if !got.present || got.rawCount != 0 || got.selectedCount != 0 ||
		got.rejectedCount != 1 || len(got.items) != 0 {
		t.Fatalf("wrong-shape extraction = %#v", got)
	}
}

func TestStatusLedgerRejectsUnknownStatus(t *testing.T) {
	c, _ := Lookup("im +flag-cancel")
	got, err := NewSession(c).FinalizeSuccess(map[string]any{"results": []any{
		map[string]any{"flag_type": "message", "status": "maybe"},
	}})
	if err == nil {
		t.Fatalf("unknown result status was accepted: %#v", got)
	}
	assertUnsafeEvidenceError(t, err)
}

func TestUnsafeEvidenceRemainsForbiddenAcrossFinalizeError(t *testing.T) {
	c, _ := Lookup("im +feed-shortcut-create")
	s := NewSession(c)
	if err := s.ObserveRequest(map[string]any{"shortcuts": []any{
		map[string]any{"feed_card_id": "oc_a"},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err := s.FinalizeSuccess(map[string]any{"failed_shortcuts": []any{
		map[string]any{"shortcut": map[string]any{"missing": "feed_card_id"}},
	}})
	if err == nil {
		t.Fatal("malformed evidence was accepted")
	}
	for i := 0; i < 2; i++ {
		err = s.FinalizeError(err)
		assertUnsafeEvidenceError(t, err)
	}
}

func assertUnsafeEvidenceError(t *testing.T, err error) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal ||
		problem.Subtype != errs.SubtypeInvalidResponse ||
		problem.Retryable || problem.Hint != hintUnsafeEvidence {
		t.Fatalf("unsafe evidence error = %#v, ok=%v", problem, ok)
	}
	if output.ExitCodeOf(err) != output.ExitInternal {
		t.Fatalf("unsafe evidence exit = %d", output.ExitCodeOf(err))
	}
}

func TestLedgerSelectorDoesNotCopySecrets(t *testing.T) {
	c, _ := Lookup("im chat.members create")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{
		"id_list":         []any{"ou_a"},
		"content":         "secret body",
		"phone":           "123",
		"idempotency_key": "secret-key",
		"access_token":    "token",
		"next_page_token": "page",
	})
	got, err := s.FinalizeSuccess(map[string]any{"invalid_id_list": []any{"ou_a"}})
	if err != nil {
		t.Fatal(err)
	}
	completion := got.Data.(map[string]any)["completion"].(Completion)
	if len(completion.FailedItems) != 1 || completion.FailedItems[0] != "ou_a" {
		t.Fatalf("completion leaked or lost selector: %#v", completion)
	}
}

func TestFeedLedgerKeepsOnlyRetryableIdentityFields(t *testing.T) {
	c, _ := Lookup("im feed.groups batch_add_item")
	s := NewSession(c)
	s.ObserveRequest(map[string]any{"items": []any{
		map[string]any{"feed_id": "oc_a", "feed_type": "chat", "content": "secret"},
	}})
	got, err := s.FinalizeSuccess(map[string]any{"failed_items": []any{
		map[string]any{"item": map[string]any{"feed_id": "oc_a", "feed_type": "chat"}, "error_message": "server text"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	item := got.Data.(map[string]any)["completion"].(Completion).FailedItems[0].(map[string]any)
	if len(item) != 2 || item["feed_id"] != "oc_a" || item["feed_type"] != "chat" {
		t.Fatalf("failed item = %#v", item)
	}
}

func TestCompletionIsClosedOverRequestedItems(t *testing.T) {
	simple := func(id string) ledgerItem { return ledgerItem{key: id, value: id} }
	compound := func(feedType, feedID string) ledgerItem {
		return ledgerItem{
			key: feedType + "\x00" + feedID,
			value: map[string]any{
				"feed_id": feedID, "feed_type": feedType,
			},
		}
	}
	for _, tc := range []struct {
		name      string
		requested []ledgerItem
		failed    []ledgerItem
		pending   []ledgerItem
	}{
		{
			name:      "single IDs",
			requested: []ledgerItem{simple("a"), simple("b"), simple("c"), simple("a")},
			failed:    []ledgerItem{simple("b"), simple("c"), simple("c"), simple("unknown")},
			pending:   []ledgerItem{simple("b"), simple("b"), simple("pending-unknown")},
		},
		{
			name: "compound IDs",
			requested: []ledgerItem{
				compound("chat", "oc_a"), compound("doc", "doc_b"), compound("chat", "oc_a"),
			},
			failed: []ledgerItem{
				compound("chat", "oc_a"), compound("chat", "oc_a"), compound("chat", "oc_unknown"),
				compound("doc", "doc_b"),
			},
			pending: []ledgerItem{
				compound("doc", "doc_b"), compound("doc", "doc_b"), compound("doc", "doc_unknown"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := completion(tc.requested, tc.failed, tc.pending, PartialRecoveryFailedItemsOnly)
			if got.RequestedCount != got.SucceededCount+got.FailedCount+got.PendingCount {
				t.Fatalf("non-exclusive counts: %#v", got)
			}
			if got.FailedCount != 1 || got.PendingCount != 1 {
				t.Fatalf("failed/pending overlap was not resolved: %#v", got)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "unknown") {
				t.Fatalf("unrequested response item entered retry ledger: %s", raw)
			}
		})
	}
}

func TestWriteSessionUnknownStrategyFailsClosed(t *testing.T) {
	session := NewSession(Contract{
		Key:      "im future write",
		Strategy: Strategy{Kind: StrategyKind("future_write")},
	})
	_, err := session.FinalizeSuccess(map[string]any{"accepted": true})
	if err == nil || !errs.IsInternal(err) {
		t.Fatalf("expected typed internal error, got %v", err)
	}
}
