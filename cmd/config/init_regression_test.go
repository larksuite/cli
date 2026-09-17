// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
)

const probePath = "/open-apis/application/v6/larksuite_cli_app/probe"

// registerProbeStub records every probe POST without requiring one, so a test
// can assert either that the report was sent or that it never was.
func registerProbeStub(reg *httpmock.Registry) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method:   http.MethodPost,
		URL:      probePath,
		Body:     `{"code":0}`,
		Reusable: true,
		Optional: true,
	}
	reg.Register(stub)
	return stub
}

// seedAgentProfile writes a default profile already bound to an Agent Employee.
func seedAgentProfile(t *testing.T, appID string) {
	t.Helper()
	seed := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId:           appID,
		AppSecret:       core.SecretInput{Plain: "app-secret"},
		Brand:           core.BrandFeishu,
		Users:           []core.AppUser{},
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
		AgentScope:      "im:message",
	}}}
	if err := core.SaveMultiAppConfig(seed); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// TestConfigInitKeepingSecretPreservesAgentBinding covers the readline prompts,
// which are reached with an app-id flag and no secret. Answering the secret
// prompt with a blank line means "keep what is configured"; rebuilding the
// profile from the prompt answers instead would drop the Agent Employee
// binding, and an unclassified profile with no users resolves `--as auto` to
// bot — at which point the app credentials mint an Agent token that travels as
// a tenant token.
func TestConfigInitKeepingSecretPreservesAgentBinding(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	seedAgentProfile(t, "cli_aat")

	f.IOStreams.IsTerminal = true
	f.IOStreams.In = strings.NewReader("\n\n\n") // App ID, App Secret, Brand

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", Brand: "feishu",
	}); err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	app := multi.CurrentAppConfig("")
	if app == nil {
		t.Fatal("config lost its only profile")
	}
	if app.CredentialKind != core.CredentialAAT {
		t.Errorf("credential kind = %q, want %q", app.CredentialKind, core.CredentialAAT)
	}
	if app.PrincipalOpenID != "ou_agent" {
		t.Errorf("principal = %q, want ou_agent", app.PrincipalOpenID)
	}
	if app.AgentScope != "im:message" {
		t.Errorf("agent scope = %q, want im:message", app.AgentScope)
	}
}

// TestConfigInitKeepingSecretRefusesToChangeAppID pins the other half: without
// a secret the CLI cannot verify a different app, so it must say so instead of
// pairing the new app id with the stored secret.
func TestConfigInitKeepingSecretRefusesToChangeAppID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	seedAgentProfile(t, "cli_aat")

	f.IOStreams.IsTerminal = true
	f.IOStreams.In = strings.NewReader("cli_other\n\n\n")

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", Brand: "feishu",
	})
	if err == nil {
		t.Fatal("changing the app id without a secret must be refused")
	}

	multi, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", loadErr)
	}
	if app := multi.CurrentAppConfig(""); app == nil || app.AppId != "cli_aat" {
		t.Fatalf("the refused init must leave the profile untouched, got %#v", app)
	}
}

// TestConfigInitStandardAppReportsSetup pins that a standard app still reports
// its CLI setup to the open platform on the success path, and that it does so
// on the token already minted for verification rather than a second one.
func TestConfigInitStandardAppReportsSetup(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	registerInitInspection(reg, true, 0, "")
	probe := registerProbeStub(reg)

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret",
		Brand: "feishu",
	}); err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}
	if n := len(probe.CapturedBodies); n != 1 {
		t.Fatalf("probe reports = %d, want exactly 1", n)
	}
	if got := probe.CapturedHeaders.Get("Authorization"); got != "Bearer fresh-aat" {
		t.Errorf("probe must reuse the minted token, got %q", got)
	}
}

// TestConfigInitAgentEmployeeSkipsSetupReport pins that the report is skipped
// for an Agent Employee. Its client_credentials call yields an Agent token, and
// the probe endpoint takes a tenant one — sending it there is exactly what the
// credential contract exists to prevent.
func TestConfigInitAgentEmployeeSkipsSetupReport(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	registerInitInspection(reg, true, 66, "ou_agent")
	probe := registerProbeStub(reg)

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret",
		Brand: "feishu",
	}); err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}
	if n := len(probe.CapturedBodies); n != 0 {
		t.Fatalf("an Agent token must never reach the probe endpoint, got %d report(s)", n)
	}
}

// TestConfigInitInterruptedWritesNothing pins that cancelling the command is
// taken as an instruction to change nothing. An interrupted verification looks
// like a transport failure, and falling open on it would write the very
// configuration the user just cancelled and report success doing it.
func TestConfigInitInterruptedWritesNothing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc
	seedAgentProfile(t, "cli_existing")

	ctx, cancel := context.WithCancel(context.Background())
	reg.Register(&httpmock.Stub{
		Method:  http.MethodPost,
		URL:     core.OAuthTokenV3Path,
		OnMatch: func(*http.Request) { cancel() },
		Error:   context.Canceled,
	})

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: ctx, AppID: "cli_new", appSecret: "new-secret", Brand: "feishu",
	})
	if err == nil {
		t.Fatal("an interrupted init must report that it did not finish")
	}

	multi, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", loadErr)
	}
	app := multi.CurrentAppConfig("")
	if app == nil || app.AppId != "cli_existing" {
		t.Fatalf("the cancelled init replaced the configuration anyway: %#v", app)
	}
	if app.CredentialKind != core.CredentialAAT {
		t.Errorf("the cancelled init dropped the Agent binding: %#v", app)
	}
	if got, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_new")); got != "" {
		t.Errorf("the cancelled init stored a secret for the new app: %q", got)
	}
}

