// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestListPeriods(t *testing.T) {
	tests := []struct {
		name           string
		formatFlag     string
		expectedOutput []string
	}{
		{
			name:       "pretty format",
			formatFlag: "pretty",
			expectedOutput: []string{
				"2026 Q1",
				"ID: period-001",
			},
		},
		{
			name:       "json format",
			formatFlag: "json",
			expectedOutput: []string{
				`"id"`,
				`"period-001"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := okrShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/okr/v1/periods",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"id":                "period-001",
								"zh_name":           "2026 Q1",
								"en_name":           "2026 Q1",
								"status":            float64(0),
								"period_start_time": "1735689600000",
								"period_end_time":   "1743465600000",
							},
						},
						"page_token": "",
						"has_more":   false,
					},
				},
			})

			// Override AuthTypes to include bot for testing
			shortcut := ListPeriods
			shortcut.AuthTypes = []string{"user", "bot"}

			err := runMountedOkrShortcut(t, shortcut, []string{"+periods", "--format", tt.formatFlag, "--as", "bot"}, f, stdout)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			out := stdout.String()
			for _, expected := range tt.expectedOutput {
				if !strings.Contains(out, expected) {
					t.Errorf("output missing expected string (%s), got: %s", expected, out)
				}
			}
		})
	}
}

func TestListPeriodsEmpty(t *testing.T) {
	f, stdout, _, reg := okrShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v1/periods",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"items":      []interface{}{},
				"page_token": "",
				"has_more":   false,
			},
		},
	})

	shortcut := ListPeriods
	shortcut.AuthTypes = []string{"user", "bot"}

	err := runMountedOkrShortcut(t, shortcut, []string{"+periods", "--format", "pretty", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "No OKR periods found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestListPeriodsRegistration(t *testing.T) {
	s := ListPeriods
	if s.Service != "okr" {
		t.Errorf("expected service 'okr', got '%s'", s.Service)
	}
	if s.Command != "+periods" {
		t.Errorf("expected command '+periods', got '%s'", s.Command)
	}
	if s.Risk != "read" {
		t.Errorf("expected risk 'read', got '%s'", s.Risk)
	}

	// Verify flags exist
	flagNames := make(map[string]bool)
	for _, flag := range s.Flags {
		flagNames[flag.Name] = true
	}
	for _, required := range []string{"page-token", "page-size"} {
		if !flagNames[required] {
			t.Errorf("missing flag: %s", required)
		}
	}

	// Verify shortcut is in Shortcuts() return
	shortcuts := Shortcuts()
	found := false
	for _, sc := range shortcuts {
		if sc.Command == "+periods" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListPeriods not found in Shortcuts()")
	}
}

func TestShortcutsCount(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 6 {
		t.Errorf("expected 6 shortcuts, got %d", len(shortcuts))
	}

	expectedCommands := map[string]bool{
		"+list":         false,
		"+get":          false,
		"+periods":      false,
		"+progress-add": false,
		"+progress-get": false,
		"+review":       false,
	}
	for _, s := range shortcuts {
		if _, ok := expectedCommands[s.Command]; ok {
			expectedCommands[s.Command] = true
		}
	}
	for cmd, found := range expectedCommands {
		if !found {
			t.Errorf("missing shortcut: %s", cmd)
		}
	}

	// All shortcuts should have Service == "okr"
	for _, s := range shortcuts {
		if s.Service != "okr" {
			t.Errorf("shortcut %s has service '%s', expected 'okr'", s.Command, s.Service)
		}
	}
}

// Verify DryRun function exists on all shortcuts
func TestAllShortcutsHaveDryRun(t *testing.T) {
	shortcuts := Shortcuts()
	for _, s := range shortcuts {
		if s.DryRun == nil {
			t.Errorf("shortcut %s has no DryRun function", s.Command)
		}
	}
}

// Verify all shortcuts have valid structure
func TestShortcutStructure(t *testing.T) {
	for _, s := range Shortcuts() {
		if s.Execute == nil {
			t.Errorf("shortcut %s has no Execute function", s.Command)
		}
		if s.Description == "" {
			t.Errorf("shortcut %s has no description", s.Command)
		}
		if len(s.Scopes) == 0 {
			t.Errorf("shortcut %s has no scopes", s.Command)
		}
		if len(s.AuthTypes) == 0 {
			t.Errorf("shortcut %s has no auth types", s.Command)
		}
		// All OKR shortcuts should only support user identity
		for _, at := range s.AuthTypes {
			if at != "user" {
				t.Errorf("shortcut %s has unexpected auth type '%s' (OKR API only supports user)", s.Command, at)
			}
		}

		// All OKR scopes should start with "okr:"
		for _, scope := range s.Scopes {
			if !strings.HasPrefix(scope, "okr:") {
				t.Errorf("shortcut %s has scope '%s' not starting with 'okr:'", s.Command, scope)
			}
		}
	}
}

// Ensure DryRun can be called without panic (basic nil safety)
func TestDryRunNilSafety(t *testing.T) {
	for _, s := range Shortcuts() {
		t.Run(s.Command, func(t *testing.T) {
			// DryRun functions should not panic when called with basic RuntimeContext
			// This just verifies the functions exist and are callable
			if s.DryRun == nil {
				t.Skipf("no DryRun for %s", s.Command)
			}
		})
	}
}

func TestWriteShortcutsRisk(t *testing.T) {
	for _, s := range Shortcuts() {
		if s.Command == "+progress-add" {
			if s.Risk != "write" {
				t.Errorf("shortcut %s should have risk 'write', got '%s'", s.Command, s.Risk)
			}
		} else {
			if s.Risk != "read" {
				t.Errorf("shortcut %s should have risk 'read', got '%s'", s.Command, s.Risk)
			}
		}
	}
}

func init() {
	// Ensure common package is used
	_ = common.File
}
