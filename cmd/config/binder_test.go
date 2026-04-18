// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// fakeBinder is a test double for SourceBinder. selectCandidate only touches
// Name and ConfigPath (for error messages); ListCandidates/Build are not called
// from selectCandidate, so we can leave them as no-ops.
type fakeBinder struct {
	name string
	path string
}

func (b *fakeBinder) Name() string                                { return b.name }
func (b *fakeBinder) ConfigPath() string                          { return b.path }
func (b *fakeBinder) ListCandidates() ([]Candidate, error)        { return nil, nil }
func (b *fakeBinder) Build(appID string) (*core.AppConfig, error) { return nil, nil }

// tuiUnreachable is a tuiPrompt that fails the test if called. It's the
// guardrail that proves the non-TUI decision paths really do stay out of the
// interactive prompt — otherwise a green test could still hide a silent TUI.
func tuiUnreachable(t *testing.T) func([]Candidate) (*Candidate, error) {
	t.Helper()
	return func([]Candidate) (*Candidate, error) {
		t.Fatal("tuiPrompt must not be called in flag mode")
		return nil, nil
	}
}

// assertCandidate compares the full Candidate struct via DeepEqual so that
// any future field added to Candidate is covered automatically.
func assertCandidate(t *testing.T, got *Candidate, want Candidate) {
	t.Helper()
	if got == nil {
		t.Fatal("expected non-nil Candidate")
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("candidate mismatch:\n  got:  %+v\n  want: %+v", *got, want)
	}
}

func TestSelectCandidate_ZeroCandidates_OpenClaw(t *testing.T) {
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	_, err := selectCandidate(b, nil, "", false, tuiUnreachable(t))
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "no Feishu app configured in openclaw.json",
		Hint:    "configure channels.feishu.appId in openclaw.json",
	})
}

func TestSelectCandidate_ZeroCandidates_GenericSource(t *testing.T) {
	// Locks in the generic fallback so that any future source added to
	// newBinder gets a well-formed validation error on "zero candidates"
	// even before it has a bespoke error message.
	b := &fakeBinder{name: "hermes", path: "/tmp/.env"}
	_, err := selectCandidate(b, nil, "", false, tuiUnreachable(t))
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "validation",
		Message: "hermes: no app configured",
	})
}

func TestSelectCandidate_SingleCandidate_NoFlag_AutoSelect(t *testing.T) {
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{{AppID: "cli_only", Label: "default"}}
	got, err := selectCandidate(b, candidates, "", false, tuiUnreachable(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCandidate(t, got, Candidate{AppID: "cli_only", Label: "default"})
}

func TestSelectCandidate_AppIDFlag_ExactMatch(t *testing.T) {
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{
		{AppID: "cli_work", Label: "work"},
		{AppID: "cli_home", Label: "home"},
	}
	got, err := selectCandidate(b, candidates, "cli_home", false, tuiUnreachable(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCandidate(t, got, Candidate{AppID: "cli_home", Label: "home"})
}

func TestSelectCandidate_AppIDFlag_NoMatch(t *testing.T) {
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{
		{AppID: "cli_work", Label: "work"},
		{AppID: "cli_home", Label: "home"},
	}
	_, err := selectCandidate(b, candidates, "nonexistent", false, tuiUnreachable(t))
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: `--app-id "nonexistent" not found in openclaw.json`,
		Hint:    "available app IDs:\n  cli_work (work)\n  cli_home (home)",
	})
}

func TestSelectCandidate_MultiCandidate_NoFlag_NonTUI(t *testing.T) {
	// Flag-mode with multiple candidates and no --app-id must produce a
	// validation error and the candidate list, never an interactive prompt.
	// isTUI is the single gate; a real terminal alone must not trigger TUI.
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{
		{AppID: "cli_work", Label: "work"},
		{AppID: "cli_home", Label: "home"},
	}
	_, err := selectCandidate(b, candidates, "", false, tuiUnreachable(t))
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: "multiple accounts in openclaw.json; pass --app-id <id>",
		Hint:    "available app IDs:\n  cli_work (work)\n  cli_home (home)",
	})
}

func TestSelectCandidate_MultiCandidate_NoFlag_TUI(t *testing.T) {
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{
		{AppID: "cli_work", Label: "work"},
		{AppID: "cli_home", Label: "home"},
	}
	var gotCandidates []Candidate
	got, err := selectCandidate(b, candidates, "", true, func(cs []Candidate) (*Candidate, error) {
		gotCandidates = cs
		return &cs[1], nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Whole-slice DeepEqual so additions to Candidate propagate to this check.
	if !reflect.DeepEqual(gotCandidates, candidates) {
		t.Errorf("tuiPrompt received %+v, want %+v", gotCandidates, candidates)
	}
	assertCandidate(t, got, Candidate{AppID: "cli_home", Label: "home"})
}

func TestSelectCandidate_SingleCandidate_WrongFlag(t *testing.T) {
	// Even with only one candidate, a wrong --app-id must error rather than
	// silently auto-selecting. An explicit mismatch is always a user mistake,
	// not a reason to override their intent.
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{{AppID: "cli_only"}}
	_, err := selectCandidate(b, candidates, "nonexistent", false, tuiUnreachable(t))
	assertExitError(t, err, output.ExitValidation, output.ErrDetail{
		Type:    "openclaw",
		Message: `--app-id "nonexistent" not found in openclaw.json`,
		Hint:    "available app IDs:\n  cli_only",
	})
}

func TestSelectCandidate_AppIDFlag_WinsOverTUI(t *testing.T) {
	// An explicit --app-id short-circuits the prompt even in TUI mode: a
	// flag the user typed should never be second-guessed by an interactive
	// prompt asking the same question.
	b := &fakeBinder{name: "openclaw", path: "/tmp/openclaw.json"}
	candidates := []Candidate{
		{AppID: "cli_a"},
		{AppID: "cli_b"},
	}
	got, err := selectCandidate(b, candidates, "cli_b", true, tuiUnreachable(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCandidate(t, got, Candidate{AppID: "cli_b"})
}
