// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	identitypkg "github.com/larksuite/cli/internal/identity"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

// Token recovery metadata is producer-owned and immutable. Presentation is
// selected by the build-local surface passed to recovery.Render.
func TestTokenMissing_hintUsesBuildLocalSurface(t *testing.T) {
	cause := errors.New("credential chain exhausted")
	source := newTokenMissingError(identitypkg.AsUser, cause)
	var original *errs.AuthenticationError
	if !errors.As(source, &original) || !strings.Contains(original.Hint, "auth login") {
		t.Fatalf("producer must keep auth login hint, got %v", source)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	var concealed *errs.AuthenticationError
	rendered := recovery.Render(source, plan)
	if !errors.As(rendered, &concealed) {
		t.Fatalf("rendered error = %T, want *errs.AuthenticationError", rendered)
	}
	if concealed == original {
		t.Fatal("Render must clone the typed error")
	}
	if strings.Contains(concealed.Hint, "auth login") ||
		!strings.Contains(concealed.Hint, "supported authorization flow") {
		t.Errorf("concealed hint = %q, want target-free authorization fallback", concealed.Hint)
	}
	if !errors.Is(rendered, cause) {
		t.Error("render clone lost the credential-chain cause")
	}

	var visible *errs.AuthenticationError
	if !errors.As(recovery.Render(source, nil), &visible) || !strings.Contains(visible.Hint, "auth login") {
		t.Errorf("visible render must keep auth login, got %+v", visible)
	}
	if !strings.Contains(original.Hint, "auth login") {
		t.Errorf("concealed render mutated source hint: %q", original.Hint)
	}
}
