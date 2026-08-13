// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
	"github.com/zalando/go-keyring"
)

// `lark-cli auth check` is a predicate command: its README contract is
// `exit 0 = ok, 1 = missing`. The JSON answer goes to stdout; stderr stays
// empty so callers can write `if lark-cli auth check ...; then ... fi`
// without their logs getting polluted by an error envelope on the negative
// branch. These tests pin that contract end-to-end through the dispatcher.

func TestAuthCheckRun_NotLoggedIn_ExitOneWithStdoutOnly(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		// UserOpenId left empty: triggers the not_logged_in branch.
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "calendar:calendar:read"})

	if got := output.ExitCodeOf(err); got != 1 {
		t.Errorf("exit code = %d, want 1 (predicate 'missing' signal)", got)
	}
	var bare *output.BareError
	if !errors.As(err, &bare) {
		t.Fatalf("expected *output.BareError (ErrBare), got %T: %v", err, err)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty for predicate negative answer, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != false {
		t.Errorf("stdout.ok = %v, want false", payload["ok"])
	}
	if payload["error"] != "not_logged_in" {
		t.Errorf("stdout.error = %v, want 'not_logged_in'", payload["error"])
	}
}

func TestAuthCheckRun_NoStoredToken_ExitOneWithStdoutOnly(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_user", UserName: "tester",
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "calendar:calendar:read"})

	if got := output.ExitCodeOf(err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v", err)
	}
	if payload["ok"] != false {
		t.Errorf("stdout.ok = %v, want false", payload["ok"])
	}
	if payload["error"] != "no_token" {
		t.Errorf("stdout.error = %v, want 'no_token'", payload["error"])
	}
}

func TestAuthCheckRun_ScopedTokenPresent_ExitZero(t *testing.T) {
	// Predicate command happy path: stored token covers every required
	// scope. Exit must be 0 (nil error, not ErrBare), stdout carries the
	// `{"ok":true,...}` JSON answer, and stderr stays empty so shell
	// callers can rely on `if lark-cli auth check ...; then` without log
	// pollution. Pairs with the two exit-1 negatives above so both
	// branches of the predicate contract are pinned.
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_user",
		UserName:   "tester",
	}
	now := time.Now()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:            cfg.AppID,
		UserOpenId:       cfg.UserOpenId,
		AccessToken:      "user-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
		GrantedAt:        now.Add(-time.Hour).UnixMilli(),
		Scope:            "im:message docx:document",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "im:message"})

	if err != nil {
		t.Fatalf("expected nil error for happy path (exit 0), got %v", err)
	}
	if got := output.ExitCodeOf(err); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty for predicate exit-0 answer, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	granted, ok := payload["granted"].([]any)
	if !ok || len(granted) != 1 || granted[0] != "im:message" {
		t.Errorf("stdout.granted = %v, want [im:message]", payload["granted"])
	}
	if payload["missing"] != nil {
		t.Errorf("stdout.missing = %v, want nil/absent on happy path", payload["missing"])
	}
	if _, has := payload["suggestion"]; has {
		t.Errorf("stdout.suggestion must be absent on happy path; got %v", payload["suggestion"])
	}
}

type authCheckExternalProvider struct {
	token        *extcred.Token
	capabilities credential.ProviderCapabilities
	resolveCalls int
}

func (p *authCheckExternalProvider) Name() string { return "external-check-test" }

func (p *authCheckExternalProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "test-app", Brand: extcred.BrandFeishu}, nil
}

func (p *authCheckExternalProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	p.resolveCalls++
	return p.token, nil
}

func (p *authCheckExternalProvider) CredentialCapabilities() credential.ProviderCapabilities {
	return p.capabilities
}

func externalAuthCheckFactory(t *testing.T, canInspectScopes bool, token *extcred.Token) (*cmdutil.Factory, *authCheckExternalProvider) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID: "test-app",
		Brand: core.BrandFeishu,
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	provider := &authCheckExternalProvider{
		token: token,
		capabilities: credential.ProviderCapabilities{
			ProvidesOnDemandAuth: true,
			CanInspectScopes:     canInspectScopes,
		},
	}
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{provider},
		nil,
		nil,
		nil,
	)
	return f, provider
}

