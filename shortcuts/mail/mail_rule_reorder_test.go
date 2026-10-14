// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// reorderRuleIDs algorithm unit tests
// ---------------------------------------------------------------------------

func TestReorderRuleIDs_Adjust2Rules(t *testing.T) {
	// Current: D,A,G,C,B,E → User: E,A → Result: D,E,G,C,B,A
	result, err := reorderRuleIDs([]string{"E", "A"}, []string{"D", "A", "G", "C", "B", "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"D", "E", "G", "C", "B", "A"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestReorderRuleIDs_Adjust4Rules(t *testing.T) {
	// Current: D,A,G,C,B,E → User: D,B,E,A → Result: D,G,C,B,E,A
	result, err := reorderRuleIDs([]string{"D", "B", "E", "A"}, []string{"D", "A", "G", "C", "B", "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"D", "G", "C", "B", "E", "A"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestReorderRuleIDs_FullList(t *testing.T) {
	// Current: D,A,G,C,B,E → User: D,A,G,C,B,E → Result: D,A,G,C,B,E
	current := []string{"D", "A", "G", "C", "B", "E"}
	result, err := reorderRuleIDs(current, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sliceEqual(result, current) {
		t.Errorf("got %v, want %v", result, current)
	}
}

func TestReorderRuleIDs_SingleRule(t *testing.T) {
	// Current: D,A,G,C,B,E → User: E → Result: D,A,G,C,B,E (single anchor, no reordering effect)
	result, err := reorderRuleIDs([]string{"E"}, []string{"D", "A", "G", "C", "B", "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"D", "A", "G", "C", "B", "E"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestReorderRuleIDs_Last2Rules(t *testing.T) {
	// Current: D,A,G,C,B,E → User: B,E → Result: D,A,G,C,B,E
	result, err := reorderRuleIDs([]string{"B", "E"}, []string{"D", "A", "G", "C", "B", "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"D", "A", "G", "C", "B", "E"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestReorderRuleIDs_FirstAndLast(t *testing.T) {
	// Current: D,A,G,C,B,E → User: D,E → Result: D,A,G,C,B,E
	result, err := reorderRuleIDs([]string{"D", "E"}, []string{"D", "A", "G", "C", "B", "E"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"D", "A", "G", "C", "B", "E"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

func TestReorderRuleIDs_UserIDNotFound(t *testing.T) {
	_, err := reorderRuleIDs([]string{"X"}, []string{"D", "A", "G", "C", "B", "E"})
	if err == nil {
		t.Fatal("expected error for non-existent rule ID, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestReorderRuleIDs_EmptyCurrent(t *testing.T) {
	_, err := reorderRuleIDs([]string{"A"}, []string{})
	if err == nil {
		t.Fatal("expected error for non-existent rule ID with empty current, got nil")
	}
}

func TestReorderRuleIDs_TwoRulesSwap(t *testing.T) {
	// Simple swap: Current: A,B → User: B,A → Result: B,A
	result, err := reorderRuleIDs([]string{"B", "A"}, []string{"A", "B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"B", "A"}
	if !sliceEqual(result, want) {
		t.Errorf("got %v, want %v", result, want)
	}
}

// ---------------------------------------------------------------------------
// parseRuleIDs tests
// ---------------------------------------------------------------------------

func TestParseRuleIDs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"A,B,C", []string{"A", "B", "C"}},
		{" A , B , C ", []string{"A", "B", "C"}},
		{"single", []string{"single"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := parseRuleIDs(tt.input)
		if !sliceEqual(got, tt.want) {
			t.Errorf("parseRuleIDs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// extractRuleIDs tests
// ---------------------------------------------------------------------------

func TestExtractRuleIDs(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": "rule1"},
			map[string]interface{}{"id": "rule2"},
			map[string]interface{}{"id": "rule3"},
		},
	}
	got := extractRuleIDs(data)
	want := []string{"rule1", "rule2", "rule3"}
	if !sliceEqual(got, want) {
		t.Errorf("extractRuleIDs() = %v, want %v", got, want)
	}
}

func TestExtractRuleIDs_NilData(t *testing.T) {
	got := extractRuleIDs(nil)
	if got != nil {
		t.Errorf("extractRuleIDs(nil) = %v, want nil", got)
	}
}

func TestExtractRuleIDs_EmptyItems(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{},
	}
	got := extractRuleIDs(data)
	if len(got) != 0 {
		t.Errorf("extractRuleIDs() = %v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Shortcut metadata tests
// ---------------------------------------------------------------------------

func TestMailRuleReorder_ShortcutMetadata(t *testing.T) {
	if MailRuleReorder.Service != "mail" {
		t.Errorf("Service = %q, want %q", MailRuleReorder.Service, "mail")
	}
	if MailRuleReorder.Command != "+rule-reorder" {
		t.Errorf("Command = %q, want %q", MailRuleReorder.Command, "+rule-reorder")
	}
	if MailRuleReorder.Risk != "write" {
		t.Errorf("Risk = %q, want %q", MailRuleReorder.Risk, "write")
	}
	required := map[string]bool{
		"mail:user_mailbox.rule:read":  true,
		"mail:user_mailbox.rule:write": true,
	}
	for _, s := range MailRuleReorder.Scopes {
		delete(required, s)
	}
	if len(required) != 0 {
		t.Errorf("MailRuleReorder.Scopes missing %v", required)
	}
	authSet := map[string]bool{"user": true, "tenant": true}
	for _, a := range MailRuleReorder.AuthTypes {
		delete(authSet, a)
	}
	if len(authSet) != 0 {
		t.Errorf("MailRuleReorder.AuthTypes missing %v", authSet)
	}
	if !MailRuleReorder.HasFormat {
		t.Error("HasFormat should be true")
	}
}

// ---------------------------------------------------------------------------
// Validate callback tests
// ---------------------------------------------------------------------------

func TestMailRuleReorder_Validate_EmptyRuleIDs(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{"rule-ids": ""})
	err := MailRuleReorder.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for empty --rule-ids")
	}
	if !strings.Contains(err.Error(), "--rule-ids is required") {
		t.Errorf("error should mention '--rule-ids is required', got: %v", err)
	}
}

func TestMailRuleReorder_Validate_DuplicateIDs(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{"rule-ids": "A,A,B"})
	err := MailRuleReorder.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for duplicate rule IDs")
	}
	if !strings.Contains(err.Error(), "duplicate rule ID") {
		t.Errorf("error should mention 'duplicate rule ID', got: %v", err)
	}
}

func TestMailRuleReorder_Validate_EmptyIDInList(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{"rule-ids": "A,,B"})
	err := MailRuleReorder.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for empty ID in list")
	}
	if !strings.Contains(err.Error(), "empty ID") {
		t.Errorf("error should mention 'empty ID', got: %v", err)
	}
}

func TestMailRuleReorder_Validate_ValidInput(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{"rule-ids": "A,B,C"})
	err := MailRuleReorder.Validate(context.Background(), runtime)
	if err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DryRun callback tests
// ---------------------------------------------------------------------------

func TestMailRuleReorder_DryRun(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{
		"rule-ids": "E,A",
	})

	dry := MailRuleReorder.DryRun(context.Background(), runtime)
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	s := string(raw)

	for _, want := range []string{
		`"method":"GET"`,
		`/user_mailboxes/me/rules`,
		`"method":"POST"`,
		`/user_mailboxes/me/rules/reorder`,
		`"rule_ids"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry-run JSON missing %q; got:\n%s", want, s)
		}
	}
}

func TestMailRuleReorder_DryRun_CustomMailbox(t *testing.T) {
	runtime := runtimeForRuleReorder(t, map[string]string{
		"rule-ids": "E,A",
		"mailbox":  "shared@example.com",
	})

	dry := MailRuleReorder.DryRun(context.Background(), runtime)
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	s := string(raw)

	if !strings.Contains(s, "shared@example.com") {
		t.Errorf("dry-run JSON should contain mailbox in path; got:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
// HTTP mock integration tests
// ---------------------------------------------------------------------------

func TestMailRuleReorder_Execute_Success(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Mock: List rules
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "D"},
					map[string]interface{}{"id": "A"},
					map[string]interface{}{"id": "G"},
					map[string]interface{}{"id": "C"},
					map[string]interface{}{"id": "B"},
					map[string]interface{}{"id": "E"},
				},
			},
		},
	})

	// Mock: Reorder rules
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/rules/reorder",
		Body: map[string]interface{}{
			"code": 0,
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "E,A",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	ruleIDs, ok := data["rule_ids"].([]interface{})
	if !ok {
		t.Fatalf("expected rule_ids array, got %T", data["rule_ids"])
	}
	want := []string{"D", "E", "G", "C", "B", "A"}
	if len(ruleIDs) != len(want) {
		t.Fatalf("got %d rule IDs, want %d", len(ruleIDs), len(want))
	}
	for i, id := range ruleIDs {
		s, _ := id.(string)
		if s != want[i] {
			t.Errorf("rule_ids[%d] = %q, want %q", i, s, want[i])
		}
	}
}

func TestMailRuleReorder_Execute_RuleIDNotFound(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Mock: List rules
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "A"},
					map[string]interface{}{"id": "B"},
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "X",
	}, f, stdout)

	if err == nil {
		t.Fatal("expected error for non-existent rule ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestMailRuleReorder_Execute_ListAPIError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Mock: List rules fails
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 99991400,
			"msg":  "permission denied",
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder",
		"--rule-ids", "A",
	}, f, stdout)

	if err == nil {
		t.Fatal("expected error for list API failure")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func runtimeForRuleReorder(t *testing.T, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range MailRuleReorder.Flags {
		cmd.Flags().String(fl.Name, "", "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
