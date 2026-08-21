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

func TestOutputFallbackBuildsCompletionByAllowlist(t *testing.T) {
	const secret = "SECRET_MARKER"
	tests := []struct {
		name       string
		result     Result
		wantStatus string
		wantScope  string
		wantCounts bool
		wantFinal  bool
	}{
		{
			name: "completed required result",
			result: Result{OK: true, Data: map[string]any{
				"message_id": secret,
			}},
			wantStatus: "complete",
			wantScope:  "none",
		},
		{
			name: "batch partial",
			result: Result{Data: map[string]any{
				"completion": Completion{
					Status:         "partial",
					RequestedCount: 2,
					SucceededCount: 1,
					FailedCount:    1,
					FailedItems:    []any{secret},
					RetryScope:     "failed_items_only",
				},
			}},
			wantStatus: "partial",
			wantScope:  "failed_items_only",
			wantCounts: true,
		},
		{
			name: "accepted unverified",
			result: Result{OK: true, Data: map[string]any{
				"completion": map[string]any{
					"status":               "accepted_unverified",
					"final_state_verified": false,
					"retry_scope":          "none",
					"message":              secret,
				},
			}},
			wantStatus: "accepted_unverified",
			wantScope:  "none",
			wantFinal:  true,
		},
		{
			name: "mention partial",
			result: Result{Data: map[string]any{
				"mention_result": MessageMentionResult{
					Status:                "partial_unattributed",
					Requested:             []string{secret},
					Confirmed:             []MessageMentionConfirmation{},
					Missing:               []string{},
					UnattributedRequested: []string{secret},
					All:                   "not_requested",
					RetryScope:            "none",
				},
			}},
			wantStatus: "partial_unattributed",
			wantScope:  "none",
		},
		{
			name: "unknown recovery values are not trusted",
			result: Result{OK: true, Data: map[string]any{
				"completion": map[string]any{
					"status":      secret,
					"retry_scope": secret,
				},
			}},
			wantStatus: "complete",
			wantScope:  "none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, signal := BuildJQOutputFallback(tc.result)
			if output.ExitCodeOf(signal) != output.ExitAPI {
				t.Fatalf("exit = %d", output.ExitCodeOf(signal))
			}
			raw, err := json.Marshal(env)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), secret) {
				t.Fatalf("fallback leaked payload: %s", raw)
			}
			data := env.Data.(map[string]any)
			if len(data) != 1 {
				t.Fatalf("data = %#v", data)
			}
			completion := data["completion"].(map[string]any)
			if completion["status"] != tc.wantStatus || completion["retry_scope"] != tc.wantScope {
				t.Fatalf("completion = %#v", completion)
			}
			_, hasCounts := completion["requested_count"]
			if hasCounts != tc.wantCounts {
				t.Fatalf("completion counts presence = %v, want %v: %#v", hasCounts, tc.wantCounts, completion)
			}
			_, hasFinal := completion["final_state_verified"]
			if hasFinal != tc.wantFinal {
				t.Fatalf("final state presence = %v, want %v: %#v", hasFinal, tc.wantFinal, completion)
			}
			for _, forbidden := range []string{"succeeded_items", "failed_items", "pending_items", "message"} {
				if _, exists := completion[forbidden]; exists {
					t.Fatalf("completion copied %s: %#v", forbidden, completion)
				}
			}
		})
	}
}

func TestContentSafetyOutputFallbackUsesFixedPublicProblem(t *testing.T) {
	env, signal := BuildContentSafetyOutputFallback(Result{Data: map[string]any{}})
	if output.ExitCodeOf(signal) != output.ExitContentSafety {
		t.Fatalf("exit = %d", output.ExitCodeOf(signal))
	}
	problem := env.Error.(*errs.Problem)
	if problem.Category != errs.CategoryPolicy || problem.Subtype != errs.SubtypeContentSafety ||
		problem.Message != "Output blocked after the IM write completed" {
		t.Fatalf("problem = %#v", problem)
	}
}
