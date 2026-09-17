// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package identitydiag

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keychain"
)

// memoryKeychain stands in for the OS keychain so a test can seed a cached
// credential and observe that inspection reads it without renewing it.
type memoryKeychain struct{ items map[string]string }

func newMemoryKeychain() *memoryKeychain {
	return &memoryKeychain{items: map[string]string{}}
}

func (m *memoryKeychain) Get(service, account string) (string, error) {
	value, ok := m.items[service+"/"+account]
	if !ok {
		return "", keychain.ErrNotFound
	}
	return value, nil
}

func (m *memoryKeychain) Set(service, account, value string) error {
	m.items[service+"/"+account] = value
	return nil
}

func (m *memoryKeychain) Remove(service, account string) error {
	delete(m.items, service+"/"+account)
	return nil
}

func agentDiagConfig() *core.CliConfig {
	return &core.CliConfig{
		ProfileName:     "agent",
		AppID:           "cli_agent",
		AppSecret:       "secret",
		Brand:           core.BrandFeishu,
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
		UserOpenId:      "ou_agent",
	}
}

// TestInspectAgentProfileMakesNoRequests pins that describing an Agent Employee
// profile does not acquire a credential. The mock registry has no stubs, so any
// outbound request fails the test — checking your status must not change it.
func TestInspectAgentProfileMakesNoRequests(t *testing.T) {
	cfg := agentDiagConfig()
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Inspect(context.Background(), f, cfg)

	if got.Credential != string(core.CredentialAAT) {
		t.Fatalf("credential = %q, want aat", got.Credential)
	}
	if got.Principal != PrincipalAgentEmployee {
		t.Fatalf("principal = %q, want %q", got.Principal, PrincipalAgentEmployee)
	}
	if !got.User.Available || got.User.Status != StatusReady {
		t.Fatalf("user = %#v, want a ready user identity", got.User)
	}
	if got.User.OpenID != "ou_agent" {
		t.Fatalf("user openID = %q, want the verified principal", got.User.OpenID)
	}
	if got.User.Verified != nil {
		t.Fatal("a read-only inspection must not claim verification")
	}
	if got.Bot.Available || got.Bot.Status != StatusUnavailable {
		t.Fatalf("bot = %#v, want structurally unavailable", got.Bot)
	}
}

