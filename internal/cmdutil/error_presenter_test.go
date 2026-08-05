// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

func TestFactoryPresentErrorClonesAndPreservesPermissionMachineFields(t *testing.T) {
	source := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithCode(99991679).
		WithLogID("log-123").
		WithMissingScopes("docx:document").
		WithRequestedScopes("docx:document", "drive:drive").
		WithGrantedScopes("drive:drive").
		WithIdentity("user")
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
