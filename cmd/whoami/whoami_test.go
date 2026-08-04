// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whoami

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/keychain"
)

func TestResolveSource(t *testing.T) {
	tests := []struct {
		name         string
		changedAs    bool
		flagAs       core.Identity
		autoDetected bool
		strictForced core.Identity
		want         string
	}{
		{"explicit flag user", true, core.AsUser, false, "", "flag"},
		{"explicit flag bot", true, core.AsBot, false, "", "flag"},
		{"flag auto falls through to auto-detect", true, core.AsAuto, true, "", "auto_detect"},
		{"auto detected", false, "", true, "", "auto_detect"},
		{"strict mode", false, "", false, core.AsBot, "strict_mode"},
		{"default_as", false, "", false, "", "default_as"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSource(tt.changedAs, tt.flagAs, tt.autoDetected, tt.strictForced)
			if got != tt.want {
				t.Errorf("resolveSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildResult_UserValid(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "my-app", AppID: "cli_x", Brand: core.BrandLark, DefaultAs: core.AsAuto}
	diag := identitydiag.Result{
		User: identitydiag.Identity{Available: true, Status: "ready", TokenStatus: "valid", OpenID: "ou_x", UserName: "Alice"},
	}
	r := buildResult(cfg, core.AsUser, "auto_detect", diag, credential.IdentitySelection{})

	if r.Identity != "user" || r.IdentitySource != "auto_detect" {
		t.Fatalf("identity/source = %q/%q", r.Identity, r.IdentitySource)
	}
	// tokenStatus mirrors the unified Status vocab ("ready"), not the raw "valid".
	if !r.Available || r.TokenStatus != "ready" {
		t.Fatalf("available=%v status=%q", r.Available, r.TokenStatus)
	}
	if r.OnBehalfOf == nil || r.OnBehalfOf.OpenID != "ou_x" || r.OnBehalfOf.UserName != "Alice" {
		t.Fatalf("onBehalfOf = %#v, want Alice/ou_x", r.OnBehalfOf)
	}
	if r.Hint != "" {
		t.Fatalf("hint = %q, want empty", r.Hint)
	}
	if r.Profile != "my-app" || r.AppID != "cli_x" || r.Brand != core.BrandLark {
		t.Fatalf("app context = %#v", r)
	}
}

func TestBuildResult_UserMissingToken(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandLark}
	diag := identitydiag.Result{
		User: identitydiag.Identity{Available: false, Status: "missing", Hint: "run: lark-cli auth login --help"}, // never logged in
	}
	r := buildResult(cfg, core.AsUser, "auto_detect", diag, credential.IdentitySelection{})

	if r.Available {
		t.Fatalf("available = true, want false")
	}
	if r.TokenStatus != "missing" {
		t.Fatalf("tokenStatus = %q, want missing", r.TokenStatus)
	}
	// whoami renders the diagnosed hint verbatim (single source of truth) so it
	// stays correct for the external-provider path without whoami knowing about it.
	if r.Hint != diag.User.Hint {
		t.Fatalf("hint = %q, want propagated %q", r.Hint, diag.User.Hint)
	}
	if r.DefaultAs != "auto" {
		t.Fatalf("defaultAs = %q, want auto (empty normalized)", r.DefaultAs)
	}
}

func TestBuildResult_BotReady(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu, DefaultAs: core.AsBot}
	diag := identitydiag.Result{
		Bot: identitydiag.Identity{Available: true, Status: "ready"},
	}
	r := buildResult(cfg, core.AsBot, "default_as", diag, credential.IdentitySelection{})

	if r.Identity != "bot" || r.IdentitySource != "default_as" {
		t.Fatalf("identity/source = %q/%q", r.Identity, r.IdentitySource)
	}
	if !r.Available || r.TokenStatus != "ready" {
		t.Fatalf("available=%v status=%q", r.Available, r.TokenStatus)
	}
	if r.OnBehalfOf != nil {
		t.Fatalf("bot must not carry onBehalfOf: %#v", r.OnBehalfOf)
	}
	if r.Hint != "" {
		t.Fatalf("hint = %q, want empty", r.Hint)
	}
}

func TestBuildResult_BotNotConfigured(t *testing.T) {
	cfg := &core.CliConfig{ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu}
	diag := identitydiag.Result{
		Bot: identitydiag.Identity{Available: false, Status: "not_configured", Hint: "run: lark-cli config --help"},
	}
	r := buildResult(cfg, core.AsBot, "auto_detect", diag, credential.IdentitySelection{})

	if r.Available {
		t.Fatalf("available = true, want false")
	}
	if r.TokenStatus != "not_configured" {
		t.Fatalf("tokenStatus = %q, want not_configured", r.TokenStatus)
	}
	if r.Hint != diag.Bot.Hint {
		t.Fatalf("hint = %q, want propagated %q", r.Hint, diag.Bot.Hint)
	}
}

func TestWhoami_BotJSON(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "test-profile", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{}) // bare whoami: output is always JSON, no flag needed
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var got whoamiResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, stdout.String())
	}
	if got.Identity != "bot" {
		t.Fatalf("identity = %q, want bot", got.Identity)
	}
	if !got.Available || got.TokenStatus != "ready" {
		t.Fatalf("available=%v status=%q, want true/ready", got.Available, got.TokenStatus)
	}
	if got.Profile != "test-profile" {
		t.Fatalf("profile = %q, want test-profile", got.Profile)
	}
	if got.IdentitySource == "" {
		t.Fatalf("identitySource empty")
	}
	if got.OnBehalfOf != nil {
		t.Fatalf("bot (self) must not carry onBehalfOf: %#v", got.OnBehalfOf)
	}
}

