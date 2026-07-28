// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/surface"
)

func TestHintRenderFiltersOnlyUnreferenceableTargets(t *testing.T) {
	hint := Join("; ",
		Command(TargetConfigInit, "run `lark-cli config init`"),
		Command(TargetAuthLogin, "run `lark-cli auth login`"),
		Text("inspect the local logs"),
	)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config/init": surface.CommandConcealed,
		"auth/login":  surface.CommandDeniedVisible,
	})

	if got, want := hint.String(), "run `lark-cli config init`; run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.String() = %q, want %q", got, want)
	}
	if got, want := hint.Render(plan), "run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.Render() = %q, want %q", got, want)
	}
	// Rendering is immutable and repeatable for another command tree.
	if got, want := hint.String(), "run `lark-cli config init`; run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.String() after filtering = %q, want %q", got, want)
	}
}

func TestHintRenderDoesNotLeaveDanglingSeparator(t *testing.T) {
	hint := Join("; ",
		Command(TargetConfigInit, "run config init"),
		Text(""),
		Text("inspect logs"),
	)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config": surface.CommandConcealed,
	})

	if got, want := hint.Render(plan), "inspect logs"; got != want {
		t.Fatalf("Hint.Render() = %q, want %q", got, want)
	}
}

func TestCommandOnlyHintUsesFallbackOnlyWhenTargetIsConcealed(t *testing.T) {
	hint := Join("", Command(TargetAuthLogin, "run auth")).
		WithFallback("use the supported authorization flow")
	if got := hint.String(); got != "run auth" {
		t.Fatalf("visible hint = %q", got)
	}
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	if got := hint.Render(plan); got != "use the supported authorization flow" {
		t.Fatalf("concealed hint = %q", got)
	}
}

func TestAnnotatePreservesErrorChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	typed := errs.NewConfigError(errs.SubtypeNotConfigured, "not configured").
		WithCause(sentinel)

	annotated := Annotate(typed, Join("", Text("hint")))
	if !errors.Is(annotated, sentinel) {
		t.Error("Annotate broke errors.Is traversal")
	}
	if problem, ok := errs.ProblemOf(annotated); !ok || problem != &typed.Problem {
		t.Error("Annotate hid the underlying typed error")
	}
	if Annotate(nil, Join("", Text("hint"))) != nil {
		t.Error("Annotate(nil) must return nil")
	}
}
