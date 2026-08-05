// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

// Permission producers attach auth/login as a semantic recovery target. Each
// build filters a clone, while non-command guidance and the source error remain
// intact.
func TestPermissionHint_usesBuildLocalSurface(t *testing.T) {
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	for _, st := range []errs.Subtype{errs.SubtypeMissingScope, errs.SubtypeTokenScopeInsufficient, errs.SubtypeUserUnauthorized} {
		hint := PermissionHint([]string{"im:message"}, "user", st, "")
		sourceTyped := errs.NewPermissionError(st, "permission denied").
			WithHint("%s", hint)
		source := recovery.Attach(sourceTyped, permissionRecoveryHint([]string{"im:message"}, "user", st, ""))

		rendered := recovery.Render(source, plan)
		var concealed *errs.PermissionError
		if !errors.As(rendered, &concealed) {
			t.Fatalf("%s: rendered error = %T, want *errs.PermissionError", st, rendered)
		}
		if concealed == sourceTyped {
			t.Errorf("%s: Render must clone the typed error", st)
		}
		if strings.Contains(concealed.Hint, "auth login") {
			t.Errorf("%s: concealed hint still points at auth login: %q", st, concealed.Hint)
		}
		if !strings.Contains(sourceTyped.Hint, "auth login") {
			t.Errorf("%s: render mutated producer hint: %q", st, sourceTyped.Hint)
		}
		var visible *errs.PermissionError
		if !errors.As(recovery.Render(source, nil), &visible) || !strings.Contains(visible.Hint, "auth login") {
			t.Errorf("%s: visible render must keep auth login, got %+v", st, visible)
		} else {
			for _, want := range []string{
				"--no-wait --json",
				"verification_url",
				"auth login --device-code <device_code>",
				"in a later turn",
			} {
				if !strings.Contains(visible.Hint, want) {
					t.Errorf("%s: OAuth recovery missing %q: %q", st, want, visible.Hint)
				}
			}
		}
	}

	tokenHint := PermissionHint([]string{"im:message"}, "user", errs.SubtypeTokenScopeInsufficient, "")
	if !strings.Contains(tokenHint, "check the token's granted scopes") {
		t.Errorf("token-scope recovery lost token policy guidance: %q", tokenHint)
	}
	userHint := PermissionHint([]string{"im:message"}, "user", errs.SubtypeUserUnauthorized, "")
	if !strings.Contains(userHint, "external-chat or admin policy") {
		t.Errorf("user-unauthorized recovery lost external policy guidance: %q", userHint)
	}

	// Non-command recovery guidance is retained under the same plan.
	consoleHint := PermissionHint(nil, "bot", errs.SubtypeAppScopeNotApplied, "https://example.com")
	consoleErr := errs.NewPermissionError(errs.SubtypeAppScopeNotApplied, "permission denied").
		WithHint("%s", consoleHint)
	consoleErr.ConsoleURL = "https://example.com"
	rendered := recovery.Render(
		recovery.Attach(consoleErr, permissionRecoveryHint(nil, "bot", errs.SubtypeAppScopeNotApplied, "https://example.com")),
		plan,
	)
	var consoleClone *errs.PermissionError
	if !errors.As(rendered, &consoleClone) || !strings.Contains(consoleClone.Hint, "developer console") {
		t.Errorf("console guidance must survive auth concealment, got %+v", consoleClone)
	}
	if consoleClone.ConsoleURL != consoleErr.ConsoleURL {
		t.Errorf("render clone lost ConsoleURL: got %q want %q", consoleClone.ConsoleURL, consoleErr.ConsoleURL)
	}
}
