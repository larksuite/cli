// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func bareMeetingQueryRuntime(as core.Identity) *common.RuntimeContext {
	return common.TestNewRuntimeContextWithIdentity(&cobra.Command{Use: "test"}, defaultConfig(), as)
}

func TestNormalizeMeetingQueryPermissionError_NilRuntimeReturnsOriginalError(t *testing.T) {
	original := errs.NewPermissionError(errs.SubtypeMissingScope, "permission failure").
		WithCode(output.LarkErrUserScopeInsufficient)
	if got := normalizeMeetingQueryPermissionError(nil, original); got != original {
		t.Fatalf("got %v, want original error %v", got, original)
	}
}

func TestNormalizeMeetingQueryPermissionError_TypedNilReturnsOriginalError(t *testing.T) {
	var permissionErr *errs.PermissionError
	var original error = permissionErr
	if got := normalizeMeetingQueryPermissionError(bareMeetingQueryRuntime(core.AsUser), original); got != original {
		t.Fatalf("got %v, want original error %v", got, original)
	}
}

func assertMeetingQueryPermissionError(t *testing.T, err error, identity core.Identity, code int) {
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

	wantScope := meetingQueryUserScope
	if identity.IsBot() {
		wantScope = meetingQueryBotScope
	}
	if !strings.Contains(pe.Hint, wantScope) {
		t.Fatalf("Hint = %q, want recommended scope %q", pe.Hint, wantScope)
	}
	if len(pe.MissingScopes) != 1 || pe.MissingScopes[0] != wantScope {
		t.Fatalf("MissingScopes = %v, want only recommended scope %q", pe.MissingScopes, wantScope)
	}
	if strings.Contains(pe.Hint, "either compatible scope") {
		t.Fatalf("Hint = %q, must not repeat the OR-scope explanation from message", pe.Hint)
	}
	switch code {
	case output.LarkErrAppScopeNotEnabled:
		if strings.Contains(pe.Hint, "auth login") {
			t.Fatalf("Hint = %q, app-scope error must not recommend user login", pe.Hint)
		}
		if !strings.Contains(pe.Hint, "app developer") {
			t.Fatalf("Hint = %q, want app developer guidance", pe.Hint)
		}
		if pe.ConsoleURL == "" {
			t.Fatal("ConsoleURL is empty, want identity-specific developer-console URL")
		}
		if strings.Contains(pe.ConsoleURL, url.QueryEscape(meetingQueryUserScope)) || !strings.Contains(pe.ConsoleURL, url.QueryEscape(meetingQueryBotScope)) {
			t.Fatalf("ConsoleURL = %q, want only bot scope", pe.ConsoleURL)
		}
	case output.LarkErrUserScopeInsufficient:
		if !strings.Contains(pe.Hint, "auth login --scope") {
			t.Fatalf("Hint = %q, want auth login guidance", pe.Hint)
		}
		if pe.ConsoleURL != "" {
			t.Fatalf("ConsoleURL = %q, user-scope error must not expose a developer-console URL", pe.ConsoleURL)
		}
	default:
		t.Fatalf("unexpected code %d", code)
	}
}

