// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
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
	if pe.Subtype != errs.SubtypeMissingScope && pe.Subtype != errs.SubtypeAppScopeNotApplied {
		t.Fatalf("Subtype = %q, want a missing-scope subtype", pe.Subtype)
	}
	if pe.Identity != string(identity) {
		t.Fatalf("Identity = %q, want %q", pe.Identity, identity)
	}

	// missing_scopes is server/system ground truth: neither compatible scope is
	// present. The human hint separately recommends one recovery action.
	wantScope := meetingQueryUserScope
	if identity.IsBot() {
		wantScope = meetingQueryBotScope
	}
	if !reflect.DeepEqual(pe.MissingScopes, meetingQueryAnyScopes) {
		t.Fatalf("MissingScopes = %v, want %v", pe.MissingScopes, meetingQueryAnyScopes)
	}
	if !strings.Contains(pe.Hint, wantScope) {
		t.Fatalf("Hint = %q, want recommended scope %q", pe.Hint, wantScope)
	}
	if !strings.Contains(pe.Hint, "either") {
		t.Fatalf("Hint = %q, want compatible OR-scope explanation", pe.Hint)
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
		if strings.Contains(pe.ConsoleURL, url.QueryEscape(meetingQueryUserScope)) {
			t.Fatalf("ConsoleURL = %q, must recommend only bot scope", pe.ConsoleURL)
		}
	} else {
		if !strings.Contains(pe.Hint, "auth login --scope") {
			t.Fatalf("Hint = %q, want auth login guidance", pe.Hint)
		}
		if pe.ConsoleURL != "" {
			t.Fatalf("ConsoleURL = %q, want empty for user identity", pe.ConsoleURL)
		}
	}
	for _, scope := range meetingQueryAnyScopes {
		if !strings.Contains(pe.Error(), scope) {
			t.Fatalf("error %q does not mention compatible scope %q", pe.Error(), scope)
		}
	}
}

func TestCheckMeetingQueryAnyScope_AllowsCompatibleScopes(t *testing.T) {
	cases := []struct {
		name     string
		identity core.Identity
		scopes   string
	}{
		{name: "user_only_event", identity: core.AsUser, scopes: meetingQueryUserScope},
		{name: "user_only_join", identity: core.AsUser, scopes: meetingQueryBotScope},
		{name: "user_both", identity: core.AsUser, scopes: strings.Join(meetingQueryAnyScopes, " ")},
		{name: "bot_only_event", identity: core.AsBot, scopes: meetingQueryUserScope},
		{name: "bot_only_join", identity: core.AsBot, scopes: meetingQueryBotScope},
		{name: "bot_both", identity: core.AsBot, scopes: strings.Join(meetingQueryAnyScopes, " ")},
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
		{name: "bot", identity: core.AsBot},
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
		{
			name: "bot_resolver_error",
			makeRuntime: func() *common.RuntimeContext {
				return newMeetingQueryRuntime(core.AsBot, &meetingQueryTokenResolver{
					err: errors.New("boom"),
				})
			},
		},
		{
			name: "bot_empty_scopes",
			makeRuntime: func() *common.RuntimeContext {
				return newMeetingQueryRuntime(core.AsBot, &meetingQueryTokenResolver{
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

func TestCheckMeetingQueryAnyScope_PropagatesContextErrors(t *testing.T) {
	for _, wantErr := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(wantErr.Error(), func(t *testing.T) {
			runtime := newMeetingQueryRuntime(core.AsUser, &meetingQueryTokenResolver{err: wantErr})
			err := checkMeetingQueryAnyScope(context.Background(), runtime)
			if !errors.Is(err, wantErr) {
				t.Fatalf("checkMeetingQueryAnyScope() error = %v, want %v", err, wantErr)
			}
		})
	}
}

func TestNormalizeMeetingQueryPermissionError_RecommendsOneCompatibleScope(t *testing.T) {
	for _, code := range []int{output.LarkErrAppScopeNotEnabled, output.LarkErrUserScopeInsufficient} {
		for _, identity := range []core.Identity{core.AsUser, core.AsBot} {
			name := string(identity) + "_code_" + fmt.Sprint(code)
			t.Run(name, func(t *testing.T) {
				subtype := errs.SubtypeMissingScope
				if code == output.LarkErrAppScopeNotEnabled {
					subtype = errs.SubtypeAppScopeNotApplied
				}
				original := errs.NewPermissionError(subtype, "upstream permission failure").
					WithCode(code).
					WithLogID("log-id").
					WithRetryable().
					WithMissingScopes(meetingQueryAnyScopes...).
					WithRequestedScopes("requested:scope").
					WithGrantedScopes("granted:scope")
				original.Troubleshooter = "https://example.com/troubleshoot"

				got := normalizeMeetingQueryPermissionError(bareMeetingQueryRuntime(identity), original)
				var pe *errs.PermissionError
				if !errors.As(got, &pe) {
					t.Fatalf("got %T, want *errs.PermissionError", got)
				}
				if pe == original {
					t.Fatal("normalizer mutated the upstream error instead of copying it")
				}
				if !errors.Is(got, original) {
					t.Fatal("normalized error does not preserve the upstream error as cause")
				}
				if pe.Code != code || pe.Subtype != subtype || pe.LogID != "log-id" || !pe.Retryable {
					t.Fatalf("diagnostics changed: %+v", pe.Problem)
				}
				if pe.Troubleshooter != original.Troubleshooter {
					t.Fatalf("Troubleshooter = %q, want %q", pe.Troubleshooter, original.Troubleshooter)
				}
				if !reflect.DeepEqual(pe.MissingScopes, meetingQueryAnyScopes) {
					t.Fatalf("MissingScopes = %v, want %v", pe.MissingScopes, meetingQueryAnyScopes)
				}
				if !reflect.DeepEqual(pe.RequestedScopes, original.RequestedScopes) || !reflect.DeepEqual(pe.GrantedScopes, original.GrantedScopes) {
					t.Fatalf("scope diagnostics changed: requested=%v granted=%v", pe.RequestedScopes, pe.GrantedScopes)
				}
				assertMeetingQueryPermissionError(t, got, identity)
			})
		}
	}
}

func TestNormalizeMeetingQueryPermissionError_PassesThroughNonMatchingErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "single_scope",
			err: errs.NewPermissionError(errs.SubtypeMissingScope, "single").
				WithCode(output.LarkErrUserScopeInsufficient).
				WithMissingScopes(meetingQueryUserScope),
		},
		{
			name: "bot_not_in_meeting",
			err:  errs.NewPermissionError(errs.SubtypePermissionDenied, "not in meeting").WithCode(10005),
		},
		{
			name: "not_in_gray",
			err: errs.NewPermissionError(errs.SubtypePermissionDenied, "not in gray").
				WithCode(20017).
				WithMissingScopes(meetingQueryAnyScopes...),
		},
		{name: "plain_error", err: errors.New("boom")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMeetingQueryPermissionError(bareMeetingQueryRuntime(core.AsBot), tc.err); got != tc.err {
				t.Fatalf("got %T %v, want original error %T %v", got, got, tc.err, tc.err)
			}
		})
	}
}