func TestWhoami_RejectsInvalidAs(t *testing.T) {
	for _, bad := range []string{"admin", "USER", "bogus123", ""} {
		t.Run("as="+bad, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				ProfileName: "p", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
			})
			cmd := NewCmdWhoami(f)
			cmd.SetArgs([]string{"--as", bad})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("Execute() with --as %q = nil, want validation error", bad)
			}
			// Lock in the typed validation contract: an unsupported identity must
			// surface as a *errs.ValidationError on --as, not just any error.
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Execute() with --as %q: error type = %T, want *errs.ValidationError: %v", bad, err, err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
			}
			if ve.Param != "--as" {
				t.Errorf("Param = %q, want %q", ve.Param, "--as")
			}
		})
	}
}

func TestWhoami_ConfigErrorPropagates(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "p", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	wantErr := fmt.Errorf("boom")
	f.Config = func() (*core.CliConfig, error) { return nil, wantErr }

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want propagated config error")
	}
	// The f.Config() failure must propagate unchanged, not be masked by a later
	// command-execution error.
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want it to wrap %v", err, wantErr)
	}
}

func TestWhoami_StrictModeRejectsCrossIdentity(t *testing.T) {
	// Bot-only account → strict mode bot. A real `--as user` call would be
	// rejected by CheckStrictMode; whoami must reject it identically rather than
	// previewing a user identity the next call would refuse.
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "p", AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		SupportedIdentities: 2, // bot only
	})
	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--as", "user", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() with --as user under strict bot = nil, want strict-mode rejection")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *errs.ValidationError: %v", err, err)
	}
}

type fakeExtProvider struct {
	name    string
	account *extcred.Account
}

func (p *fakeExtProvider) Name() string { return p.name }
func (p *fakeExtProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return p.account, nil
}
func (p *fakeExtProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil // no UAT served locally; whoami runs with verify=false
}

func externalWhoamiFactory(cfg *core.CliConfig) (*cmdutil.Factory, *bytes.Buffer) {
	cred := credential.NewCredentialProvider(
		[]extcred.Provider{&fakeExtProvider{name: "corp-sso", account: &extcred.Account{AppID: cfg.AppID}}},
		nil, nil,
		func() (*http.Client, error) { return nil, nil },
	)
	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		Config:     func() (*core.CliConfig, error) { return cfg, nil },
		Credential: cred,
		IOStreams:  &cmdutil.IOStreams{Out: out, ErrOut: &bytes.Buffer{}},
	}
	return f, out
}

// Regression for the external-provider blind spot: with credentials managed by
// an extension provider, a signed-in user must read as available, and an
// unavailable identity must not be told to "auth login" (which is blocked).
func TestWhoami_ExternalProvider_UserReady(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsAll), UserOpenId: "ou_x", UserName: "Alice",
	}
	f, out := externalWhoamiFactory(cfg)

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--as", "user", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got whoamiResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, out.String())
	}
	if got.Identity != "user" || !got.Available || got.TokenStatus != "ready" {
		t.Fatalf("got %#v, want user/available/ready", got)
	}
	if got.OnBehalfOf == nil || got.OnBehalfOf.UserName != "Alice" || got.OnBehalfOf.OpenID != "ou_x" {
		t.Fatalf("onBehalfOf = %#v, want Alice/ou_x (delegated)", got.OnBehalfOf)
	}
	if got.Hint != "" {
		t.Fatalf("hint = %q, want empty when available", got.Hint)
	}
}

