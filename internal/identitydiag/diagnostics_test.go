// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package identitydiag

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	extcred "github.com/larksuite/cli/extension/credential"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/zalando/go-keyring"
)

func TestDiagnose_NoUserReportsBotReadyAndUserMissing(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.Bot.Status != StatusReady || !got.Bot.Available {
		t.Fatalf("bot = %#v, want ready and available", got.Bot)
	}
	if got.User.Status != StatusMissing || got.User.Available {
		t.Fatalf("user = %#v, want missing and unavailable", got.User)
	}
}

func TestDiagnose_BotIdentityNotConfigured(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test-app", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.Bot.Status != StatusNotConfigured || got.Bot.Available {
		t.Fatalf("bot = %#v, want not_configured and unavailable", got.Bot)
	}
}

func TestDiagnose_VerifyBotIdentity(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	stub := &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"bot": map[string]interface{}{
				"open_id":  "ou_bot",
				"app_name": "diagnostic bot",
			},
		},
	}
	reg.Register(stub)

	got := Diagnose(context.Background(), f, cfg, true)
	if got.Bot.Status != StatusReady || !got.Bot.Available {
		t.Fatalf("bot = %#v, want ready and available", got.Bot)
	}
	if got.Bot.Verified == nil || !*got.Bot.Verified {
		t.Fatalf("bot verified = %v, want true", got.Bot.Verified)
	}
	if got.Bot.OpenID != "ou_bot" || got.Bot.AppName != "diagnostic bot" {
		t.Fatalf("bot info = %#v, want open id and app name", got.Bot)
	}
	if got := stub.CapturedHeaders.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer test-token")
	}
}

func TestDiagnose_VerifyUserIdentity(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app-user",
		AppSecret:  "secret",
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
		Scope:            "offline_access",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"bot": map[string]interface{}{
				"open_id":  "ou_bot",
				"app_name": "diagnostic bot",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})

	got := Diagnose(context.Background(), f, cfg, true)
	if got.User.Status != StatusReady || !got.User.Available {
		t.Fatalf("user = %#v, want ready and available", got.User)
	}
	if got.User.Verified == nil || !*got.User.Verified {
		t.Fatalf("user verified = %v, want true", got.User.Verified)
	}
	if got.User.OpenID != "ou_user" || got.User.UserName != "tester" {
		t.Fatalf("user = %#v, want user identity details", got.User)
	}
}

func TestDiagnose_VerifyBotIdentity_HTTPErrorSurfacesEnvelope(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Status: http.StatusUnauthorized,
		Body: map[string]interface{}{
			"code": 99991663,
			"msg":  "app ticket invalid",
		},
	})

	got := Diagnose(context.Background(), f, cfg, true)
	if got.Bot.Status != StatusVerifyFailed || got.Bot.Available {
		t.Fatalf("bot = %#v, want verify_failed and unavailable", got.Bot)
	}
	if got.Bot.Verified == nil || *got.Bot.Verified {
		t.Fatalf("bot verified = %v, want false", got.Bot.Verified)
	}
	if !strings.Contains(got.Bot.Message, "401") || !strings.Contains(got.Bot.Message, "99991663") {
		t.Fatalf("bot message = %q, want both HTTP code and envelope code", got.Bot.Message)
	}
}

func TestDiagnose_VerifyBotIdentity_BusinessErrorCode(t *testing.T) {
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 10013,
			"msg":  "scope not granted",
		},
	})

	got := Diagnose(context.Background(), f, cfg, true)
	if got.Bot.Status != StatusVerifyFailed || got.Bot.Available {
		t.Fatalf("bot = %#v, want verify_failed and unavailable", got.Bot)
	}
	if !strings.Contains(got.Bot.Message, "10013") || !strings.Contains(got.Bot.Message, "scope not granted") {
		t.Fatalf("bot message = %q, want envelope code/msg", got.Bot.Message)
	}
}