// TestInspectAgentProfileReportsCacheState pins that the read-only inspection
// still answers the question users actually ask — how long the credential in
// hand lasts — by peeking at the cache without renewing it.
func TestInspectAgentProfileReportsCacheState(t *testing.T) {
	cfg := agentDiagConfig()
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	kc := newMemoryKeychain()
	f.Keychain = kc
	now := time.Now()
	payload, err := json.Marshal(&larkauth.StoredAAToken{
		AppID:       cfg.AppID,
		UserOpenID:  cfg.PrincipalOpenID,
		AccessToken: "cached-aat",
		GrantedAt:   now.Add(-time.Minute).UnixMilli(),
		ExpiresAt:   now.Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := kc.Set(keychain.LarkCliService, larkauth.AATAccountKey(cfg.AppID), string(payload)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	got := Inspect(context.Background(), f, cfg)
	if got.User.TokenStatus != larkauth.AATStatusValid {
		t.Fatalf("tokenStatus = %q, want %q", got.User.TokenStatus, larkauth.AATStatusValid)
	}
	if got.User.ExpiresAt == "" {
		t.Fatal("a cached credential must report when it expires")
	}
}

// TestInspectAgentProfileWithoutPrincipal pins that a profile classified as an
// Agent Employee but missing its principal is reported as unconfigured rather
// than as ready.
func TestInspectAgentProfileWithoutPrincipal(t *testing.T) {
	cfg := agentDiagConfig()
	cfg.PrincipalOpenID = ""
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	got := Inspect(context.Background(), f, cfg)
	if got.User.Available || got.User.Status != StatusNotConfigured {
		t.Fatalf("user = %#v, want not_configured", got.User)
	}
}

// newLocalAgentFactory builds a factory whose credential chain is the real
// CLI-owned one: the profile on disk, the token cache in kc, and minting through
// the mock transport. --verify is then exercised against the code that actually
// resolves an Agent Employee credential rather than a stand-in.
func newLocalAgentFactory(t *testing.T, cfg *core.CliConfig, kc *memoryKeychain) (*cmdutil.Factory, *httpmock.Registry) {
	t.Helper()
	dir := t.TempDir()
	config := `{"currentApp":"agent","apps":[{"name":"agent","appId":"cli_agent","appSecret":"secret",` +
		`"brand":"feishu","credentialKind":"aat","principalOpenId":"ou_agent","users":[]}]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	f.Keychain = kc
	acct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return kc }, "", 0)
	f.Credential = credential.NewCredentialProvider(nil, acct,
		credential.NewDefaultTokenProvider(acct, f.HttpClient, io.Discard), f.HttpClient)
	return f, reg
}

// seedCachedAAT stores a fresh Agent token for cfg so that ResolveAccess is a
// pure cache hit.
func seedCachedAAT(t *testing.T, kc *memoryKeychain, cfg *core.CliConfig) {
	t.Helper()
	now := time.Now()
	payload, err := json.Marshal(&larkauth.StoredAAToken{
		AppID:       cfg.AppID,
		UserOpenID:  cfg.PrincipalOpenID,
		AccessToken: "cached-aat",
		GrantedAt:   now.Add(-time.Minute).UnixMilli(),
		ExpiresAt:   now.Add(time.Hour).UnixMilli(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := kc.Set(keychain.LarkCliService, larkauth.AATAccountKey(cfg.AppID), string(payload)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
}

func introspectionBody(active bool, userType int, openID string) string {
	exp := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	return `{"active":` + strconv.FormatBool(active) + `,"client_id":"cli_agent","token_type":"Bearer","exp":` + exp +
		`,"user_type":` + strconv.Itoa(userType) + `,"open_id":"` + openID + `"}`
}

// TestDiagnoseAgentVerifyIntrospectsCachedToken pins what --verify means: the
// server is asked. A cache hit answers "here is a token" without asking anyone
// whether the Agent behind it is still enabled, so a verified result must be
// backed by an Introspection call on the token actually in hand.
func TestDiagnoseAgentVerifyIntrospectsCachedToken(t *testing.T) {
	cfg := agentDiagConfig()
	kc := newMemoryKeychain()
	f, reg := newLocalAgentFactory(t, cfg, kc)
	seedCachedAAT(t, kc, cfg)
	introspect := &httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthIntrospect,
		Body:   introspectionBody(true, 66, "ou_agent"),
	}
	reg.Register(introspect)

	got := Diagnose(context.Background(), f, cfg, true)

	if got.User.Verified == nil || !*got.User.Verified {
		t.Fatalf("user = %#v, want verified", got.User)
	}
	if got.User.VerificationSource != "local_introspection" {
		t.Fatalf("verificationSource = %q, want local_introspection", got.User.VerificationSource)
	}
	if n := len(introspect.CapturedBodies); n != 1 {
		t.Fatalf("introspection calls = %d, want exactly 1", n)
	}
	if body := string(introspect.CapturedBody); !strings.Contains(body, "token=cached-aat") {
		t.Fatalf("introspection must be asked about the token in hand, got body %q", body)
	}
}

// TestDiagnoseAgentVerifyFailsWhenIntrospectionRejectsCachedToken pins the
// other half: a cached token the server no longer accepts is reported as a
// failed verification, not as ready.
func TestDiagnoseAgentVerifyFailsWhenIntrospectionRejectsCachedToken(t *testing.T) {
	cfg := agentDiagConfig()
	kc := newMemoryKeychain()
	f, reg := newLocalAgentFactory(t, cfg, kc)
	seedCachedAAT(t, kc, cfg)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthIntrospect,
		Body:   introspectionBody(false, 66, "ou_agent"),
	})

	got := Diagnose(context.Background(), f, cfg, true)

	if got.User.Status != StatusVerifyFailed || got.User.Available {
		t.Fatalf("user = %#v, want verify_failed and unavailable", got.User)
	}
	if got.User.Verified == nil || *got.User.Verified {
		t.Fatalf("verified = %v, want false", got.User.Verified)
	}
}

// TestDiagnoseAgentVerifyDoesNotIntrospectTwiceAfterMint pins that a token
// minted during this very resolution is not asked about again: the mint path
// already introspected it, and the stubs here accept exactly one call each.
func TestDiagnoseAgentVerifyDoesNotIntrospectTwiceAfterMint(t *testing.T) {
	cfg := agentDiagConfig()
	f, reg := newLocalAgentFactory(t, cfg, newMemoryKeychain())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    core.OAuthTokenV3Path,
		Body:   `{"code":0,"access_token":"fresh-aat","token_type":"Bearer","expires_in":7200}`,
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathOAuthIntrospect,
		Body:   introspectionBody(true, 66, "ou_agent"),
	})

	got := Diagnose(context.Background(), f, cfg, true)

	if got.User.Verified == nil || !*got.User.Verified {
		t.Fatalf("user = %#v, want verified after a fresh mint", got.User)
	}
	if got.User.VerificationSource != "local_introspection" {
		t.Fatalf("verificationSource = %q, want local_introspection", got.User.VerificationSource)
	}
}