func TestNormalizeMeetingQueryPermissionError_RecommendsScopeForMatchingIdentity(t *testing.T) {
	cases := []struct {
		name     string
		identity core.Identity
		code     int
		subtype  errs.Subtype
	}{
		{name: "user_with_user_scope_error", identity: core.AsUser, code: output.LarkErrUserScopeInsufficient, subtype: errs.SubtypeMissingScope},
		{name: "bot_with_app_scope_error", identity: core.AsBot, code: output.LarkErrAppScopeNotEnabled, subtype: errs.SubtypeAppScopeNotApplied},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantScope := meetingQueryUserScope
			if tc.identity == core.AsBot {
				wantScope = meetingQueryBotScope
			}
			wantMessage := "access denied for " + string(tc.identity) + " identity; recommended scope: " + wantScope
			original := errs.NewPermissionError(tc.subtype, "upstream permission failure").
				WithCode(tc.code).
				WithLogID("log-id").
				WithRetryable().
				WithIdentity(string(tc.identity)).
				WithMissingScopes(meetingQueryUserScope, meetingQueryBotScope).
				WithRequestedScopes("requested:scope").
				WithGrantedScopes("granted:scope")
			if tc.identity == core.AsBot {
				original.ConsoleURL = "https://example.com/scopes"
			}
			original.Troubleshooter = "https://example.com/troubleshoot"

			got := normalizeMeetingQueryPermissionError(bareMeetingQueryRuntime(tc.identity), original)
			var pe *errs.PermissionError
			if !errors.As(got, &pe) {
				t.Fatalf("got %T, want *errs.PermissionError", got)
			}
			if got != original || pe != original {
				t.Fatal("normalizer did not return the original permission error")
			}
			if pe.Code != tc.code || pe.Subtype != tc.subtype || pe.LogID != "log-id" || !pe.Retryable {
				t.Fatalf("diagnostics changed: %+v", pe.Problem)
			}
			if pe.Troubleshooter != original.Troubleshooter {
				t.Fatalf("Troubleshooter = %q, want %q", pe.Troubleshooter, original.Troubleshooter)
			}
			if pe.Message != wantMessage {
				t.Fatalf("Message = %q, want %q", pe.Message, wantMessage)
			}
			if tc.identity == core.AsBot {
				consoleURL, err := url.Parse(pe.ConsoleURL)
				if err != nil {
					t.Fatalf("ConsoleURL = %q is invalid: %v", pe.ConsoleURL, err)
				}
				if consoleURL.Host == "" || consoleURL.Query().Get("clientID") != "test-app" || consoleURL.Query().Get("scopes") != meetingQueryBotScope {
					t.Fatalf("ConsoleURL = %q, want test-app and only bot scope", pe.ConsoleURL)
				}
			} else if pe.ConsoleURL != "" {
				t.Fatalf("ConsoleURL = %q, user-scope error must not expose a developer-console URL", pe.ConsoleURL)
			}
			assertMeetingQueryPermissionError(t, got, tc.identity, tc.code)
		})
	}
}

func TestNormalizeMeetingQueryPermissionError_PassesThroughNonMatchingErrors(t *testing.T) {
	cases := []struct {
		name     string
		identity core.Identity
		err      error
	}{
		{
			name:     "user_with_app_scope_error",
			identity: core.AsUser,
			err: errs.NewPermissionError(errs.SubtypeAppScopeNotApplied, "app scope error").
				WithCode(output.LarkErrAppScopeNotEnabled),
		},
		{
			name:     "bot_with_user_scope_error",
			identity: core.AsBot,
			err: errs.NewPermissionError(errs.SubtypeMissingScope, "user scope error").
				WithCode(output.LarkErrUserScopeInsufficient),
		},
		{
			name:     "auto_with_user_scope_error",
			identity: core.AsAuto,
			err: errs.NewPermissionError(errs.SubtypeMissingScope, "auto identity").
				WithCode(output.LarkErrUserScopeInsufficient),
		},
		{
			name: "bot_not_in_meeting",
			err:  errs.NewPermissionError(errs.SubtypePermissionDenied, "not in meeting").WithCode(10005),
		},
		{
			name: "not_in_gray",
			err: errs.NewPermissionError(errs.SubtypePermissionDenied, "not in gray").
				WithCode(20017),
		},
		{name: "plain_error", err: errors.New("boom")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity := tc.identity
			if identity == "" {
				identity = core.AsBot
			}
			if got := normalizeMeetingQueryPermissionError(bareMeetingQueryRuntime(identity), tc.err); got != tc.err {
				t.Fatalf("got %T %v, want original error %T %v", got, got, tc.err, tc.err)
			}
		})
	}
}
