// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
)

type scopeCheckTokenResolver struct {
	result *credential.TokenResult
	err    error
}

func (r *scopeCheckTokenResolver) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return r.result, r.err
}

func TestEnhancePermissionError_MissingScopeType(t *testing.T) {
	scopes := []string{"calendar:calendar:read"}
	err := &output.ExitError{
		Code:   1,
		Detail: &output.ErrDetail{Type: "missing_scope", Message: "missing scope"},
	}
	got := enhancePermissionError(err, scopes)
	var exitErr *output.ExitError
	if !errors.As(got, &exitErr) {
		t.Fatalf("expected ExitError, got %T", got)
	}
	if exitErr.Detail.Hint == "" {
		t.Error("expected hint for missing_scope type")
	}
	if !strings.Contains(exitErr.Detail.Hint, "calendar:calendar:read") {
		t.Errorf("hint %q missing scope info", exitErr.Detail.Hint)
	}
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
	if !strings.Contains(permErr.Hint, "drive:drive:read") {
		t.Errorf("hint %q missing scope info", permErr.Hint)
	}
}

// TestEnhancePermissionError_KeywordScanRemoved pins that an *output.ExitError
// whose Detail.Type is NOT "permission" / "missing_scope" is no longer
// matched by upstream-message keyword scan. This is the contract change in
// T15: typed routing replaces the brittle keyword scan, so canonical
// message rewrites cannot accidentally flip an unrelated api_error into
// the permission-enhancement path.
func TestEnhancePermissionError_KeywordScanRemoved(t *testing.T) {
	scopes := []string{"contact:contact:read"}
	cases := []struct {
		name string
		msg  string
	}{
		{"permission keyword", "Permission denied for resource"},
		{"scope keyword", "Insufficient scope for operation"},
		{"authorization keyword", "Authorization required"},
		{"unauthorized keyword", "request unauthorized by server"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &output.ExitError{
				Code:   1,
				Detail: &output.ErrDetail{Type: "api_error", Message: tc.msg},
			}
			got := enhancePermissionError(err, scopes)
			if got != err {
				t.Errorf("expected original error returned (type=api_error must not match), got %T: %v", got, got)
			}
		})
	}
}

func TestEnhancePermissionError(t *testing.T) {
	scopes := []string{"calendar:calendar:read", "drive:drive:read"}

	tests := []struct {
		name       string
		err        error
		wantHint   bool
		hintSubstr string
	}{
		{
			name: "permission type gets enhanced",
			err: &output.ExitError{
				Code:   1,
				Detail: &output.ErrDetail{Type: "permission", Message: "no permission"},
			},
			wantHint:   true,
			hintSubstr: "scope",
		},
		{
			name: "mcp_error with unauthorized keyword not enhanced (keyword scan removed)",
			err: &output.ExitError{
				Code:   1,
				Detail: &output.ErrDetail{Type: "mcp_error", Message: "request unauthorized by server"},
			},
			wantHint: false,
		},
		{
			name: "api_error without keyword not modified",
			err: &output.ExitError{
				Code:   1,
				Detail: &output.ErrDetail{Type: "api_error", Message: "timeout"},
			},
			wantHint: false,
		},
		{
			name:     "plain error not modified",
			err:      fmt.Errorf("plain error"),
			wantHint: false,
		},
		{
			name: "nil Detail not modified",
			err: &output.ExitError{
				Code:   1,
				Detail: nil,
			},
			wantHint: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enhancePermissionError(tt.err, scopes)

			if !tt.wantHint {
				// Should return original error unchanged
				if got != tt.err {
					t.Errorf("expected original error returned, got different error: %v", got)
				}
				return
			}

			// Should return an enhanced ExitError with a hint
			var exitErr *output.ExitError
			if !errors.As(got, &exitErr) {
				t.Fatalf("expected ExitError, got %T: %v", got, got)
			}
			if exitErr.Detail == nil {
				t.Fatal("expected Detail to be non-nil")
			}
			if exitErr.Detail.Hint == "" {
				t.Fatal("expected non-empty hint")
			}
			if !strings.Contains(exitErr.Detail.Hint, tt.hintSubstr) {
				t.Errorf("hint %q does not contain %q", exitErr.Detail.Hint, tt.hintSubstr)
			}
			// Verify the hint includes the actual scopes
			for _, s := range scopes {
				if !strings.Contains(exitErr.Detail.Hint, s) {
					t.Errorf("hint %q does not contain scope %q", exitErr.Detail.Hint, s)
				}
			}
		})
	}
}

func TestCheckShortcutScopes_PropagatesContextCancellation(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{err: context.Canceled}, nil),
	}

	err := checkShortcutScopes(f, context.Background(), core.AsUser, &core.CliConfig{AppID: "app-1"}, []string{"im:message:read"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("checkShortcutScopes() error = %v, want context.Canceled", err)
	}
}

// TestCheckShortcutScopes_ReturnsTypedPermissionError pins that the local
// precheck — when it finds the issued token is missing required scopes —
// emits a typed *errs.PermissionError with Subtype MissingScope, the resolved
// Identity, and the deterministic MissingScopes set. AI/script consumers
// downstream rely on these structured fields instead of parsing the hint
// string. The Hint still carries the actionable `auth login --scope ...`
// command for human consumers.
func TestCheckShortcutScopes_ReturnsTypedPermissionError(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{
			result: &credential.TokenResult{Token: "t", Scopes: "im:message:read calendar:calendar:read"},
		}, nil),
	}

	required := []string{"im:message:read", "drive:drive:read", "docx:document:read"}
	err := checkShortcutScopes(f, context.Background(), core.AsUser, &core.CliConfig{AppID: "app-1"}, required)
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
	if permErr.Identity != string(core.AsUser) {
		t.Errorf("Identity = %q, want %q", permErr.Identity, string(core.AsUser))
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
	if permErr.Hint == "" {
		t.Error("Hint must carry the `auth login --scope ...` recovery action")
	}
	if !strings.Contains(permErr.Hint, "auth login") {
		t.Errorf("Hint = %q, want it to mention `auth login`", permErr.Hint)
	}
}

func TestCheckShortcutScopes_IgnoresNonContextTokenErrors(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{err: errors.New("token cache unavailable")}, nil),
	}

	err := checkShortcutScopes(f, context.Background(), core.AsUser, &core.CliConfig{AppID: "app-1"}, []string{"im:message:read"})
	if err != nil {
		t.Fatalf("checkShortcutScopes() error = %v, want nil", err)
	}
}
