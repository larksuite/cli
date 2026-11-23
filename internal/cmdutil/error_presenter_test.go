// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

func TestFactoryPresentErrorClonesAndPreservesPermissionMachineFields(t *testing.T) {
	cause := errors.New("permission cause")
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithCode(99991679).
		WithLogID("log-123").
		WithMissingScopes("docx:document").
		WithRequestedScopes("docx:document", "drive:drive").
		WithGrantedScopes("drive:drive").
		WithIdentity("user").
		WithCause(cause)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	f := &Factory{
		ResolvedIdentity: core.AsUser,
		Recovery: recovery.NewProjector(func() *surface.Plan {
			return plan
		}),
	}

	rendered := f.PresentError(source, ErrorPresentationOptions{})
	presented, ok := rendered.(*errs.PermissionError)
	if !ok {
		t.Fatalf("PresentError() = %T, want *errs.PermissionError", rendered)
	}
	if presented == source {
		t.Fatal("PresentError returned the producer instead of a clone")
	}
	if !errors.Is(rendered, cause) {
		t.Fatalf("PresentError did not preserve cause %v: %v", cause, rendered)
	}
	problem, ok := errs.ProblemOf(rendered)
	if !ok {
		t.Fatalf("PresentError() = %T, want typed problem", rendered)
	}
	if problem.Category != errs.CategoryAuthorization || problem.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("problem = %s/%s, want authorization/missing_scope", problem.Category, problem.Subtype)
	}
	if presented.Code != source.Code || presented.LogID != source.LogID ||
		presented.Identity != source.Identity || presented.Subtype != source.Subtype {
		t.Fatalf("presented machine fields = %+v, source = %+v", presented, source)
	}
	if strings.Join(presented.MissingScopes, ",") != strings.Join(source.MissingScopes, ",") ||
		strings.Join(presented.RequestedScopes, ",") != strings.Join(source.RequestedScopes, ",") ||
		strings.Join(presented.GrantedScopes, ",") != strings.Join(source.GrantedScopes, ",") {
		t.Fatalf("presented scope fields = %+v, source = %+v", presented, source)
	}
	if strings.Contains(presented.Hint, "auth login") ||
		!strings.Contains(presented.Hint, "supported authorization flow") {
		t.Fatalf("presented concealed hint = %q", presented.Hint)
	}
	if source.Hint != "" {
		t.Fatalf("PresentError mutated producer hint: %q", source.Hint)
	}
}

func TestFactoryPresentErrorRebuildsUnannotatedCanonicalHintWithInvocationContext(t *testing.T) {
	canonical := errclass.PermissionHint(nil, "user", errs.SubtypeMissingScope, "")
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithIdentity("user").
		WithHint("%s", canonical)
	projector := recovery.NewProjectorWithContext(nil, recovery.RenderContext{Profile: "team-beta"})
	f := &Factory{ResolvedIdentity: core.AsUser, Recovery: projector}

	rendered := f.PresentError(source, ErrorPresentationOptions{
		DeclaredScopes: func() []string { return []string{"calendar:calendar.event:read"} },
	})
	presented, ok := rendered.(*errs.PermissionError)
	if !ok {
		t.Fatalf("PresentError() = %T, want *errs.PermissionError", rendered)
	}
	for _, want := range []string{
		`--profile='team-beta'`,
		`--scope "calendar:calendar.event:read"`,
		"--no-wait --json",
		"--device-code",
	} {
		if !strings.Contains(presented.Hint, want) {
			t.Fatalf("presented hint %q does not contain %q", presented.Hint, want)
		}
	}
	if strings.Contains(presented.Hint, "--recommend") {
		t.Fatalf("presented hint retained generic recovery: %q", presented.Hint)
	}
	if source.Hint != canonical {
		t.Fatalf("PresentError mutated producer hint: got %q, want %q", source.Hint, canonical)
	}
}
