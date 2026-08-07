// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/identity"
)

type scopeCheckTokenResolver struct {
	result *credential.TokenResult
	err    error
}

func (r *scopeCheckTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return r.result, r.err
}

// TestEnhancePermissionError_TypedPermissionErrorRouted pins typed routing:
// an *errs.PermissionError gets enhanced regardless of its Message text,
// decoupling this helper from canonical-message rewrites that would
// previously break the legacy keyword scan.
func TestEnhancePermissionError_TypedPermissionErrorRouted(t *testing.T) {
	scopes := []string{"drive:drive:read"}
	err := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "access denied: app cli_x has not applied for the required scope(s)",
		},
	}
	got := enhancePermissionError(err, scopes)
	var permErr *errs.PermissionError
	if !errors.As(got, &permErr) {
		t.Fatalf("expected *PermissionError, got %T", got)
	}
	if permErr.Hint != "" {
		t.Errorf("business helper populated presentation hint %q; want typed facts only", permErr.Hint)
	}
	if len(permErr.MissingScopes) != 1 || permErr.MissingScopes[0] != "drive:drive:read" {
		t.Errorf("MissingScopes = %v, want [drive:drive:read]", permErr.MissingScopes)
	}
}

// TestEnhancePermissionError_NonPermissionErrorsPassThrough pins that any
// error that is not an *errs.PermissionError is returned unchanged. Typed
// routing means the upstream message text never flips an unrelated error into
// the permission-enhancement path.
func TestEnhancePermissionError_NonPermissionErrorsPassThrough(t *testing.T) {
	scopes := []string{"contact:contact:read"}
	cases := []struct {
		name string
		err  error
	}{
		{"api error with permission keyword", errs.NewAPIError(errs.SubtypeUnknown, "Permission denied for resource")},
		{"api error with scope keyword", errs.NewAPIError(errs.SubtypeUnknown, "Insufficient scope for operation")},
		{"network error", errs.NewNetworkError(errs.SubtypeNetworkTransport, "request unauthorized by server")},
		{"plain error", fmt.Errorf("plain error")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := enhancePermissionError(tc.err, scopes)
			if got != tc.err {
				t.Errorf("expected original error returned, got %T: %v", got, got)
			}
		})
	}
}

// TestEnhancePermissionError_PermissionErrorGetsScopeFacts pins that business
// enrichment records machine facts and leaves presentation to the root.
func TestEnhancePermissionError_PermissionErrorGetsScopeFacts(t *testing.T) {
	scopes := []string{"calendar:calendar:read", "drive:drive:read"}
	err := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "no permission",
		},
	}
	got := enhancePermissionError(err, scopes)

	var permErr *errs.PermissionError
	if !errors.As(got, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", got, got)
	}
	if permErr.Hint != "" {
		t.Fatalf("Hint = %q, want root presenter to own recovery", permErr.Hint)
	}
	if len(permErr.MissingScopes) != len(scopes) {
		t.Fatalf("MissingScopes = %v, want %v", permErr.MissingScopes, scopes)
	}
}

func TestCheckShortcutScopes_PropagatesContextCancellation(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{err: context.Canceled}, nil),
	}

	err := checkShortcutScopes(f, context.Background(), identity.AsUser, &configpkg.CliConfig{AppID: "app-1"}, []string{"im:message:read"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("checkShortcutScopes() error = %v, want context.Canceled", err)
	}
}

// TestCheckShortcutScopes_ReturnsTypedPermissionError pins that the local
// precheck — when it finds the issued token is missing required scopes —
// emits a typed *errs.PermissionError with Subtype MissingScope, the resolved
// Identity, and the deterministic MissingScopes set. AI/script consumers
// downstream rely on these structured fields instead of parsing hint prose.
// The root presenter turns these facts into build-local recovery.
func TestCheckShortcutScopes_ReturnsTypedPermissionError(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{
			result: &credential.TokenResult{Token: "t", Scopes: "im:message:read calendar:calendar:read"},
		}, nil),
	}

	required := []string{"im:message:read", "drive:drive:read", "docx:document:read"}
	err := checkShortcutScopes(f, context.Background(), identity.AsUser, &configpkg.CliConfig{AppID: "app-1"}, required)
	if err == nil {
		t.Fatal("expected error when token is missing required scopes, got nil")
	}

	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if permErr.Category != errs.CategoryAuthorization {
		t.Errorf("Category = %q, want %q", permErr.Category, errs.CategoryAuthorization)
	}
	if permErr.Subtype != errs.SubtypeMissingScope {
		t.Errorf("Subtype = %q, want %q", permErr.Subtype, errs.SubtypeMissingScope)
	}
	if permErr.Identity != string(identity.AsUser) {
		t.Errorf("Identity = %q, want %q", permErr.Identity, string(identity.AsUser))
	}
	wantMissing := map[string]bool{"drive:drive:read": true, "docx:document:read": true}
	for _, m := range permErr.MissingScopes {
		if !wantMissing[m] {
			t.Errorf("unexpected MissingScopes entry %q (granted scopes should not appear)", m)
		}
		delete(wantMissing, m)
	}
	if len(wantMissing) != 0 {
		t.Errorf("MissingScopes %v did not include expected entries %v", permErr.MissingScopes, wantMissing)
	}
	if permErr.Hint != "" {
		t.Errorf("Hint = %q, want root presenter to own recovery", permErr.Hint)
	}
}

func TestCheckShortcutScopes_IgnoresNonContextTokenErrors(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{err: errors.New("token cache unavailable")}, nil),
	}

	err := checkShortcutScopes(f, context.Background(), identity.AsUser, &configpkg.CliConfig{AppID: "app-1"}, []string{"im:message:read"})
	if err != nil {
		t.Fatalf("checkShortcutScopes() error = %v, want nil", err)
	}
}
