// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestApplicationAffordanceExamples(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	tests := []struct {
		method  string
		command string
	}{
		{method: "+slash-command-list", command: "lark-cli application +slash-command-list"},
		{method: "+slash-command-create", command: `lark-cli application +slash-command-create --command greet --description "say hi" --description-i18n zh_cn=问候`},
		{method: "+slash-command-update", command: `lark-cli application +slash-command-update --command greet --description "new text"`},
		{method: "+slash-command-delete", command: "lark-cli application +slash-command-delete --command greet --yes"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			raw, ok := For("application", tt.method)
			if !ok {
				t.Fatalf("For(application, %s) ok=false", tt.method)
			}
			a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatalf("application %s affordance did not parse", tt.method)
			}
			if len(a.Examples) != 1 || a.Examples[0].Command != tt.command {
				t.Fatalf("examples = %#v, want one command %q", a.Examples, tt.command)
			}
			if strings.Contains(a.Examples[0].Command, "--as ") {
				t.Fatalf("dual-identity application example must not pin an identity: %q", a.Examples[0].Command)
			}
			if len(a.Tips) == 0 {
				t.Fatal("migration must keep operational guidance in structured Tips")
			}
		})
	}
}

func TestApplicationAffordanceDecisionAndSafetyGuidance(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	create := parsedApplicationAffordance(t, "+slash-command-create")
	if !containsItem(create.AvoidWhen, "already exists") || !containsItem(create.AvoidWhen, "+slash-command-update") {
		t.Fatalf("create guidance must route ordinary edits to update: %v", create.AvoidWhen)
	}

	update := parsedApplicationAffordance(t, "+slash-command-update")
	if !containsItem(update.Prerequisites, "--command-id") || !containsItem(update.Prerequisites, "live list request") {
		t.Fatalf("update prerequisites must explain id and by-name target paths: %v", update.Prerequisites)
	}

	deleteCommand := parsedApplicationAffordance(t, "+slash-command-delete")
	if !containsItem(deleteCommand.Prerequisites, "Explicit user confirmation") ||
		!containsItem(deleteCommand.Prerequisites, "must not self-approve") {
		t.Fatalf("delete prerequisites must preserve the confirmation boundary: %v", deleteCommand.Prerequisites)
	}
}

func TestApplicationAffordanceDoesNotDuplicateRuntimeRecovery(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for _, method := range []string{
		"+slash-command-list",
		"+slash-command-create",
		"+slash-command-update",
		"+slash-command-delete",
	} {
		raw, ok := For("application", method)
		if !ok {
			t.Fatalf("For(application, %s) ok=false", method)
		}
		for _, forbidden := range []string{"Permissions and recovery", "auth login", "console_url", "missing_scopes"} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s affordance duplicates runtime recovery %q: %s", method, forbidden, raw)
			}
		}
	}
}

func parsedApplicationAffordance(t *testing.T, method string) meta.Affordance {
	t.Helper()
	raw, ok := For("application", method)
	if !ok {
		t.Fatalf("For(application, %s) ok=false", method)
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatalf("application %s affordance did not parse", method)
	}
	return a
}

func containsItem(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
