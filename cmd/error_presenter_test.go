// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/surface"
	"github.com/spf13/cobra"
)

func TestRootErrorPresenterCompletesDirectPermissionRecoveryWithoutMutatingProducer(t *testing.T) {
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("docx:document").
		WithIdentity("user")

	visible := newRootErrorPresenter(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		recovery.NewProjector(nil),
	).Present(source)
	visibleProblem, _ := errs.ProblemOf(visible)
	if !strings.Contains(visibleProblem.Hint, `auth login --scope "docx:document"`) {
		t.Fatalf("visible recovery = %q, want scoped auth login", visibleProblem.Hint)
	}
	if source.Hint != "" {
		t.Fatalf("presenter mutated producer hint: %q", source.Hint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	concealed := newRootErrorPresenter(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		recovery.NewProjector(func() *surface.Plan { return plan }),
	).Present(source)
	concealedProblem, _ := errs.ProblemOf(concealed)
	if strings.Contains(concealedProblem.Hint, "auth login") ||
		!strings.Contains(concealedProblem.Hint, "supported authorization flow") {
		t.Fatalf("concealed recovery = %q, want target-free fallback", concealedProblem.Hint)
	}
}

func TestRootErrorPresenterDoesNotRecommendUserLoginForBotPermission(t *testing.T) {
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("drive:file:download").
		WithIdentity("bot")

	rendered := newRootErrorPresenter(
		&cmdutil.Factory{ResolvedIdentity: core.AsBot},
		recovery.NewProjector(nil),
	).Present(source)
	problem, _ := errs.ProblemOf(rendered)
	if strings.Contains(problem.Hint, "auth login") ||
		!strings.Contains(problem.Hint, "app developer") {
		t.Fatalf("bot recovery = %q", problem.Hint)
	}
}

func TestRootErrorPresenterDoesNotMutateNestedPermissionCause(t *testing.T) {
	inner := errs.NewPermissionError(errs.SubtypeMissingScope, "inner permission").
		WithMissingScopes("docx:document").
		WithIdentity("user")
	outer := errs.NewInternalError(errs.SubtypeUnknown, "outer failure").
		WithHint("retry the operation").
		WithCause(inner)

	rendered := newRootErrorPresenter(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		recovery.NewProjector(nil),
	).Present(outer)

	if inner.Hint != "" {
		t.Fatalf("presenter mutated nested producer hint: %q", inner.Hint)
	}
	problem, _ := errs.ProblemOf(rendered)
	if got, want := problem.Hint, "retry the operation"; got != want {
		t.Fatalf("rendered outer hint = %q, want %q", got, want)
	}
}

func TestRootErrorPresenterDoesNotMutateNestedAuthenticationCause(t *testing.T) {
	f := factoryWithDeclaredServiceScope(t)
	source := internalauth.NewNeedUserAuthorizationError("ou_nested")
	var inner *errs.AuthenticationError
	if !errors.As(source, &inner) {
		t.Fatalf("source = %T, want nested *errs.AuthenticationError", source)
	}
	originalHint := inner.Hint
	outer := errs.NewInternalError(errs.SubtypeUnknown, "outer failure").
		WithHint("retry the operation").
		WithCause(source)

	rendered := newRootErrorPresenter(f, recovery.NewProjector(nil)).Present(outer)

	if got := inner.Hint; got != originalHint {
		t.Fatalf("presenter mutated nested authentication hint: got %q want %q", got, originalHint)
	}
	problem, _ := errs.ProblemOf(rendered)
	if got, want := problem.Hint, "retry the operation"; got != want {
		t.Fatalf("rendered outer hint = %q, want %q", got, want)
	}
}

func factoryWithDeclaredServiceScope(t *testing.T) *cmdutil.Factory {
	t.Helper()
	f := &cmdutil.Factory{ResolvedIdentity: core.AsUser}
	var target registry.CommandEntry
	for _, entry := range registry.CollectCommandScopes([]string{"calendar"}, "user") {
		if len(entry.Scopes) > 0 {
			target = entry
			break
		}
	}
	if target.Command == "" {
		t.Fatal("failed to locate a service command with declared user scopes")
	}
	parts := strings.Split(target.Command, " ")
	if len(parts) != 2 {
		t.Fatalf("service command = %q, want resource and method", target.Command)
	}
	root := &cobra.Command{Use: "lark-cli"}
	domain := &cobra.Command{Use: "calendar"}
	resource := &cobra.Command{Use: parts[0]}
	method := &cobra.Command{Use: parts[1]}
	root.AddCommand(domain)
	domain.AddCommand(resource)
	resource.AddCommand(method)
	f.CurrentCommand = method
	return f
}
