// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
)

type meetingQueryTokenResolver struct {
	result *credential.TokenResult
	err    error
}

func (r *meetingQueryTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return r.result, r.err
}

func bareMeetingQueryRuntime(as core.Identity) *common.RuntimeContext {
	return common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "test"}, defaultConfig(), as)
}

func newMeetingQueryRuntimeWithCommand(cmd *cobra.Command, as core.Identity, resolver *meetingQueryTokenResolver) *common.RuntimeContext {
	runtime := common.TestNewRuntimeContextWithIdentity(cmd, defaultConfig(), as)
	runtime.Factory = &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, resolver, nil),
	}
	return runtime
}

func newMeetingQueryRuntimeWithScopes(cmd *cobra.Command, as core.Identity, scopes string) *common.RuntimeContext {
	return newMeetingQueryRuntimeWithCommand(cmd, as, &meetingQueryTokenResolver{
		result: &credential.TokenResult{
			Token:  "test-token",
			Scopes: scopes,
		},
	})
}

func newMeetingQueryRuntime(as core.Identity, resolver *meetingQueryTokenResolver) *common.RuntimeContext {
	return newMeetingQueryRuntimeWithCommand(&cobra.Command{Use: "test"}, as, resolver)
}

func assertMeetingQueryPermissionError(t *testing.T, err error, identity core.Identity) {
	t.Helper()

	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if pe.Category != errs.CategoryAuthorization {
		t.Fatalf("Category = %q, want %q", pe.Category, errs.CategoryAuthorization)
	}
	if pe.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("Subtype = %q, want %q", pe.Subtype, errs.SubtypeMissingScope)
	}
	if pe.Identity != string(identity) {
		t.Fatalf("Identity = %q, want %q", pe.Identity, identity)
	}

	// missing_scopes / Hint drive the AI self-heal path, so they must surface
	// only the single scope the current identity can obtain.
	wantScope := meetingQueryUserScope
	otherScope := meetingQueryBotScope
	if identity.IsBot() {
		wantScope = meetingQueryBotScope
		otherScope = meetingQueryUserScope
	}
	if !reflect.DeepEqual(pe.MissingScopes, []string{wantScope}) {
		t.Fatalf("MissingScopes = %v, want %v", pe.MissingScopes, []string{wantScope})
	}
	if !strings.Contains(pe.Hint, wantScope) {
		t.Fatalf("Hint = %q, want recommended scope %q", pe.Hint, wantScope)
	}
	if strings.Contains(pe.Hint, otherScope) {
		t.Fatalf("Hint = %q, must not steer %s identity toward %q", pe.Hint, identity, otherScope)
	}
	if identity.IsBot() {
		if strings.Contains(pe.Hint, "auth login") {
			t.Fatalf("Hint = %q, must not recommend user login for bot identity", pe.Hint)
		}
		if !strings.Contains(pe.Hint, "developer console") {
			t.Fatalf("Hint = %q, want developer console guidance", pe.Hint)
		}
		if !strings.Contains(pe.ConsoleURL, url.QueryEscape(wantScope)) {
			t.Fatalf("ConsoleURL = %q, want scope %q", pe.ConsoleURL, wantScope)
		}
	} else if !strings.Contains(pe.Hint, "auth login --scope") {
		t.Fatalf("Hint = %q, want auth login guidance", pe.Hint)
	}
	if !strings.Contains(pe.Error(), wantScope) {
		t.Fatalf("error %q does not mention required scope %q", pe.Error(), wantScope)
	}
}

func TestCheckMeetingQueryScope_UserAllowsCompatibleScopes(t *testing.T) {
	cases := []struct {
		name     string
		identity core.Identity
		scopes   string
	}{
		{name: "user_only_event", identity: core.AsUser, scopes: meetingQueryUserScope},
		{name: "user_only_join", identity: core.AsUser, scopes: meetingQueryBotScope},
		{name: "user_both", identity: core.AsUser, scopes: strings.Join(meetingQueryAnyScopes, " ")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newMeetingQueryRuntime(tc.identity, &meetingQueryTokenResolver{
				result: &credential.TokenResult{
					Token:  "test-token",
					Scopes: tc.scopes,
				},
			})
			if err := checkMeetingQueryAnyScope(context.Background(), runtime); err != nil {
				t.Fatalf("checkMeetingQueryAnyScope() error = %v, want nil", err)
			}
		})
	}
}

func TestCheckMeetingQueryAnyScope_MissingScopesReturnsPermissionError(t *testing.T) {
	cases := []struct {
		name     string
		identity core.Identity
	}{
		{name: "user", identity: core.AsUser},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newMeetingQueryRuntime(tc.identity, &meetingQueryTokenResolver{
				result: &credential.TokenResult{
					Token:  "test-token",
					Scopes: "calendar:calendar:read",
				},
			})
			err := checkMeetingQueryAnyScope(context.Background(), runtime)
			if err == nil {
				t.Fatal("expected permission error, got nil")
			}
			assertMeetingQueryPermissionError(t, err, tc.identity)
		})
	}
}

func TestCheckMeetingQueryAnyScope_IsLenientWhenLocalScopeStateIsUnavailable(t *testing.T) {
	cases := []struct {
		name        string
		makeRuntime func() *common.RuntimeContext
	}{
		{
			name: "nil_runtime",
			makeRuntime: func() *common.RuntimeContext {
				return nil
			},
		},
		{
			name: "nil_factory",
			makeRuntime: func() *common.RuntimeContext {
				return bareMeetingQueryRuntime(core.AsUser)
			},
		},
		{
			name: "nil_credential",
			makeRuntime: func() *common.RuntimeContext {
				runtime := bareMeetingQueryRuntime(core.AsUser)
				runtime.Factory = &cmdutil.Factory{}
				return runtime
			},
		},
		{
			name: "resolver_error",
			makeRuntime: func() *common.RuntimeContext {
				return newMeetingQueryRuntime(core.AsUser, &meetingQueryTokenResolver{
					err: errors.New("boom"),
				})
			},
		},
		{
			name: "empty_scopes",
			makeRuntime: func() *common.RuntimeContext {
				return newMeetingQueryRuntime(core.AsUser, &meetingQueryTokenResolver{
					result: &credential.TokenResult{Token: "test-token"},
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkMeetingQueryAnyScope(context.Background(), tc.makeRuntime()); err != nil {
				t.Fatalf("checkMeetingQueryAnyScope() error = %v, want nil", err)
			}
		})
	}
}

func TestCheckMeetingQueryScope_BotUsesPublishedTenantScopes(t *testing.T) {
	cases := []struct {
		name     string
		scopes   []string
		known    bool
		fetchErr error
		wantErr  bool
	}{
		{name: "join_scope_granted", scopes: []string{meetingQueryBotScope}, known: true},
		{name: "event_scope_granted", scopes: []string{meetingQueryUserScope}, known: true},
		{name: "no_tenant_scopes", scopes: []string{}, known: true, wantErr: true},
		{name: "app_metadata_unavailable", fetchErr: errors.New("metadata unavailable")},
		{name: "app_not_published", known: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runtime := bareMeetingQueryRuntime(core.AsBot)
			err := checkMeetingQueryScopeWithTenantScopes(
				context.Background(),
				runtime,
				func(context.Context, *common.RuntimeContext) ([]string, bool, error) {
					return tc.scopes, tc.known, tc.fetchErr
				},
			)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("checkMeetingQueryScopeWithTenantScopes() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("checkMeetingQueryScopeWithTenantScopes() error = nil, want missing scope")
			}
			assertMeetingQueryPermissionError(t, err, core.AsBot)
		})
	}
}
