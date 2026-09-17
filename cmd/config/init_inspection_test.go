// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
)

func registerInitInspection(reg *httpmock.Registry, active bool, userType int, openID string) {
	tokenStub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    core.OAuthTokenV3Path,
		Body:   `{"code":0,"access_token":"fresh-aat","token_type":"Bearer","expires_in":7200}`,
	}
	introspectionStub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    auth.PathOAuthIntrospect,
		Body: `{"active":` + strconv.FormatBool(active) +
			`,"client_id":"cli_aat","token_type":"Bearer","iat":` + strconv.FormatInt(time.Now().Unix(), 10) +
			`,"exp":` + strconv.FormatInt(time.Now().Add(2*time.Hour).Unix(), 10) +
			`,"user_type":` + strconv.Itoa(userType) + `,"open_id":"` + openID + `"}`,
	}
	reg.Register(tokenStub)
	reg.Register(introspectionStub)
}

func TestConfigInitExistingAppPersistsVerifiedAATProfileAndCache(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc
	registerInitInspection(reg, true, 66, "ou_agent")

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret",
		Brand: "feishu", ProfileName: "agent-prod",
	})
	if err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	app := multi.FindApp("agent-prod")
	if app == nil || app.CredentialKind != core.CredentialAAT || app.PrincipalOpenID != "ou_agent" {
		t.Fatalf("stored app = %#v", app)
	}
	if app.AppSecret.Ref == nil || app.AppSecret.Ref.ID != "appsecret:cli_aat" {
		t.Fatalf("stored app secret = %#v", app.AppSecret)
	}
	if len(app.Users) != 0 {
		t.Fatalf("Agent principal must not be stored in Users: %#v", app.Users)
	}
	secret, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat"))
	if secret != "app-secret" {
		t.Fatalf("stored app secret = %q", secret)
	}
	stored, err := auth.GetStoredAAT(kc, "cli_aat")
	if err != nil || stored == nil {
		t.Fatalf("GetStoredAAT() = %#v, %v", stored, err)
	}
	if stored.AppID != "cli_aat" || stored.UserOpenID != "ou_agent" || stored.AccessToken != "fresh-aat" ||
		stored.ExpiresAt <= stored.GrantedAt {
		t.Fatalf("stored AAT = %#v", stored)
	}
}

// registerInitIntrospectionBody registers a reusable token stub and one
// Introspection stub answering with the given raw body, for the cases where the
// verdict itself is the thing under test.
func registerInitIntrospectionBody(reg *httpmock.Registry, introspectionBody string) {
	reg.Register(&httpmock.Stub{
		Method:   http.MethodPost,
		URL:      core.OAuthTokenV3Path,
		Body:     `{"code":0,"access_token":"fresh-aat","token_type":"Bearer","expires_in":7200}`,
		Reusable: true,
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    auth.PathOAuthIntrospect,
		Body:   introspectionBody,
	})
}