// unreadableKeychain answers every Get with the error a lost master key
// produces: the encrypted entries are still there, nothing can decrypt them.
// Set regenerates the key and succeeds, which is why `lark-cli config init` is
// the documented repair.
type unreadableKeychain struct {
	inner *statefulKeychain
	err   error
}

func (k *unreadableKeychain) Get(service, account string) (string, error) {
	return "", k.err
}
func (k *unreadableKeychain) Set(service, account, value string) error {
	return k.inner.Set(service, account, value)
}
func (k *unreadableKeychain) Remove(service, account string) error {
	return k.inner.Remove(service, account)
}

// TestConfigInitRepairsUnreadableCredentialStorage pins the recovery path the
// CLI prints when the master key is gone: rerun config init. Refusing to start
// because the old value cannot be read would close the only door on offer, and
// leave the outcome depending on whether verification happened to be online.
func TestConfigInitRepairsUnreadableCredentialStorage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	inner := newStatefulKeychain()
	f.Keychain = &unreadableKeychain{inner: inner, err: errors.New("keychain not initialized")}
	registerInitInspection(reg, true, 0, "")
	registerProbeStub(reg)

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret",
		Brand: "feishu",
	}); err != nil {
		t.Fatalf("config init must repair unreadable storage, got error = %v", err)
	}
	if got, _ := inner.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat")); got != "app-secret" {
		t.Fatalf("stored app secret = %q, want the freshly written one", got)
	}
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if app := multi.CurrentAppConfig(""); app == nil || app.AppId != "cli_aat" {
		t.Fatalf("stored app = %#v", app)
	}
}

// TestConfigInitFallsOpenWithoutAnHTTPClient pins that a factory that cannot
// build a client falls open instead of refusing. Verification never ran, which
// says nothing about the credentials; before verification existed this failure
// only cost the setup report.
func TestConfigInitFallsOpenWithoutAnHTTPClient(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	f.HttpClient = func() (*http.Client, error) { return nil, errors.New("no transport") }

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_plain", appSecret: "app-secret",
		Brand: "feishu",
	}); err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	app := multi.CurrentAppConfig("")
	if app == nil || app.AppId != "cli_plain" {
		t.Fatalf("stored app = %#v", app)
	}
	if app.CredentialKind != "" {
		t.Errorf("an unverified profile must stay unclassified, got %q", app.CredentialKind)
	}
	if !strings.Contains(stderr.String(), "could not determine whether app") {
		t.Errorf("the user must be told the profile was saved unverified, got: %s", stderr.String())
	}
	if stored, _ := auth.GetStoredAAT(f.Keychain, "cli_plain"); stored != nil {
		t.Errorf("an unverified profile must not leave an Agent token behind, got %#v", stored)
	}
}

// `config init --new --name X` reaches saveAsProfile with a brand-new app. When
// X was bound to an Agent Employee, the binding describes the old app and must
// go with it; leaving it behind produces a profile that claims one Agent's
// principal while holding another app's credentials, and every command then
// fails as if the platform had rebound the Agent.
func TestSaveAsProfileDropsAgentBindingWhenAppChanges(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := newStatefulKeychain()
	if err := auth.SetStoredAAT(kc, &auth.StoredAAToken{
		AppID:       "cli_old",
		UserOpenID:  "ou_old",
		AccessToken: "aat-old",
		ExpiresAt:   time.Now().Add(time.Hour).UnixMilli(),
		GrantedAt:   time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("seed AAT: %v", err)
	}
	existing := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name:            "agent",
		AppId:           "cli_old",
		AppSecret:       core.PlainSecret("old-secret"),
		Brand:           core.BrandFeishu,
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_old",
		AgentScope:      "im:message",
	}}}

	if err := saveAsProfile(existing, kc, "agent", "cli_new", core.PlainSecret("new-secret"), core.BrandFeishu, ""); err != nil {
		t.Fatalf("saveAsProfile: %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	app := saved.Apps[0]
	if app.AppId != "cli_new" {
		t.Fatalf("AppId = %q, want cli_new", app.AppId)
	}
	if app.CredentialKind != "" || app.PrincipalOpenID != "" || app.AgentScope != "" {
		t.Fatalf("agent binding survived the app change: kind=%q principal=%q scope=%q",
			app.CredentialKind, app.PrincipalOpenID, app.AgentScope)
	}
	if stored, _ := auth.GetStoredAAT(kc, "cli_old"); stored != nil {
		t.Fatal("the old app's AAT record survived, but no profile points at it any more")
	}
}
