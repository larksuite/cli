// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

// The producer always describes its auth-login recovery action. Each command
// tree filters a clone at render time, so one concealed build cannot mutate
// the error subsequently rendered by another build.
func TestNeedUserAuthorization_hintUsesBuildLocalSurface(t *testing.T) {
	source := NewNeedUserAuthorizationError("ou_x")
	var original *errs.AuthenticationError
	if !errors.As(source, &original) {
		t.Fatalf("expected *errs.AuthenticationError, got %T", source)
	}
	if !strings.Contains(original.Hint, "auth login") {
		t.Fatalf("producer hint = %q, want auth login", original.Hint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	concealed := recovery.Render(source, plan)
	var concealedAuth *errs.AuthenticationError
	if !errors.As(concealed, &concealedAuth) {
		t.Fatalf("rendered error = %T, want *errs.AuthenticationError", concealed)
	}
	if concealedAuth == original {
		t.Fatal("Render must return a clone, not mutate the producer error")
	}
	if strings.Contains(concealedAuth.Hint, "auth login") ||
		!strings.Contains(concealedAuth.Hint, "supported authorization flow") {
		t.Errorf("concealed hint = %q, want target-free authorization fallback", concealedAuth.Hint)
	}
	if !IsNeedUserAuthorizationError(concealed) {
		t.Error("render clone lost the NeedAuthorizationError cause")
	}

	var visible *errs.AuthenticationError
	if !errors.As(recovery.Render(source, nil), &visible) || !strings.Contains(visible.Hint, "auth login") {
		t.Errorf("visible render must keep auth login, got %+v", visible)
	}
	if !strings.Contains(original.Hint, "auth login") {
		t.Errorf("concealed render mutated source hint: %q", original.Hint)
	}
}
