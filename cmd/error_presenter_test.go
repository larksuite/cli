// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/surface"
	"github.com/spf13/cobra"
)

func TestRootErrorPresenterCompletesDirectPermissionRecoveryWithoutMutatingProducer(t *testing.T) {
	cause := errors.New("permission cause")
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("docx:document").
		WithIdentity("user").
		WithCause(cause)

	visible := presentRootError(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		source,
		recovery.NewProjector(nil),
	)
	visibleProblem, ok := errs.ProblemOf(visible)
	if !ok {
		t.Fatalf("visible error = %T, want typed error", visible)
	}
	if visibleProblem.Category != errs.CategoryAuthorization {
		t.Errorf("visible category = %q, want %q", visibleProblem.Category, errs.CategoryAuthorization)
	}
	if visibleProblem.Subtype != errs.SubtypeMissingScope {
		t.Errorf("visible subtype = %q, want %q", visibleProblem.Subtype, errs.SubtypeMissingScope)
	}
	if !errors.Is(visible, cause) {
		t.Errorf("visible error lost cause %v: %v", cause, visible)
	}
	const wantVisible = "run `lark-cli auth login --scope \"docx:document\" --no-wait --json` to get device_code and verification_url; present verification_url to the user exactly and end this turn; after the user confirms authorization, run `lark-cli auth login --device-code <device_code>` in a later turn to finish login"
	if got, want := visibleProblem.Hint, wantVisible; got != want {
		t.Fatalf("visible recovery = %q, want exact split-flow recovery %q", got, want)
	}
	if source.Hint != "" {
		t.Fatalf("presenter mutated producer hint: %q", source.Hint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	concealed := presentRootError(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		source,
		recovery.NewProjector(func() *surface.Plan { return plan }),
	)
	concealedProblem, _ := errs.ProblemOf(concealed)
	if strings.Contains(concealedProblem.Hint, "auth login") ||
		!strings.Contains(concealedProblem.Hint, "supported authorization flow") {
		t.Fatalf("concealed recovery = %q, want target-free fallback", concealedProblem.Hint)
	}
}

func TestRootErrorPresenterUsesDeclaredScopesForCanonicalPermissionRecovery(t *testing.T) {
	const declaredScope = "calendar:calendar.event:read"

	f := &cmdutil.Factory{ResolvedIdentity: core.AsUser}
	root := &cobra.Command{Use: "lark-cli"}
	calendar := &cobra.Command{Use: "calendar"}
	agenda := &cobra.Command{Use: "+agenda"}
	root.AddCommand(calendar)
	calendar.AddCommand(agenda)
	cmdmeta.SetDeclaredScopes(agenda, map[string][]string{"user": {declaredScope}})
	f.CurrentCommand = agenda

	newSource := func(t *testing.T) (error, *errs.PermissionError) {
		t.Helper()
		err := errclass.BuildAPIError(
			map[string]any{"code": 230027, "msg": "operation unauthorized"},
			errclass.ClassifyContext{Identity: "user"},
		)
		typed, ok := errs.UnwrapTypedError(err)
		if !ok {
			t.Fatalf("source = %T, want typed error", err)
		}
		permission, ok := typed.(*errs.PermissionError)
		if !ok {
			t.Fatalf("source = %T, want *errs.PermissionError", err)
		}
		if len(permission.MissingScopes) != 0 || !strings.Contains(permission.Hint, "--recommend") {
			t.Fatalf("source = %+v, want canonical generic recovery without server scope facts", permission)
		}
		return err, permission
	}

	source, sourcePermission := newSource(t)
	sourceHint := sourcePermission.Hint
	visible := presentRootError(f, source, recovery.NewProjector(nil))
	presented, ok := visible.(*errs.PermissionError)
	if !ok {
		t.Fatalf("visible = %T, want *errs.PermissionError", visible)
	}
	wantVisible := errclass.PermissionRecovery(
		[]string{declaredScope},
		"user",
		errs.SubtypeUserUnauthorized,
		"",
	).String()
	if presented.Hint != wantVisible {
		t.Fatalf("visible recovery = %q, want declared-scope recovery %q", presented.Hint, wantVisible)
	}
	if len(presented.MissingScopes) != 0 {
		t.Fatalf("presentation fabricated missing_scopes: %v", presented.MissingScopes)
	}
	if sourcePermission.Hint != sourceHint || len(sourcePermission.MissingScopes) != 0 {
		t.Fatalf("presenter mutated producer: %+v", sourcePermission)
	}

	const serverScope = "calendar:calendar.event:read:server"
	serverSource := errclass.BuildAPIError(
		map[string]any{
			"code": 99991679,
			"msg":  "missing scope",
			"error": map[string]any{
				"permission_violations": []any{map[string]any{"subject": serverScope}},
			},
		},
		errclass.ClassifyContext{Identity: "user"},
	)
	var serverProducer *errs.PermissionError
	if !errors.As(serverSource, &serverProducer) {
		t.Fatalf("server source = %T, want *errs.PermissionError", serverSource)
	}
	serverPresentedError := presentRootError(f, serverSource, recovery.NewProjector(nil))
	serverPresented, ok := serverPresentedError.(*errs.PermissionError)
	if !ok {
		t.Fatalf("server presented = %T, want *errs.PermissionError", serverPresentedError)
	}
	wantServer := errclass.PermissionRecovery(
		[]string{serverScope},
		"user",
		errs.SubtypeMissingScope,
		"",
	).String()
	if serverPresented.Hint != wantServer {
		t.Fatalf("server recovery = %q, want authoritative server scope %q", serverPresented.Hint, wantServer)
	}
	if len(serverPresented.MissingScopes) != 1 || serverPresented.MissingScopes[0] != serverScope {
		t.Fatalf("presented missing_scopes = %v, want [%s]", serverPresented.MissingScopes, serverScope)
	}
	if len(serverProducer.MissingScopes) != 1 || serverProducer.MissingScopes[0] != serverScope {
		t.Fatalf("presenter mutated server producer: %+v", serverProducer)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	concealedSource, _ := newSource(t)
	concealed := presentRootError(f, concealedSource, recovery.NewProjector(func() *surface.Plan { return plan }))
	concealedPermission, ok := concealed.(*errs.PermissionError)
	if !ok {
		t.Fatalf("concealed = %T, want *errs.PermissionError", concealed)
	}
	wantConcealed := errclass.PermissionRecovery(
		[]string{declaredScope},
		"user",
		errs.SubtypeUserUnauthorized,
		"",
	).Render(plan)
	if concealedPermission.Hint != wantConcealed {
		t.Fatalf("concealed recovery = %q, want declared-scope fallback %q", concealedPermission.Hint, wantConcealed)
	}
	if strings.Contains(concealedPermission.Hint, "auth login") || !strings.Contains(concealedPermission.Hint, declaredScope) {
		t.Fatalf("concealed recovery leaked a command or lost scope context: %q", concealedPermission.Hint)
	}

	custom := errs.NewPermissionError(errs.SubtypeUserUnauthorized, "permission denied").
		WithIdentity("user").
		WithHint("ask the tenant admin to review the resource policy")
	customPresented := presentRootError(f, custom, recovery.NewProjector(nil))
	customProblem, _ := errs.ProblemOf(customPresented)
	if got, want := customProblem.Hint, custom.Hint; got != want {
		t.Fatalf("custom recovery = %q, want producer guidance %q", got, want)
	}
}

func TestRootErrorPresenterPreservesPermissionGuidanceWhenAuthLoginIsConcealed(t *testing.T) {
	const authorizationFallback = "obtain or refresh a user credential through this distribution's supported authorization flow, have the user complete authorization, then retry\ncurrent command requires scope(s): im:message"
	tests := []struct {
		name     string
		subtype  errs.Subtype
		wantHint string
	}{
		{
			name:     "token scope insufficient",
			subtype:  errs.SubtypeTokenScopeInsufficient,
			wantHint: "check the token's granted scopes; " + authorizationFallback,
		},
		{
			name:     "user unauthorized",
			subtype:  errs.SubtypeUserUnauthorized,
			wantHint: authorizationFallback + "; if re-auth does not help, the operation may be blocked by external-chat or admin policy",
		},
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	projector := recovery.NewProjector(func() *surface.Plan { return plan })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("permission cause")
			source := errs.NewPermissionError(tt.subtype, "permission denied").
				WithMissingScopes("im:message").
				WithIdentity("user").
				WithCause(cause)

			rendered := presentRootError(
				&cmdutil.Factory{ResolvedIdentity: core.AsUser},
				source,
				projector,
			)
			presented, ok := rendered.(*errs.PermissionError)
			if !ok {
				t.Fatalf("rendered error = %T, want *errs.PermissionError", rendered)
			}
			problem, ok := errs.ProblemOf(rendered)
			if !ok {
				t.Fatalf("ProblemOf(%T) failed: %v", rendered, rendered)
			}
			if problem.Category != errs.CategoryAuthorization || problem.Subtype != tt.subtype {
				t.Errorf("problem = %s/%s, want authorization/%s", problem.Category, problem.Subtype, tt.subtype)
			}
			if got := presented.Hint; got != tt.wantHint {
				t.Fatalf("concealed recovery = %q, want exact joined recovery %q", got, tt.wantHint)
			}
			if strings.Contains(presented.Hint, "auth login") {
				t.Fatalf("concealed recovery leaks unavailable auth login target: %q", presented.Hint)
			}
			if presented.Message != source.Message || presented.Identity != "user" ||
				len(presented.MissingScopes) != 1 || presented.MissingScopes[0] != "im:message" {
				t.Fatalf("presented machine fields = %+v, source = %+v", presented, source)
			}
			if !errors.Is(rendered, cause) {
				t.Fatalf("rendered error lost cause %v: %v", cause, rendered)
			}
			if source.Hint != "" {
				t.Fatalf("presenter mutated producer hint: %q", source.Hint)
			}
		})
	}
}

func TestRootErrorPresenterDoesNotRecommendUserLoginForBotPermission(t *testing.T) {
	tests := []struct {
		subtype errs.Subtype
		want    string
	}{
		{subtype: errs.SubtypeMissingScope, want: "app developer"},
		{subtype: errs.SubtypeTokenScopeInsufficient, want: "token's granted scopes"},
		{subtype: errs.SubtypeUserUnauthorized, want: "required bot permissions"},
		{subtype: errs.SubtypePermissionDenied, want: "this bot"},
	}
	for _, tt := range tests {
		t.Run(string(tt.subtype), func(t *testing.T) {
			source := errs.NewPermissionError(tt.subtype, "bot permission failure").
				WithMissingScopes("drive:file:download").
				WithIdentity("bot")

			rendered := presentRootError(
				&cmdutil.Factory{ResolvedIdentity: core.AsBot},
				source,
				recovery.NewProjector(nil),
			)
			problem, ok := errs.ProblemOf(rendered)
			if !ok {
				t.Fatalf("rendered error = %T, want typed permission error", rendered)
			}
			for _, forbidden := range []string{"auth login", "verification_url", "device_code", "user credential"} {
				if strings.Contains(strings.ToLower(problem.Hint), forbidden) {
					t.Errorf("bot recovery %q contains user OAuth guidance %q", problem.Hint, forbidden)
				}
			}
			if !strings.Contains(problem.Hint, tt.want) {
				t.Errorf("bot recovery = %q, want guidance containing %q", problem.Hint, tt.want)
			}
			if source.Hint != "" || source.Identity != "bot" || len(source.MissingScopes) != 1 {
				t.Errorf("presenter mutated producer: %+v", source)
			}
		})
	}
}

func TestRootErrorPresenterDoesNotMutateNestedPermissionCause(t *testing.T) {
	inner := errs.NewPermissionError(errs.SubtypeMissingScope, "inner permission").
		WithMissingScopes("docx:document").
		WithIdentity("user")
	outer := errs.NewInternalError(errs.SubtypeUnknown, "outer failure").
		WithHint("retry the operation").
		WithCause(inner)

	rendered := presentRootError(
		&cmdutil.Factory{ResolvedIdentity: core.AsUser},
		outer,
		recovery.NewProjector(nil),
	)

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

	rendered := presentRootError(f, outer, recovery.NewProjector(nil))

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