func TestAuthCheckRun_ExternalDirectUsesProviderScopes(t *testing.T) {
	f, provider := externalAuthCheckFactory(t, true, &extcred.Token{
		Value:  "external-uat",
		Scopes: "im:message docx:document",
	})
	stdout := f.IOStreams.Out.(*bytes.Buffer)

	err := authCheckRun(&CheckOptions{
		Factory: f,
		Scope:   "im:message",
	})
	if err != nil {
		t.Fatalf("authCheckRun() error = %v", err)
	}
	if provider.resolveCalls != 1 {
		t.Fatalf("ResolveToken calls = %d, want 1", provider.resolveCalls)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Fatalf("stdout.ok = %v, want true; payload=%v", payload["ok"], payload)
	}
	granted, ok := payload["granted"].([]any)
	if !ok || len(granted) != 1 || granted[0] != "im:message" {
		t.Fatalf("stdout.granted = %v, want [im:message]", payload["granted"])
	}
}

func TestAuthCheckRun_ExternalProxyReturnsTypedUnknown(t *testing.T) {
	f, provider := externalAuthCheckFactory(t, false, &extcred.Token{
		Value: "proxy-placeholder",
	})

	err := authCheckRun(&CheckOptions{
		Factory: f,
		Scope:   "im:message",
	})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed error", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %#v, want validation/failed_precondition", problem)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(problem.Message, "unsupported") || validation.Param != "" || problem.Hint == "" {
		t.Fatalf("problem = %#v, want explicit unsupported result with actionable hint and no param", problem)
	}
	if provider.resolveCalls != 0 {
		t.Fatalf("ResolveToken calls = %d, want 0 for proxy scope check", provider.resolveCalls)
	}
}

func TestAuthCheckRun_ExternalDirectWithoutScopeMetadataReturnsTypedUnknown(t *testing.T) {
	f, _ := externalAuthCheckFactory(t, true, &extcred.Token{
		Value: "external-uat",
	})

	err := authCheckRun(&CheckOptions{
		Factory: f,
		Scope:   "im:message",
	})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed error", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %#v, want validation/failed_precondition", problem)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(problem.Message, "unknown") || validation.Param != "" || problem.Hint == "" {
		t.Fatalf("problem = %#v, want explicit unknown result with actionable hint and no param", problem)
	}
}

func TestAuthCheckRun_EmptyScopeIsValidationError(t *testing.T) {
	// Scope validation is a real input error, not a predicate negative
	// answer — it must surface as a typed ValidationError with the normal
	// stderr envelope, distinct from the silent ErrBare predicate path.
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "   "})
	if err == nil {
		t.Fatal("expected validation error for empty --scope")
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want ExitValidation (%d)", got, output.ExitValidation)
	}
}

func TestAuthCheckRun_ConcealedLoginOmitsSuggestion(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_user",
		UserName:   "tester",
	}
	now := time.Now()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:            cfg.AppID,
		UserOpenId:       cfg.UserOpenId,
		AccessToken:      "user-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
		GrantedAt:        now.Add(-time.Hour).UnixMilli(),
		Scope:            "im:message",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	visibleFactory, visibleStdout, _, _ := cmdutil.TestFactory(t, cfg)
	if err := authCheckRun(&CheckOptions{
		Factory: visibleFactory,
		Scope:   "calendar:calendar:read",
	}); output.ExitCodeOf(err) != 1 {
		t.Fatalf("default check exit = %d, want predicate miss exit 1", output.ExitCodeOf(err))
	}
	var visiblePayload map[string]any
	if err := json.Unmarshal(visibleStdout.Bytes(), &visiblePayload); err != nil {
		t.Fatalf("default stdout must be valid JSON: %v", err)
	}
	const wantSuggestion = "run `lark-cli auth login --scope \"calendar:calendar:read\" --no-wait --json` to get device_code and verification_url; present verification_url to the user exactly and end this turn; after the user confirms authorization, run `lark-cli auth login --device-code <device_code>` in a later turn to finish login"
	if suggestion, _ := visiblePayload["suggestion"].(string); suggestion != wantSuggestion {
		t.Fatalf("default suggestion = %q, want executable split-flow recovery %q", suggestion, wantSuggestion)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	err := authCheckRunContext(
		context.Background(),
		&CheckOptions{Factory: f, Scope: "calendar:calendar:read"},
		recovery.NewProjector(func() *surface.Plan { return plan }),
	)
	if got := output.ExitCodeOf(err); got != 1 {
		t.Fatalf("exit code = %d, want predicate miss exit 1", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must stay empty, got:\n%s", stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if _, ok := payload["suggestion"]; ok {
		t.Fatalf("concealed auth/login left a dead suggestion: %#v", payload["suggestion"])
	}
	if missing, ok := payload["missing"].([]any); !ok || len(missing) != 1 {
		t.Fatalf("projection removed missing-scope facts: %#v", payload["missing"])
	}
}
