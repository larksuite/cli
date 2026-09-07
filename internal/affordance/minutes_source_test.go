// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestMinutesAffordanceExamples(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	tests := []struct {
		method  string
		command string
	}{
		{
			method:  "+todo",
			command: `lark-cli minutes +todo --minute-token obcnxxxxxxxxxxxxxxxxxxxx --operation add --todo "跟进预算审批" --is-done=false --as user`,
		},
		{
			method:  "+word-replace",
			command: `lark-cli minutes +word-replace --minute-token obcnxxxxxxxxxxxxxxxxxxxx --replace-words '[{"source_word":"旧词","target_word":"新词"},{"source_word":"Foo","target_word":"Bar"}]' --as user`,
		},
		{
			method:  "+summary",
			command: `lark-cli minutes +summary --minute-token obcnxxxxxxxxxxxxxxxxxxxx --summary "**会议结论**\n- 方案 A 通过\n- 下周跟进排期" --as user`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			raw, ok := For("minutes", tt.method)
			if !ok {
				t.Fatalf("For(minutes, %s) ok=false", tt.method)
			}
			a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatalf("minutes %s affordance did not parse", tt.method)
			}
			if len(a.Examples) != 1 || a.Examples[0].Command != tt.command {
				t.Fatalf("examples = %#v, want one command %q", a.Examples, tt.command)
			}
		})
	}
}

func TestMinutesAffordanceDoesNotDuplicateRuntimeRecovery(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for _, method := range []string{"+todo", "+word-replace", "+summary"} {
		raw, ok := For("minutes", method)
		if !ok {
			t.Fatalf("For(minutes, %s) ok=false", method)
		}
		for _, forbidden := range []string{"auth login", "missing_scopes", "console_url"} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s affordance duplicates runtime recovery %q: %s", method, forbidden, raw)
			}
		}
	}
}

// The transcript is large, so requiring a read before or after a replace makes
// every keyword edit slow. Per-word outcomes come back in the response instead.
func TestMinutesWordReplaceAffordanceDoesNotRequireTranscriptRead(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For("minutes", "+word-replace")
	if !ok {
		t.Fatal("For(minutes, +word-replace) ok=false")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("minutes +word-replace affordance did not parse")
	}
	for _, prereq := range a.Prerequisites {
		if strings.Contains(prereq, "--transcript") || strings.Contains(prereq, "+detail") {
			t.Errorf("+word-replace must not require a transcript read first, got prerequisite: %s", prereq)
		}
	}
	if !containsItem(a.Prerequisites, "minute_token") {
		t.Errorf("+word-replace should state where minute_token comes from, got: %v", a.Prerequisites)
	}
}

func TestMinutesTodoAffordanceRoutesAwayFromTask(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For("minutes", "+todo")
	if !ok {
		t.Fatal("For(minutes, +todo) ok=false")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("minutes +todo affordance did not parse")
	}
	if !containsItem(a.AvoidWhen, "task") {
		t.Fatalf("+todo must steer Task-list work away from minutes: %v", a.AvoidWhen)
	}
}