func TestWhoami_ExternalProvider_UserHintNotKeychain(t *testing.T) {
	cfg := &core.CliConfig{
		ProfileName: "p", AppID: "cli_x", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsUser), // user supported but not signed in
	}
	f, out := externalWhoamiFactory(cfg)

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{"--as", "user", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var got whoamiResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, out.String())
	}
	if got.Identity != "user" || got.Available {
		t.Fatalf("got identity=%q available=%v, want user/false", got.Identity, got.Available)
	}
	if strings.Contains(got.Hint, "auth login") {
		t.Fatalf("hint must not point at auth login under external provider: %q", got.Hint)
	}
	if !strings.Contains(got.Hint, "external") {
		t.Fatalf("hint should explain external management: %q", got.Hint)
	}
}

// noopWhoamiKeychain is a no-op KeychainAccess; the profile below uses a
// plaintext secret, so no keychain lookup is actually required.
type noopWhoamiKeychain struct{}

func (noopWhoamiKeychain) Get(service, account string) (string, error) { return "", nil }
func (noopWhoamiKeychain) Set(service, account, value string) error    { return nil }
func (noopWhoamiKeychain) Remove(service, account string) error        { return nil }

// credentialSourceSecret is the profile secret written to config for
// TestWhoamiIncludesCredentialSource. It must never leak into whoami's output
// (security: never leak a secret).
const credentialSourceSecret = "test-secret"

// profileSelectionFactory builds a Factory whose CredentialProvider resolves
// an explicit profile ("tenant_a") supplied via the LARKSUITE_CLI_PROFILE env
// fallback (not --profile), so Selection().Source resolves to
// env:LARKSUITE_CLI_PROFILE and Explicit() is true, with no direct
// app-credential env vars present.
func profileSelectionFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer) {
	t.Helper()
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	multi := &core.MultiAppConfig{
		CurrentApp: "tenant_a",
		Apps: []core.AppConfig{{
			Name:      "tenant_a",
			AppId:     "cli_a",
			AppSecret: core.PlainSecret(credentialSourceSecret),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return noopWhoamiKeychain{} }, "tenant_a")
	cred := credential.NewCredentialProvider([]extcred.Provider{&envprovider.Provider{}}, defaultAcct, nil, nil)
	cred.WithProfileFromEnv("tenant_a")

	cfg := &core.CliConfig{ProfileName: "tenant_a", AppID: "cli_a", AppSecret: credentialSourceSecret, Brand: core.BrandFeishu}
	out := &bytes.Buffer{}
	f := &cmdutil.Factory{
		Config:     func() (*core.CliConfig, error) { return cfg, nil },
		Credential: cred,
		IOStreams:  &cmdutil.IOStreams{Out: out, ErrOut: &bytes.Buffer{}},
	}
	return f, out
}

// TestWhoamiIncludesCredentialSource locks in the diagnostic fields surfaced
// from the cached credential.IdentitySelection: credentialSource,
// explicit, and directCredentialEnv. whoami must read the cached selection
// as-is, not re-infer it.
func TestWhoamiIncludesCredentialSource(t *testing.T) {
	f, out := profileSelectionFactory(t)

	cmd := NewCmdWhoami(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	raw := out.String()
	if strings.Contains(raw, credentialSourceSecret) {
		t.Fatalf("whoami output leaked the profile secret: %s", raw)
	}

	var got whoamiResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, raw)
	}
	if got.CredentialSource != string(credential.SourceEnvProfile) {
		t.Fatalf("credentialSource = %q, want %q", got.CredentialSource, credential.SourceEnvProfile)
	}
	if !got.Explicit {
		t.Fatalf("explicit = false, want true")
	}
	if got.DirectCredentialEnv.Present {
		t.Fatalf("directCredentialEnv.present = true, want false: %#v", got.DirectCredentialEnv)
	}
	if !strings.Contains(raw, `"credentialSource": "env:LARKSUITE_CLI_PROFILE"`) {
		t.Fatalf("raw JSON missing credentialSource literal: %s", raw)
	}
	if got.DirectCredentialEnv.Present || len(got.DirectCredentialEnv.Keys) != 0 ||
		got.DirectCredentialEnv.AppID != "" || got.DirectCredentialEnv.Matched || got.DirectCredentialEnv.ConflictsWithProfile {
		t.Fatalf("directCredentialEnv = %#v, want only present:false set", got.DirectCredentialEnv)
	}
}