// TestConfigInitIntrospectionVerdictFallsOpen pins that a negative answer from
// Introspection never blocks config init. Introspection is asked one question,
// "is this app an Agent Employee"; any answer other than yes -- inactive,
// another client id, invalid_token, an unparseable body -- means the question
// was not answered, not that the credentials are bad. The token endpoint has
// already accepted them, and a standard app must not be locked out of the CLI
// because Introspection treats its tenant token differently.
func TestConfigInitIntrospectionVerdictFallsOpen(t *testing.T) {
	exp := strconv.FormatInt(time.Now().Add(2*time.Hour).Unix(), 10)
	cases := []struct {
		name string
		body string
	}{
		{name: "inactive token", body: `{"active":false,"client_id":"cli_aat","user_type":66,"open_id":"ou_agent","exp":` + exp + `}`},
		{name: "different client id", body: `{"active":true,"client_id":"cli_other","user_type":1,"exp":` + exp + `}`},
		{name: "invalid_token error", body: `{"error":"invalid_token","error_description":"revoked"}`},
		{name: "unparseable body", body: `<html>gateway</html>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
			f, _, stderr, reg := cmdutil.TestFactory(t, nil)
			kc := newStatefulKeychain()
			f.Keychain = kc
			registerInitIntrospectionBody(reg, tc.body)

			err := configInitRun(&ConfigInitOptions{
				Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret", Brand: "feishu",
			})
			if err != nil {
				t.Fatalf("configInitRun() error = %v, want fail-open save", err)
			}
			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatal(err)
			}
			app := multi.CurrentAppConfig("")
			if app == nil || app.AppId != "cli_aat" || app.CredentialKind != "" || app.PrincipalOpenID != "" {
				t.Fatalf("stored app = %#v, want unclassified cli_aat", app)
			}
			if got, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat")); got != "app-secret" {
				t.Fatalf("stored app secret = %q", got)
			}
			if got, _ := kc.Get(keychain.LarkCliService, auth.AATAccountKey("cli_aat")); got != "" {
				t.Fatalf("unexpected AAT cache: %q", got)
			}
			if !strings.Contains(stderr.String(), "Agent Employee") {
				t.Fatalf("stderr = %q, want a warning that Agent Employee detection was skipped", stderr.String())
			}
		})
	}
}

// TestConfigInitTokenEndpointRejectionWritesNothing pins the one verdict that
// still blocks: the token endpoint itself refusing the app credentials. That is
// deterministic and about the credentials, so saving them would only defer the
// same failure to the first real command.
func TestConfigInitTokenEndpointRejectionWritesNothing(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    core.OAuthTokenV3Path,
		Body:   `{"error":"invalid_client","error_description":"The client secret is invalid.","code":20002}`,
	})

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "wrong-secret", Brand: "feishu",
	})
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) || cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("error = %T %v, want config/invalid_client", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "config.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config.json stat error = %v, want not exist", statErr)
	}
	if got, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat")); got != "" {
		t.Fatalf("app secret was written after the token endpoint rejected it: %q", got)
	}
	if got, _ := kc.Get(keychain.LarkCliService, auth.AATAccountKey("cli_aat")); got != "" {
		t.Fatalf("AAT was written after the token endpoint rejected the app: %q", got)
	}
}

// TestConfigInitIntrospectionTimeoutStillReportsSetup pins that the fail-open
// path gets a live context. Inspection times out on its own budget; the probe
// that follows must not inherit that spent budget, or it fails before it starts
// and the setup report is silently lost on exactly the slow networks the
// longer inspection timeout was meant to tolerate.
func TestConfigInitIntrospectionTimeoutStillReportsSetup(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	previous := initCredentialInspectionTimeout
	initCredentialInspectionTimeout = 20 * time.Millisecond
	t.Cleanup(func() { initCredentialInspectionTimeout = previous })

	f, _, _, reg := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	reg.Register(&httpmock.Stub{
		Method:   http.MethodPost,
		URL:      core.OAuthTokenV3Path,
		Body:     `{"code":0,"access_token":"tat","token_type":"Bearer","expires_in":7200}`,
		Reusable: true,
	})
	reg.Register(&httpmock.Stub{
		Method:  http.MethodPost,
		URL:     auth.PathOAuthIntrospect,
		OnMatch: func(*http.Request) { time.Sleep(60 * time.Millisecond) },
		Error:   context.DeadlineExceeded,
	})
	probe := registerProbeStub(reg)

	if err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_slow", appSecret: "app-secret", Brand: "feishu",
	}); err != nil {
		t.Fatalf("configInitRun() error = %v, want fail-open save", err)
	}
	if n := len(probe.CapturedBodies); n != 1 {
		t.Fatalf("probe reports = %d, want exactly 1 after a slow inspection", n)
	}
}

// TestConfigInitInterruptedDuringIntrospectionWritesNothing is the companion of
// TestConfigInitInterruptedWritesNothing for the second network call: a Ctrl-C
// that lands during Introspection is still an instruction to change nothing,
// and must not be read as an Introspection failure that falls open.
func TestConfigInitInterruptedDuringIntrospectionWritesNothing(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc

	ctx, cancel := context.WithCancel(context.Background())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    core.OAuthTokenV3Path,
		Body:   `{"code":0,"access_token":"tat","token_type":"Bearer","expires_in":7200}`,
	})
	reg.Register(&httpmock.Stub{
		Method:  http.MethodPost,
		URL:     auth.PathOAuthIntrospect,
		OnMatch: func(*http.Request) { cancel() },
		Error:   context.Canceled,
	})

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: ctx, AppID: "cli_new", appSecret: "new-secret", Brand: "feishu",
	})
	if err == nil {
		t.Fatal("an interrupted init must report that it did not finish")
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "config.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config.json stat error = %v, want not exist", statErr)
	}
	if got, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_new")); got != "" {
		t.Fatalf("the cancelled init stored a secret for the new app: %q", got)
	}
}

func TestConfigInitNonAgentKeepsLegacyProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc
	registerInitInspection(reg, true, 0, "")

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "app-secret", Brand: "feishu",
	})
	if err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := multi.CurrentAppConfig("")
	if app == nil || app.CredentialKind != "" || app.PrincipalOpenID != "" {
		t.Fatalf("legacy app gained AAT classification: %#v", app)
	}
	if got, _ := kc.Get(keychain.LarkCliService, auth.AATAccountKey("cli_aat")); got != "" {
		t.Fatalf("legacy app stored AAT cache: %q", got)
	}
}

// TestConfigInitRebindsExistingProfileToAnotherAgentEmployee pins the in-place
// rebind path: an existing Agent Employee snapshot is replaced wholesale, not merged.
func TestConfigInitRebindsExistingProfileToAnotherAgentEmployee(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc

	const oldAppID = "cli_old_agent"
	const newAppID = "cli_new_agent"

	// Seed "agent-prod" bound to another Agent Employee app.
	if err := kc.Set(keychain.LarkCliService, core.AppSecretAccountKey(oldAppID), "old-secret"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetStoredAAT(kc, &auth.StoredAAToken{
		AppID:       oldAppID,
		UserOpenID:  "ou_old_agent",
		AccessToken: "old-aat",
		ExpiresAt:   time.Now().Add(time.Hour).UnixMilli(),
		GrantedAt:   time.Now().UnixMilli(),
		Scope:       "old:scope",
	}); err != nil {
		t.Fatal(err)
	}
	seed := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name:            "agent-prod",
		AppId:           oldAppID,
		AppSecret:       core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: core.AppSecretAccountKey(oldAppID)}},
		Brand:           core.BrandFeishu,
		Users:           []core.AppUser{},
		PrincipalOpenID: "ou_old_agent",
		CredentialKind:  core.CredentialAAT,
		AgentScope:      "old:scope",
	}}}
	if err := core.SaveMultiAppConfig(seed); err != nil {
		t.Fatal(err)
	}

	// Rebind it to a different app, principal and scope.
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    core.OAuthTokenV3Path,
		Body:   `{"code":0,"access_token":"fresh-aat-2","token_type":"Bearer","expires_in":7200,"scope":"new:scope"}`,
	})
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    auth.PathOAuthIntrospect,
		Body: `{"active":true,"client_id":"` + newAppID + `","token_type":"Bearer","iat":` +
			strconv.FormatInt(time.Now().Unix(), 10) + `,"exp":` +
			strconv.FormatInt(time.Now().Add(2*time.Hour).Unix(), 10) +
			`,"user_type":66,"open_id":"ou_new_agent"}`,
	})

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: newAppID, appSecret: "new-secret",
		Brand: "feishu", ProfileName: "agent-prod",
	})
	if err != nil {
		t.Fatalf("configInitRun() error = %v", err)
	}

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	app := multi.FindApp("agent-prod")
	if app == nil {
		t.Fatal("profile \"agent-prod\" vanished after rebind")
	}
	if app.AppId != newAppID {
		t.Fatalf("AppId = %q, want the new app id %q", app.AppId, newAppID)
	}
	if app.CredentialKind != core.CredentialAAT {
		t.Fatalf("CredentialKind = %q, want %q", app.CredentialKind, core.CredentialAAT)
	}
	if app.PrincipalOpenID != "ou_new_agent" {
		t.Fatalf("PrincipalOpenID = %q, want the newly verified principal, got the stale one back", app.PrincipalOpenID)
	}
	if app.AgentScope != "new:scope" {
		t.Fatalf("AgentScope = %q, want the newly minted scope %q, not the stale or empty value", app.AgentScope, "new:scope")
	}

	stored, err := auth.GetStoredAAT(kc, newAppID)
	if err != nil || stored == nil {
		t.Fatalf("GetStoredAAT() = %#v, %v", stored, err)
	}
	if stored.Scope != "new:scope" {
		t.Fatalf("stored AAT scope = %q, want %q", stored.Scope, "new:scope")
	}
}

func TestPersistInspectedInitRestoresKeychainWhenConfigCommitFails(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	if err := os.Mkdir(filepath.Join(configDir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	kc := newStatefulKeychain()
	f.Keychain = kc
	registerInitInspection(reg, true, 66, "ou_agent")
	if err := kc.Set(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat"), "old-secret"); err != nil {
		t.Fatal(err)
	}
	if err := kc.Set(keychain.LarkCliService, auth.AATAccountKey("cli_aat"), "old-cache-bytes"); err != nil {
		t.Fatal(err)
	}
	err := persistInspectedInit(context.Background(), f, "", nil,
		"cli_aat", "new-secret", core.BrandFeishu, "")
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeStorage {
		t.Fatalf("error = %T %v, want internal/storage", err, err)
	}
	secret, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_aat"))
	cache, _ := kc.Get(keychain.LarkCliService, auth.AATAccountKey("cli_aat"))
	if secret != "old-secret" || cache != "old-cache-bytes" {
		t.Fatalf("rollback secret/cache = %q/%q, want old values", secret, cache)
	}
}

type failingRestoreKeychain struct {
	*statefulKeychain
	failAccount string
	failValue   string
	err         error
}

func (k *failingRestoreKeychain) Set(service, account, value string) error {
	if account == k.failAccount && value == k.failValue {
		return k.err
	}
	return k.statefulKeychain.Set(service, account, value)
}

func TestPersistInspectedInitReportsIncompleteKeychainRollback(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	if err := os.Mkdir(filepath.Join(configDir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	f, _, _, reg := cmdutil.TestFactory(t, nil)
	registerInitInspection(reg, true, 66, "ou_agent")

	base := newStatefulKeychain()
	secretAccount := core.AppSecretAccountKey("cli_aat")
	if err := base.Set(keychain.LarkCliService, secretAccount, "old-secret"); err != nil {
		t.Fatal(err)
	}
	if err := base.Set(keychain.LarkCliService, auth.AATAccountKey("cli_aat"), "old-cache-bytes"); err != nil {
		t.Fatal(err)
	}
	restoreErr := errors.New("keychain restore failed")
	f.Keychain = &failingRestoreKeychain{
		statefulKeychain: base,
		failAccount:      secretAccount,
		failValue:        "old-secret",
		err:              restoreErr,
	}

	err := persistInspectedInit(context.Background(), f, "", nil,
		"cli_aat", "new-secret", core.BrandFeishu, "")
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeStorage ||
		!errors.Is(err, restoreErr) {
		t.Fatalf("error = %T %v, want storage error preserving rollback failure", err, err)
	}
	if !strings.Contains(internalErr.Message, "rollback was incomplete") {
		t.Fatalf("message = %q, want incomplete rollback warning", internalErr.Message)
	}
}

// TestConfigInitFailsOpenWhenInspectionUnreachable pins the compatibility
// contract for a standard app: when the accounts endpoint cannot answer whether
// the app is an Agent Employee, config init saves the profile exactly as it did
// before inspection existed (unclassified, no AAT cache) and warns, instead of
// refusing. Only a deterministic credential rejection may block.
func TestConfigInitFailsOpenWhenInspectionUnreachable(t *testing.T) {
	cases := []struct {
		name  string
		stubs func(reg *httpmock.Registry)
	}{
		{name: "token endpoint unreachable", stubs: func(*httpmock.Registry) {}},
		{name: "introspection unreachable", stubs: func(reg *httpmock.Registry) {
			reg.Register(&httpmock.Stub{
				Method: http.MethodPost,
				URL:    core.OAuthTokenV3Path,
				Body:   `{"code":0,"access_token":"tat","token_type":"Bearer","expires_in":7200}`,
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, _, stderr, reg := cmdutil.TestFactory(t, nil)
			kc := newStatefulKeychain()
			f.Keychain = kc
			tc.stubs(reg)

			err := configInitRun(&ConfigInitOptions{
				Factory: f, Ctx: context.Background(), AppID: "cli_plain", appSecret: "app-secret", Brand: "feishu",
			})
			if err != nil {
				t.Fatalf("configInitRun() error = %v, want fail-open save", err)
			}
			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatal(err)
			}
			app := multi.CurrentAppConfig("")
			if app == nil || app.AppId != "cli_plain" || app.CredentialKind != "" || app.PrincipalOpenID != "" {
				t.Fatalf("stored app = %#v, want unclassified cli_plain", app)
			}
			if got, _ := kc.Get(keychain.LarkCliService, core.AppSecretAccountKey("cli_plain")); got != "app-secret" {
				t.Fatalf("stored app secret = %q", got)
			}
			if got, _ := kc.Get(keychain.LarkCliService, auth.AATAccountKey("cli_plain")); got != "" {
				t.Fatalf("unexpected AAT cache: %q", got)
			}
			if !strings.Contains(stderr.String(), "Agent Employee") {
				t.Fatalf("stderr = %q, want a warning that Agent Employee detection was skipped", stderr.String())
			}
		})
	}
}

// TestConfigInitRefusesToDowngradeAgentProfileWhenInspectionUnreachable pins
// the one place fail-open must not apply: a profile already bound to an Agent
// Employee is never silently rewritten as an unclassified app on the strength
// of credentials nobody could verify.
func TestConfigInitRefusesToDowngradeAgentProfileWhenInspectionUnreachable(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = newStatefulKeychain()
	seed := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name:            "agent-prod",
		AppId:           "cli_aat",
		AppSecret:       core.PlainSecret("old-secret"),
		Brand:           core.BrandFeishu,
		Users:           []core.AppUser{},
		PrincipalOpenID: "ou_agent",
		CredentialKind:  core.CredentialAAT,
	}}}
	if err := core.SaveMultiAppConfig(seed); err != nil {
		t.Fatal(err)
	}

	err := configInitRun(&ConfigInitOptions{
		Factory: f, Ctx: context.Background(), AppID: "cli_aat", appSecret: "new-secret",
		Brand: "feishu", ProfileName: "agent-prod",
	})
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, want validation/failed_precondition refusing the downgrade", err, err)
	}
	multi, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	app := multi.FindApp("agent-prod")
	if app == nil || app.CredentialKind != core.CredentialAAT || app.PrincipalOpenID != "ou_agent" {
		t.Fatalf("profile was rewritten: %#v", app)
	}
}