func TestDiagnose_VerifyUserIdentity_ServerRejects(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app-reject",
		AppSecret:  "secret",
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
		Scope:            "offline_access",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/bot/v3/info",
		Body: map[string]interface{}{
			"code": 0,
			"bot":  map[string]interface{}{"open_id": "ou_bot", "app_name": "bot"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    larkauth.PathUserInfoV1,
		Body: map[string]interface{}{
			"code": 99991661,
			"msg":  "access token invalid",
		},
	})

	got := Diagnose(context.Background(), f, cfg, true)
	if got.User.Status != StatusVerifyFailed || got.User.Available {
		t.Fatalf("user = %#v, want verify_failed and unavailable", got.User)
	}
	if got.User.Verified == nil || *got.User.Verified {
		t.Fatalf("user verified = %v, want false", got.User.Verified)
	}
	if !strings.Contains(got.User.Message, "server rejected token") {
		t.Fatalf("user message = %q, want 'server rejected token'", got.User.Message)
	}
}

func TestDiagnose_UserIdentityExpired(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app-expired",
		AppSecret:  "secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_expired",
		UserName:   "tester",
	}
	now := time.Now()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:            cfg.AppID,
		UserOpenId:       cfg.UserOpenId,
		AccessToken:      "user-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(-time.Minute).UnixMilli(),
		GrantedAt:        now.Add(-24 * time.Hour).UnixMilli(),
		Scope:            "offline_access",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	got := Diagnose(context.Background(), f, cfg, false)
	if got.User.Status != StatusMissing || got.User.Available {
		t.Fatalf("user = %#v, want missing and unavailable", got.User)
	}
	if got.User.Hint == "" {
		t.Fatalf("user hint is empty, want re-login hint")
	}
}

func TestDiagnose_BotIdentityStrictUserOnly(t *testing.T) {
	// SupportedIdentities = SupportsUser (1) only — bot path should be
	// reported as not_configured even though an app secret is present.
	cfg := &core.CliConfig{
		AppID:               "test-app",
		AppSecret:           "secret",
		Brand:               core.BrandFeishu,
		SupportedIdentities: 1,
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.Bot.Status != StatusNotConfigured || got.Bot.Available {
		t.Fatalf("bot = %#v, want not_configured and unavailable", got.Bot)
	}
}

func TestDiagnose_UserIdentityMissingAppConfig(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.User.Status != StatusNotConfigured || got.User.Available {
		t.Fatalf("user = %#v, want not_configured and unavailable", got.User)
	}
}

func TestStatusMessage(t *testing.T) {
	cases := map[string]string{
		StatusReady:         StatusReady,
		StatusNotConfigured: "not configured",
		StatusVerifyFailed:  "verify failed",
		StatusNeedsRefresh:  "needs refresh",
		StatusMissing:       "missing",
		"unknown":           "unknown",
	}
	for in, want := range cases {
		if got := StatusMessage(in); got != want {
			t.Errorf("StatusMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiagnose_UserIdentityNeedsRefresh(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app-needs-refresh",
		AppSecret:  "secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_refresh",
		UserName:   "tester",
	}
	now := time.Now()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:            cfg.AppID,
		UserOpenId:       cfg.UserOpenId,
		AccessToken:      "user-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
		GrantedAt:        now.Add(-time.Hour).UnixMilli(),
		Scope:            "offline_access",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	got := Diagnose(context.Background(), f, cfg, false)
	if got.User.Status != StatusNeedsRefresh || !got.User.Available {
		t.Fatalf("user = %#v, want needs_refresh and available", got.User)
	}
	if got.User.TokenStatus != "needs_refresh" {
		t.Fatalf("token status = %q, want needs_refresh", got.User.TokenStatus)
	}
}

// fakeExtProvider is a minimal credential.extcred.Provider for exercising the
// external-credential diagnosis path. account makes the provider "active";
// token (when set) satisfies ResolveToken during verify.
type fakeExtProvider struct {
	name    string
	account *extcred.Account
	token   *extcred.Token
}

func (p *fakeExtProvider) Name() string { return p.name }
func (p *fakeExtProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return p.account, nil
}
func (p *fakeExtProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return p.token, nil
}

func externalFactory(prov *fakeExtProvider, cfg *core.CliConfig) *cmdutil.Factory {
	cred := credential.NewCredentialProvider(
		[]extcred.Provider{prov}, nil, nil,
		func() (*http.Client, error) { return nil, nil },
	)
	return &cmdutil.Factory{
		Config:     func() (*core.CliConfig, error) { return cfg, nil },
		Credential: cred,
		IOStreams:  &cmdutil.IOStreams{},
	}
}

// assertExternalHint locks the contract that an external-provider hint never
// points at interactive commands blocked under an external provider.
func assertExternalHint(t *testing.T, hint string) {
	t.Helper()
	if hint == "" {
		t.Fatalf("hint empty, want external guidance")
	}
	for _, blocked := range []string{"auth login", "config --help"} {
		if strings.Contains(hint, blocked) {
			t.Fatalf("hint %q must not point at %q (blocked under external provider)", hint, blocked)
		}
	}
	if !strings.Contains(hint, "external") {
		t.Fatalf("hint %q should explain credentials are external", hint)
	}
}

func TestDiagnose_External_UserReady(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsAll), UserOpenId: "ou_x", UserName: "Alice"}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	// The bug this guards: the built-in path read the keychain (empty under an
	// external provider) and reported the user as missing. Now availability
	// follows the resolved account, so a signed-in user reads as ready.
	if !got.User.Available || got.User.Status != StatusReady || got.User.TokenStatus != StatusReady {
		t.Fatalf("user = %#v, want ready/available", got.User)
	}
	if got.User.OpenID != "ou_x" || got.User.UserName != "Alice" {
		t.Fatalf("user identity = %#v", got.User)
	}
	if got.User.Hint != "" {
		t.Fatalf("hint = %q, want empty when available", got.User.Hint)
	}
	if !got.Bot.Available || got.Bot.Status != StatusReady {
		t.Fatalf("bot = %#v, want ready/available", got.Bot)
	}
}

func TestDiagnose_External_UserNotSignedIn(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsAll)}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.User.Available || got.User.Status != StatusMissing {
		t.Fatalf("user = %#v, want missing/unavailable", got.User)
	}
	assertExternalHint(t, got.User.Hint)
}

func TestDiagnose_External_BotOnly(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsBot), UserOpenId: "ou_x"}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if !got.Bot.Available || got.Bot.Status != StatusReady {
		t.Fatalf("bot = %#v, want ready/available", got.Bot)
	}
	// Provider declares bot-only: user is unavailable even though an open id is
	// present, and the hint is external (not "auth login").
	if got.User.Available || got.User.Status != StatusNotConfigured {
		t.Fatalf("user = %#v, want not_configured/unavailable", got.User)
	}
	assertExternalHint(t, got.User.Hint)
}

func TestDiagnose_External_UserOnly(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandLark, SupportedIdentities: uint8(extcred.SupportsUser), UserOpenId: "ou_x", UserName: "Bob"}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, false)
	if !got.User.Available || got.User.Status != StatusReady {
		t.Fatalf("user = %#v, want ready/available", got.User)
	}
	if got.Bot.Available || got.Bot.Status != StatusNotConfigured {
		t.Fatalf("bot = %#v, want not_configured/unavailable", got.Bot)
	}
	assertExternalHint(t, got.Bot.Hint)
}

func TestDiagnose_External_VerifyUserResolvesToken(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsUser), UserOpenId: "ou_x", UserName: "Alice"}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}, token: &extcred.Token{Value: "ext-uat"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, true)
	if !got.User.Available || got.User.Verified == nil || !*got.User.Verified {
		t.Fatalf("user = %#v, want available and verified", got.User)
	}
}

func TestDiagnose_External_VerifyUserTokenUnavailable(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", Brand: core.BrandFeishu, SupportedIdentities: uint8(extcred.SupportsUser), UserOpenId: "ou_x"}
	f := externalFactory(&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: "cli_x"}}, cfg)

	got := Diagnose(context.Background(), f, cfg, true)
	if got.User.Available || got.User.Status != StatusVerifyFailed {
		t.Fatalf("user = %#v, want verify_failed/unavailable", got.User)
	}
	if got.User.Verified == nil || *got.User.Verified {
		t.Fatalf("verified = %v, want false", got.User.Verified)
	}
	assertExternalHint(t, got.User.Hint)
}
