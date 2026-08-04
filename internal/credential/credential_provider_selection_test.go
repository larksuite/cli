// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

func asConfigError(t *testing.T, err error) *errs.ConfigError {
	t.Helper()
	var ce *errs.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	return ce
}

func asValidationError(t *testing.T, err error) *errs.ValidationError {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	return ve
}

// secretValue is the profile secret written to config. It must NEVER appear in
// any error message or IdentitySelection (security: never leak a secret).
const secretValue = "your-secret"

// envSecretValue is the direct env app secret. Same no-leak guarantee.
const envSecretValue = "your-password"

// writeConfigTenantA writes a config with a single profile "tenant_a" (app_id
// "cli_a"). The secret is a plaintext secret stored in config, which resolves
// locally without a keychain lookup.
func writeConfigTenantA(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	multi := &core.MultiAppConfig{
		CurrentApp: "tenant_a",
		Apps: []core.AppConfig{{
			Name:      "tenant_a",
			AppId:     "cli_a",
			AppSecret: core.PlainSecret(secretValue),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
}

// writeConfigTenantABroken writes tenant_a with a keychain-backed secret ref
// that cannot be resolved (noop keychain returns empty), so profile secret
// resolution fails locally.
func writeConfigTenantABroken(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	// A keychain SecretRef whose key does NOT match app_id cli_a. Local secret
	// resolution fails (ValidateSecretKeyMatch), exercising profile_secret_invalid.
	multi := &core.MultiAppConfig{
		CurrentApp: "tenant_a",
		Apps: []core.AppConfig{{
			Name:      "tenant_a",
			AppId:     "cli_a",
			AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:wrong_key"}},
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}
}

func newProvider(t *testing.T, profile string, fromFlag bool) *credential.CredentialProvider {
	t.Helper()
	ep := &envprovider.Provider{}
	defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return &noopKC{} }, profile)
	cp := credential.NewCredentialProvider([]extcred.Provider{ep}, defaultAcct, nil, nil)
	if fromFlag {
		cp.WithProfileFromFlag(profile)
	} else {
		cp.WithProfileFromEnv(profile)
	}
	return cp
}

// assertNoSecretLeak fails if any secret value appears in the given strings.
func assertNoSecretLeak(t *testing.T, where string, vals ...string) {
	t.Helper()
	for _, v := range vals {
		if v == "" {
			continue
		}
		if strings.Contains(v, secretValue) {
			t.Errorf("%s leaked profile secret: %q", where, v)
		}
		if strings.Contains(v, envSecretValue) {
			t.Errorf("%s leaked env secret: %q", where, v)
		}
	}
}

func subtypeOf(t *testing.T, err error) errs.Subtype {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not a typed problem: %v", err)
	}
	return p.Subtype
}

// With no profile, direct credential, or config default, selection reports no_active_profile.
func TestSelection_NoActiveProfile(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir()) // empty dir -> no config
	cp := newProvider(t, "", false)

	sel, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeNoActiveProfile {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeNoActiveProfile)
	}
	// no_active_profile must carry credential_source=config.
	ce := asConfigError(t, err)
	if ce.CredentialSource != "config" {
		t.Errorf("credential_source = %q, want %q", ce.CredentialSource, "config")
	}
	assertNoSecretLeak(t, "state2", err.Error(), string(sel.Source))
}

// Config-default profile with an out-of-sync keychain ref: the precise typed
// cause (invalid_config) passes through on the default route too — parity
// with main, which surfaced this exact diagnosis before the profile feature.
func TestSelection_ConfigDefaultKeychainOutOfSyncSurfacesRealCause(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantABroken(t) // CurrentApp = tenant_a (app_id cli_a), out-of-sync keychain ref
	cp := newProvider(t, "", false)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeInvalidConfig {
		t.Fatalf("subtype = %q, want invalid_config (precise cause, not no_active_profile)", got)
	}
	prob, _ := errs.ProblemOf(err)
	if !strings.Contains(prob.Message, "out of sync") {
		t.Errorf("message = %q, want the out-of-sync diagnosis", prob.Message)
	}
	if !strings.Contains(prob.Hint, "appsecret:cli_a") {
		t.Errorf("hint = %q, want the expected keychain key named", prob.Hint)
	}
	assertNoSecretLeak(t, "config-default-out-of-sync", prob.Message, prob.Hint)
}

// writeMalformedConfig points the config dir at a temp file containing invalid
// JSON, with no direct-credential env set.
func writeMalformedConfig(t *testing.T) {
	t.Helper()
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := os.MkdirAll(core.GetConfigDir(), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(core.GetConfigPath(), []byte("{ this is not valid json"), 0o600); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}
}

// assertMalformedSurfaced requires the malformed config to surface as the typed
// invalid_config subtype — never masked (as profile_not_found on the explicit
// path, or no_active_profile on the config-default path), which would hide a
// broken file and misdirect the user (e.g. to `config init`, risking overwrite
// of a recoverable config).
func assertMalformedSurfaced(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error for malformed config, got nil")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("malformed config error is not typed: %v", err)
	}
	if prob.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("malformed config subtype = %q, want invalid_config (not masked)", prob.Subtype)
	}
}

// Explicit profile + malformed config → invalid_config, not profile_not_found.
func TestSelection_ExplicitProfile_MalformedConfig_PropagatesError(t *testing.T) {
	writeMalformedConfig(t)
	cp := newProvider(t, "tenant_a", true)

	_, err := cp.Selection(context.Background())
	assertMalformedSurfaced(t, err)
}

// Config-default path (no profile) + malformed config → invalid_config, not
// no_active_profile. Both paths route through core.LoadOrNotConfigured so a
// temporarily broken config is never misreported as "no active profile, run
// config init".
func TestSelection_ConfigDefault_MalformedConfig_PropagatesError(t *testing.T) {
	writeMalformedConfig(t)
	cp := newProvider(t, "", false)

	_, err := cp.Selection(context.Background())
	assertMalformedSurfaced(t, err)
}

// APP_ID without a secret or access token reports app_credential_incomplete.
func TestSelection_AppIDOnlyWithoutProfile(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "", false)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeAppCredentialIncomplete {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeAppCredentialIncomplete)
	}
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", got, output.ExitAuth)
	}
	prob, _ := errs.ProblemOf(err)
	ce := asConfigError(t, err)
	if len(ce.MissingKeys) != 0 {
		t.Errorf("missing_keys = %v, want empty because no single key is required", ce.MissingKeys)
	}
	wantAnyOf := []string{envvars.CliAppSecret, envvars.CliUserAccessToken, envvars.CliTenantAccessToken}
	if !slices.Equal(ce.RequiredAnyOf, wantAnyOf) {
		t.Errorf("required_any_of = %v, want %v", ce.RequiredAnyOf, wantAnyOf)
	}
	// required_any_of must be NAMES only, never values.
	for _, k := range ce.RequiredAnyOf {
		if strings.Contains(k, envSecretValue) || strings.Contains(k, secretValue) {
			t.Errorf("required_any_of contains a value, not a name: %q", k)
		}
	}
	wantHint := "set LARKSUITE_CLI_APP_SECRET, LARKSUITE_CLI_USER_ACCESS_TOKEN, or LARKSUITE_CLI_TENANT_ACCESS_TOKEN."
	if ce.Hint != wantHint {
		t.Errorf("hint = %q, want %q", ce.Hint, wantHint)
	}
	var blockErr *extcred.BlockError
	if !errors.As(err, &blockErr) || blockErr.Code != extcred.BlockReasonCredentialIncomplete {
		t.Fatalf("cause chain does not preserve credential BlockError: %T %v", err, err)
	}
	assertNoSecretLeak(t, "state3", prob.Message, prob.Hint)
}

func TestSelection_IncompleteDirectEnvWithoutProfile_UsesDirectRepairOnly(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "APP_SECRET-only", key: envvars.CliAppSecret},
		{name: "UAT-only", key: envvars.CliUserAccessToken},
		{name: "TAT-only", key: envvars.CliTenantAccessToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.key, "direct-value")
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

			cp := newProvider(t, "", false)
			_, err := cp.Selection(context.Background())
			ce := asConfigError(t, err)
			if got := output.ExitCodeOf(err); got != output.ExitAuth {
				t.Fatalf("exit code = %d, want %d", got, output.ExitAuth)
			}
			if ce.Message != tt.key+" is set but "+envvars.CliAppID+" is missing" {
				t.Errorf("message = %q, want provider's exact reason", ce.Message)
			}
			if ce.Hint != "set "+envvars.CliAppID+"." {
				t.Errorf("hint = %q, want direct repair only", ce.Hint)
			}
			if !slices.Equal(ce.MissingKeys, []string{envvars.CliAppID}) {
				t.Errorf("missing_keys = %v, want [%s]", ce.MissingKeys, envvars.CliAppID)
			}
		})
	}
}

// A complete direct credential without a profile is selected from APP_ID env.
func TestSelection_CompleteDirectEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliAppSecret, envSecretValue)
	writeConfigTenantA(t)
	cp := newProvider(t, "", false)

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.Source != credential.SourceEnvAppID {
		t.Fatalf("source = %q, want %q", sel.Source, credential.SourceEnvAppID)
	}
	if !sel.DirectCredentialEnv.Present {
		t.Errorf("DirectCredentialEnv.Present = false, want true")
	}
	assertNoSecretLeak(t, "state4", string(sel.Source), sel.DirectCredentialEnv.AppID)
	assertNoSecretLeak(t, "state4-keys", sel.DirectCredentialEnv.Keys...)
}

// A valid profile selected by flag reports flag:--profile as its source.
func TestSelection_ProfileOnlyFromFlag(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.Source != credential.SourceFlagProfile {
		t.Fatalf("source = %q, want %q", sel.Source, credential.SourceFlagProfile)
	}
	if sel.DirectCredentialEnv.Present {
		t.Errorf("DirectCredentialEnv.Present = true, want false")
	}
	assertNoSecretLeak(t, "state5", string(sel.Source))
}

// A valid profile selected by env reports LARKSUITE_CLI_PROFILE as its source.
func TestSelection_ProfileOnlyFromEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", false)

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.Source != credential.SourceEnvProfile {
		t.Fatalf("source = %q, want %q", sel.Source, credential.SourceEnvProfile)
	}
}

// An explicitly selected missing profile reports profile_not_found even when direct env is complete.
func TestSelection_MissingProfileWinsOverDirectEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_env")
	t.Setenv(envvars.CliAppSecret, envSecretValue)
	writeConfigTenantA(t)
	cp := newProvider(t, "does_not_exist", true)

	sel, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileNotFound {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileNotFound)
	}
	prob, _ := errs.ProblemOf(err)
	// profile_not_found must carry the credential_source that
	// named the profile — here the --profile flag.
	ce := asConfigError(t, err)
	if ce.CredentialSource != string(credential.SourceFlagProfile) {
		t.Errorf("credential_source = %q, want %q", ce.CredentialSource, credential.SourceFlagProfile)
	}
	assertNoSecretLeak(t, "state6", err.Error(), prob.Hint, string(sel.Source))
}

// REAL-path regression for review F1: a valid profile whose saved keychain
// ref is inconsistent with its app_id must surface the precise typed cause —
// invalid_config, "out of sync", hint naming the expected keychain key — via
// the actual DefaultAccountProvider, not be flattened into the generic
// profile_secret_invalid (which is reserved for untyped failures).
func TestSelection_ProfileKeychainOutOfSyncSurfacesRealCause(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantABroken(t)
	cp := newProvider(t, "tenant_a", true)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeInvalidConfig {
		t.Fatalf("subtype = %q, want invalid_config (precise cause, not the generic secret error)", got)
	}
	prob, _ := errs.ProblemOf(err)
	if !strings.Contains(prob.Message, "out of sync") {
		t.Errorf("message = %q, want the out-of-sync diagnosis", prob.Message)
	}
	if !strings.Contains(prob.Hint, "appsecret:cli_a") {
		t.Errorf("hint = %q, want the expected keychain key named", prob.Hint)
	}
	assertNoSecretLeak(t, "keychain-out-of-sync", prob.Message, prob.Hint)
}

// secretMarkerValue is a distinctive string used to prove that the
// profile_secret_invalid path drops the underlying error entirely, even when
// that underlying error's own message CONTAINS a secret. Unlike
// writeConfigTenantABroken (whose noop-keychain failure is a harmless empty
// error), this uses a custom DefaultAccountResolver whose error text embeds
// the marker, closing the gap where a leak could hide in a cause chain that
// happens to be empty in the noop-keychain case.
const secretMarkerValue = "your-access-token"

// leakingSecretResolver is a DefaultAccountResolver stub whose ResolveAccount
// fails with an error whose message contains secretMarkerValue, simulating a
// real keychain/secret-resolution failure that echoes back sensitive material
// (e.g. a keychain library including the attempted secret in its error text).
type leakingSecretResolver struct{}

func (leakingSecretResolver) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	return nil, fmt.Errorf("keychain decode failed for secret %s", secretMarkerValue)
}

// When account/secret resolution fails with an error that itself contains a
// secret, doResolveAccount emits a generic
// profile_secret_invalid ConfigError WITHOUT attaching the underlying cause,
// so a secret embedded in that underlying error can never surface through
// err.Error(), Message, Hint, the unwrapped cause chain, or Selection().
func TestSelection_ProfileSecretErrorDoesNotLeak(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantA(t) // profile "tenant_a" exists with app_id "cli_a"

	ep := &envprovider.Provider{}
	cp := credential.NewCredentialProvider([]extcred.Provider{ep}, leakingSecretResolver{}, nil, nil)
	cp.WithProfileFromFlag("tenant_a")

	sel, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileSecretInvalid {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileSecretInvalid)
	}
	ce := asConfigError(t, err)
	if ce.Profile != "tenant_a" {
		t.Errorf("profile = %q, want tenant_a", ce.Profile)
	}
	if ce.AppID != "cli_a" {
		t.Errorf("app_id = %q, want cli_a", ce.AppID)
	}

	// Walk the full unwrap chain. This is the assertion that would catch a
	// regression where the profile_secret_invalid branch starts attaching the
	// underlying error via WithCause: if it did, this loop would find the
	// marker in a wrapped link even though err.Error()/Message/Hint (which
	// only reflect the top-level ConfigError, not the chain) might look clean.
	for cur := error(ce); cur != nil; cur = errors.Unwrap(cur) {
		if strings.Contains(cur.Error(), secretMarkerValue) {
			t.Errorf("cause chain leaked secret marker: %v", cur)
		}
	}

	if strings.Contains(err.Error(), secretMarkerValue) {
		t.Errorf("err.Error() leaked secret marker: %q", err.Error())
	}
	if strings.Contains(ce.Message, secretMarkerValue) {
		t.Errorf("Message leaked secret marker: %q", ce.Message)
	}
	if strings.Contains(ce.Hint, secretMarkerValue) {
		t.Errorf("Hint leaked secret marker: %q", ce.Hint)
	}
	if strings.Contains(string(sel.Source), secretMarkerValue) {
		t.Errorf("Selection.Source leaked secret marker: %q", sel.Source)
	}
	if strings.Contains(sel.DirectCredentialEnv.AppID, secretMarkerValue) {
		t.Errorf("Selection.DirectCredentialEnv.AppID leaked secret marker: %q", sel.DirectCredentialEnv.AppID)
	}
	for _, k := range sel.DirectCredentialEnv.Keys {
		if strings.Contains(k, secretMarkerValue) {
			t.Errorf("Selection.DirectCredentialEnv.Keys leaked secret marker: %q", k)
		}
	}
	// The secret-invalid path leaves p.selection zero-valued; this trivially
	// implies no marker anywhere in it and guards against a future field being populated
	// from the failed resolution.
	if sel.Source != "" || sel.DirectCredentialEnv.Present ||
		sel.DirectCredentialEnv.AppID != "" || len(sel.DirectCredentialEnv.Keys) != 0 {
		t.Errorf("Selection() = %+v, want zero value on profile_secret_invalid", sel)
	}
}

// A profile matching a complete direct env credential wins and records the env as matched.
func TestSelection_ProfileMatchesCompleteDirectEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_a") // matches profile app_id
	t.Setenv(envvars.CliAppSecret, envSecretValue)
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.Source != credential.SourceFlagProfile {
		t.Fatalf("source = %q, want %q", sel.Source, credential.SourceFlagProfile)
	}
	if !sel.DirectCredentialEnv.Present || !sel.DirectCredentialEnv.Matched {
		t.Fatalf("DirectCredentialEnv = %+v, want Present && Matched", sel.DirectCredentialEnv)
	}
	if sel.DirectCredentialEnv.AppID != "cli_a" {
		t.Errorf("DirectCredentialEnv.AppID = %q, want cli_a", sel.DirectCredentialEnv.AppID)
	}
	assertNoSecretLeak(t, "state8", string(sel.Source), sel.DirectCredentialEnv.AppID)
	assertNoSecretLeak(t, "state8-keys", sel.DirectCredentialEnv.Keys...)
}

// APP_ID-only is sufficient for profile arbitration: a matching selected
// profile supplies all credentials and tokens, so the incomplete direct env
// must not block the profile.
func TestSelection_ProfileMatchesAppIDOnly_ProfileWins(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_a")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acct.AppID != "cli_a" || acct.AppSecret != secretValue {
		t.Fatalf("account = %+v, want selected profile credentials", acct)
	}
	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("Selection: %v", err)
	}
	if !sel.DirectCredentialEnv.Present || !sel.DirectCredentialEnv.Matched {
		t.Fatalf("DirectCredentialEnv = %+v, want Present && Matched", sel.DirectCredentialEnv)
	}
}

func TestSelection_ProfileMatchingEnvTokens_UsesProfileTokenSource(t *testing.T) {
	for _, tt := range []struct {
		name      string
		tokenType credential.TokenType
		envKey    string
	}{
		{name: "UAT", tokenType: credential.TokenTypeUAT, envKey: envvars.CliUserAccessToken},
		{name: "TAT", tokenType: credential.TokenTypeTAT, envKey: envvars.CliTenantAccessToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "cli_a")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.envKey, "env-token")
			writeConfigTenantA(t)

			ep := &envprovider.Provider{}
			defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return &noopKC{} }, "tenant_a")
			defaultToken := &mockDefaultTokenProvider{token: "profile-token"}
			cp := credential.NewCredentialProvider([]extcred.Provider{ep}, defaultAcct, defaultToken, nil)
			cp.WithProfileFromFlag("tenant_a")

			result, err := cp.ResolveToken(context.Background(), credential.TokenSpec{Type: tt.tokenType, AppID: "cli_a"})
			if err != nil {
				t.Fatalf("ResolveToken: %v", err)
			}
			if result.Token != "profile-token" {
				t.Fatalf("token = %q, want profile-token (env token must not override selected profile)", result.Token)
			}
			sel, err := cp.Selection(context.Background())
			if err != nil {
				t.Fatalf("Selection: %v", err)
			}
			if sel.Source != credential.SourceFlagProfile || !sel.DirectCredentialEnv.Present || !sel.DirectCredentialEnv.Matched {
				t.Fatalf("Selection = %+v, want profile source with matched direct env", sel)
			}
		})
	}
}

// A profile that conflicts with a complete direct env credential reports a hard conflict.
func TestSelection_ProfileConflictsWithCompleteDirectEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_x") // mismatches profile app_id cli_a
	t.Setenv(envvars.CliAppSecret, envSecretValue)
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileAppCredentialConflict {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileAppCredentialConflict)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	ve := asValidationError(t, err)
	if ve.ProfileAppID != "cli_a" {
		t.Errorf("profile_app_id = %q, want cli_a", ve.ProfileAppID)
	}
	if ve.EnvAppID != "cli_x" {
		t.Errorf("env_app_id = %q, want cli_x", ve.EnvAppID)
	}
	if !strings.Contains(ve.Hint, "unset "+envvars.CliAppID+" and "+envvars.CliAppSecret) {
		t.Errorf("hint = %q, want exact conflicting env variable names", ve.Hint)
	}
	assertNoSecretLeak(t, "state9", ve.Message, ve.Hint)
}

func TestSelection_ProfileConflictsWithAppIDOnly(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_x")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileAppCredentialConflict {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileAppCredentialConflict)
	}
	ve := asValidationError(t, err)
	if ve.ProfileAppID != "cli_a" || ve.EnvAppID != "cli_x" {
		t.Fatalf("conflict = profile:%q env:%q, want cli_a/cli_x", ve.ProfileAppID, ve.EnvAppID)
	}
	if !strings.Contains(ve.Hint, "unset "+envvars.CliAppID) {
		t.Errorf("hint = %q, want exact APP_ID unset instruction", ve.Hint)
	}
}

func TestSelection_ExplicitMissingProfileWinsOverIncompleteEnv(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_a")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_typo", false)

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileNotFound {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileNotFound)
	}
	ce := asConfigError(t, err)
	if ce.Profile != "tenant_typo" || ce.CredentialSource != string(credential.SourceEnvProfile) {
		t.Fatalf("profile error = %+v, want tenant_typo from env profile", ce)
	}
}

func TestSelection_ExplicitProfileMalformedConfigWinsOverIncompleteEnv(t *testing.T) {
	writeMalformedConfig(t)
	t.Setenv(envvars.CliAppSecret, "direct-value")
	cp := newProvider(t, "tenant_a", true)

	_, err := cp.Selection(context.Background())
	assertMalformedSurfaced(t, err)
}

// A direct secret or token without APP_ID remains incomplete even when a valid profile is selected.
func TestSelection_ProfileWithDirectEnvMissingAppID(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "APP_SECRET-only", key: envvars.CliAppSecret},
		{name: "UAT-only", key: envvars.CliUserAccessToken},
		{name: "TAT-only", key: envvars.CliTenantAccessToken},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.key, "direct-value")
			writeConfigTenantA(t)
			cp := newProvider(t, "tenant_a", true)

			_, err := cp.Selection(context.Background())
			if got := subtypeOf(t, err); got != errs.SubtypeAppCredentialIncomplete {
				t.Fatalf("subtype = %q, want %q", got, errs.SubtypeAppCredentialIncomplete)
			}
			if got := output.ExitCodeOf(err); got != output.ExitAuth {
				t.Fatalf("exit code = %d, want %d", got, output.ExitAuth)
			}
			ce := asConfigError(t, err)
			if !slices.Equal(ce.MissingKeys, []string{envvars.CliAppID}) {
				t.Errorf("missing_keys = %v, want [%s]", ce.MissingKeys, envvars.CliAppID)
			}
			wantHint := "set " + envvars.CliAppID + ", or unset " + tt.key + " to use the selected profile."
			if ce.Hint != wantHint {
				t.Errorf("hint = %q, want %q", ce.Hint, wantHint)
			}
			var blockErr *extcred.BlockError
			if !errors.As(err, &blockErr) || blockErr.Code != extcred.BlockReasonCredentialIncomplete {
				t.Fatalf("cause chain does not preserve credential BlockError: %T %v", err, err)
			}
			assertNoSecretLeak(t, "state10", ce.Message, ce.Hint)
			assertNoSecretLeak(t, "state10-keys", ce.MissingKeys...)
		})
	}
}

// An invalid policy variable is a user input mistake: it must surface as a
// typed validation error (exit 2) carrying the variable name in param and a
// repair hint — never as an internal error, and never rewritten into
// app_credential_incomplete. The original BlockError stays on the cause chain.
func TestSelection_InvalidPolicyEnvBlockBecomesTypedValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "invalid DEFAULT_AS", key: envvars.CliDefaultAs},
		{name: "invalid STRICT_MODE", key: envvars.CliStrictMode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "cli_a")
			t.Setenv(envvars.CliAppSecret, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv(tt.key, "banana")
			writeConfigTenantA(t)
			cp := newProvider(t, "tenant_a", true)

			_, err := cp.Selection(context.Background())
			if got := subtypeOf(t, err); got != errs.SubtypeInvalidArgument {
				t.Fatalf("subtype = %q, want %q", got, errs.SubtypeInvalidArgument)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want validation %d", got, output.ExitValidation)
			}
			ve := asValidationError(t, err)
			if ve.Param != tt.key {
				t.Errorf("param = %q, want %q", ve.Param, tt.key)
			}
			if !strings.Contains(ve.Message, tt.key) || !strings.Contains(ve.Message, "banana") {
				t.Errorf("message = %q, want the variable name and its invalid value", ve.Message)
			}
			if !strings.Contains(ve.Hint, tt.key) {
				t.Errorf("hint = %q, want repair guidance naming %s", ve.Hint, tt.key)
			}
			var blockErr *extcred.BlockError
			if !errors.As(err, &blockErr) || blockErr.Code != extcred.BlockReasonInvalidPolicy {
				t.Fatalf("cause chain does not preserve the invalid-policy BlockError: %T %v", err, err)
			}
			if strings.Contains(err.Error(), string(errs.SubtypeAppCredentialIncomplete)) {
				t.Fatalf("error was rewritten as app_credential_incomplete: %v", err)
			}
		})
	}
}

func TestActiveExtensionProviderName_MatchingAppIDOnlyProfileUsesBuiltin(t *testing.T) {
	t.Setenv(envvars.CliAppID, "cli_a")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)
	cp := newProvider(t, "tenant_a", true)

	name, err := cp.ActiveExtensionProviderName(context.Background())
	if err != nil {
		t.Fatalf("ActiveExtensionProviderName: %v", err)
	}
	if name != "" {
		t.Fatalf("provider name = %q, want builtin profile (empty)", name)
	}
}

// A failed profile resolution must neither propagate its error nor report an
// external takeover: this probe guards the builtin setup/repair commands
// (auth, config), which must stay usable to fix exactly these states. It
// falls back to the engagement probe instead.
func TestActiveExtensionProviderName_ProfileResolutionFailureFallsBackToProbe(t *testing.T) {
	t.Run("no provider engaged, stale profile -> builtin allowed", func(t *testing.T) {
		t.Setenv(envvars.CliAppID, "")
		t.Setenv(envvars.CliAppSecret, "")
		t.Setenv(envvars.CliUserAccessToken, "")
		t.Setenv(envvars.CliTenantAccessToken, "")
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir()) // no config -> profile_not_found
		cp := newProvider(t, "ghost", false)

		name, err := cp.ActiveExtensionProviderName(context.Background())
		if err != nil || name != "" {
			t.Fatalf("ActiveExtensionProviderName = %q, %v; want \"\", nil", name, err)
		}
	})
	t.Run("engaged env provider still reported on conflict", func(t *testing.T) {
		t.Setenv(envvars.CliAppID, "cli_x") // conflicts with tenant_a -> resolution fails
		t.Setenv(envvars.CliAppSecret, envSecretValue)
		t.Setenv(envvars.CliUserAccessToken, "")
		t.Setenv(envvars.CliTenantAccessToken, "")
		writeConfigTenantA(t)
		cp := newProvider(t, "tenant_a", true)

		name, err := cp.ActiveExtensionProviderName(context.Background())
		if err != nil || name != "env" {
			t.Fatalf("ActiveExtensionProviderName = %q, %v; want \"env\", nil", name, err)
		}
	})
}

// stubFailureResolver fails ResolveAccount with a fixed error.
type stubFailureResolver struct{ err error }

func (s *stubFailureResolver) ResolveAccount(context.Context) (*credential.Account, error) {
	return nil, s.err
}

// stubAccountResolver returns a fixed account.
type stubAccountResolver struct{ acct *credential.Account }

func (s *stubAccountResolver) ResolveAccount(context.Context) (*credential.Account, error) {
	return s.acct, nil
}

// Contract test: ANY typed resolver failure (other than not_configured) on
// the profile route passes through with its own subtype and hint. The
// real-path proof for the keychain out-of-sync case is
// TestSelection_ProfileKeychainOutOfSyncSurfacesRealCause above.
func TestSelection_ProfileTypedResolverFailurePassesThrough(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)

	typed := errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"credential backend rejected the stored profile").
		WithHint("repair the stored profile input.")
	cp := credential.NewCredentialProvider(
		[]extcred.Provider{&envprovider.Provider{}}, &stubFailureResolver{err: typed}, nil, nil)
	cp.WithProfileFromFlag("tenant_a")

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want the typed failure passed through", got)
	}
	if strings.Contains(err.Error(), "could not be resolved locally") {
		t.Fatalf("typed failure was flattened into the generic secret error: %v", err)
	}
}

// An untyped resolver failure must stay masked behind the generic secret
// error: its content is not guaranteed secret-free.
func TestSelection_ProfileUntypedResolverFailureStaysGeneric(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)

	cp := credential.NewCredentialProvider(
		[]extcred.Provider{&envprovider.Provider{}},
		&stubFailureResolver{err: fmt.Errorf("read keychain: %s", secretValue)}, nil, nil)
	cp.WithProfileFromFlag("tenant_a")

	_, err := cp.Selection(context.Background())
	if got := subtypeOf(t, err); got != errs.SubtypeProfileSecretInvalid {
		t.Fatalf("subtype = %q, want %q", got, errs.SubtypeProfileSecretInvalid)
	}
	assertNoSecretLeak(t, "untyped-resolver-failure", err.Error())
}

// The resolver re-reads the config after arbitration validated the snapshot;
// if a concurrent edit hands back a different app, the mismatch must be
// refused rather than silently using credentials arbitration never checked.
func TestSelection_ProfileResolverAppIDMismatchRefused(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t) // snapshot sees tenant_a -> cli_a

	cp := credential.NewCredentialProvider(
		[]extcred.Provider{&envprovider.Provider{}},
		&stubAccountResolver{acct: &credential.Account{AppID: "cli_other", AppSecret: secretValue}}, nil, nil)
	cp.WithProfileFromFlag("tenant_a")

	_, err := cp.Selection(context.Background())
	if err == nil {
		t.Fatal("expected error for app_id mismatch between snapshot and resolver")
	}
	if !strings.Contains(err.Error(), "config changed during resolution") {
		t.Fatalf("error = %v, want config-changed refusal", err)
	}
}

// A config that exists but cannot be read (permission denied) must surface
// invalid_config with the real cause — not profile_not_found (the profile may
// well exist in the unreadable file) and not no_active_profile.
func TestSelection_UnreadableConfigSurfacesRealCause(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX file permissions")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	writeConfigTenantA(t)
	path := core.GetConfigPath()
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	t.Run("explicit profile", func(t *testing.T) {
		cp := newProvider(t, "tenant_a", true)
		_, err := cp.Selection(context.Background())
		if got := subtypeOf(t, err); got != errs.SubtypeInvalidConfig {
			t.Fatalf("subtype = %q, want invalid_config (not profile_not_found)", got)
		}
	})
	t.Run("config default", func(t *testing.T) {
		cp := newProvider(t, "", false)
		_, err := cp.Selection(context.Background())
		if got := subtypeOf(t, err); got != errs.SubtypeInvalidConfig {
			t.Fatalf("subtype = %q, want invalid_config (not no_active_profile)", got)
		}
	})
}

// fakeSidecarProvider is a NON-env extension provider (Priority 0, Name !=
// directCredentialProviderName) that always returns a non-nil account. It
// stands in for the sidecar extension provider without needing a build tag.
type fakeSidecarProvider struct {
	appID string
}

func (f *fakeSidecarProvider) Name() string  { return "sidecar" }
func (f *fakeSidecarProvider) Priority() int { return 0 }
func (f *fakeSidecarProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: f.appID, Brand: extcred.Brand("feishu")}, nil
}
func (f *fakeSidecarProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	return &extcred.Token{Value: "sidecar-tok", Source: "sidecar"}, nil
}

// Regression: a NON-env extension provider (sidecar) that returns a managed
// account must win outright even when a profile is set. It must NOT be treated
// as a direct-credential env account: no profile arbitration, no
// profile_app_credential_conflict (even though its app_id differs from the
// profile's cli_a), and DirectCredentialEnv.Present must stay false because no
// direct env vars are set. The selection source names the provider explicitly.
func TestSelection_NonEnvExtensionProviderWinsOverProfile(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")     // no direct env credential
	t.Setenv(envvars.CliAppSecret, "") // no direct env credential
	writeConfigTenantA(t)              // profile tenant_a exists, app_id cli_a

	sidecar := &fakeSidecarProvider{appID: "sidecar_app"} // differs from cli_a
	defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return &noopKC{} }, "tenant_a")
	cp := credential.NewCredentialProvider([]extcred.Provider{sidecar}, defaultAcct, nil, nil)
	cp.WithProfileFromFlag("tenant_a")

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The sidecar account is used as-is, NOT overridden by profile arbitration.
	if acct == nil || acct.AppID != "sidecar_app" {
		t.Fatalf("account = %+v, want AppID sidecar_app (sidecar wins outright)", acct)
	}

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected Selection error: %v", err)
	}
	// No misreported direct env credential.
	if sel.DirectCredentialEnv.Present {
		t.Errorf("DirectCredentialEnv.Present = true, want false (no direct env vars set)")
	}
	if sel.Source != credential.SourceExtension("sidecar") {
		t.Errorf("source = %q, want %q", sel.Source, credential.SourceExtension("sidecar"))
	}
	// The mismatched app_id (sidecar_app vs profile cli_a) must NOT trigger a
	// profile_app_credential_conflict: both ResolveAccount and Selection above
	// returned nil errors, so no conflict (or any other) error was produced.
	// Guard against a future regression that surfaces a conflict via Selection.
	if _, selErr := cp.Selection(context.Background()); selErr != nil {
		if subtypeOf(t, selErr) == errs.SubtypeProfileAppCredentialConflict {
			t.Errorf("got profile_app_credential_conflict, want none for non-env provider")
		}
	}
	assertNoSecretLeak(t, "nonenv-sidecar", string(sel.Source), sel.DirectCredentialEnv.AppID)
}

// Without an explicit profile or direct credential, selection uses the configured current app.
func TestSelection_ConfigDefault(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	writeConfigTenantA(t) // CurrentApp = tenant_a
	cp := newProvider(t, "", false)

	sel, err := cp.Selection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.Source != credential.SourceConfigCurrentApp {
		t.Fatalf("source = %q, want %q", sel.Source, credential.SourceConfigCurrentApp)
	}
}

// forgedDirectProvider impersonates the builtin env provider by name and
// declares AccountDirect. The reservation must hold by concrete type, not by
// the forgeable Name() string.
type forgedDirectProvider struct{}

func (forgedDirectProvider) Name() string  { return "env" }
func (forgedDirectProvider) Priority() int { return 0 }
func (forgedDirectProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "forged_app", Kind: extcred.AccountDirect}, nil
}
func (forgedDirectProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func TestSelection_ForgedDirectProviderRejected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cp := credential.NewCredentialProvider([]extcred.Provider{forgedDirectProvider{}}, nil, nil, nil)
	_, err := cp.ResolveAccount(context.Background())
	if err == nil || !strings.Contains(err.Error(), "reserved for the builtin env provider") {
		t.Fatalf("err = %v, want AccountDirect reservation failure for a name-forging provider", err)
	}
}

// forgedIncompleteProvider exercises the public SPI boundary: although it
// supplies provider-owned metadata, credential_incomplete is reserved for the
// builtin env provider while arbitration diagnostics name LARKSUITE_CLI_*.
type forgedIncompleteProvider struct{}

func (forgedIncompleteProvider) Name() string  { return "vault" }
func (forgedIncompleteProvider) Priority() int { return 0 }
func (forgedIncompleteProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, &extcred.BlockError{
		Provider:    "vault",
		Reason:      "vault app credential is incomplete",
		Code:        extcred.BlockReasonCredentialIncomplete,
		AppID:       "vault_app",
		PresentKeys: []string{"VAULT_APP_ID"},
	}
}
func (forgedIncompleteProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func TestSelection_NonEnvCredentialIncompleteRejected(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	providers := []extcred.Provider{forgedIncompleteProvider{}}
	probeCP := credential.NewCredentialProvider(providers, nil, nil, nil)
	name, probeErr := probeCP.ActiveExtensionProviderName(context.Background())
	if name != "" {
		t.Fatalf("provider name = %q, want none for an invalid SPI classification", name)
	}

	arbCP := credential.NewCredentialProvider(providers, nil, nil, nil)
	_, arbErr := arbCP.ResolveAccount(context.Background())
	for label, err := range map[string]error{"probe": probeErr, "arbitration": arbErr} {
		if err == nil || !strings.Contains(err.Error(), "reserved for the builtin env provider") {
			t.Fatalf("%s err = %v, want credential_incomplete reservation failure for a non-env provider", label, err)
		}
		if got := subtypeOf(t, err); got != errs.SubtypeUnknown {
			t.Fatalf("%s subtype = %q, want unknown internal contract violation", label, got)
		}
	}
	if probeErr.Error() != arbErr.Error() {
		t.Fatalf("probe and arbitration diverge:\n  probe: %v\n  arbitration: %v", probeErr, arbErr)
	}
}

// policyBlockProvider blocks with an invalid_policy classification, standing
// in for the env provider having seen a bad LARKSUITE_CLI_DEFAULT_AS.
type policyBlockProvider struct{}

func (policyBlockProvider) Name() string  { return "env" }
func (policyBlockProvider) Priority() int { return 0 }
func (policyBlockProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, &extcred.BlockError{
		Provider: "env",
		Reason:   "invalid LARKSUITE_CLI_DEFAULT_AS \"banana\" (want user, bot, or auto)",
		Code:     extcred.BlockReasonInvalidPolicy,
		Param:    envvars.CliDefaultAs,
	}
}
func (policyBlockProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

// The gate probe must stay aligned with formal arbitration when multiple
// providers are registered: provider A's invalid_policy block surfaces as the
// same typed validation error in both, instead of the probe scanning on and
// blaming provider B as an external takeover.
func TestActiveExtensionProviderName_InvalidPolicyAlignsWithArbitration(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	providers := []extcred.Provider{policyBlockProvider{}, &fakeSidecarProvider{appID: "sidecar_app"}}

	probeCP := credential.NewCredentialProvider(providers, nil, nil, nil)
	name, probeErr := probeCP.ActiveExtensionProviderName(context.Background())
	if name != "" {
		t.Fatalf("provider name = %q, want none (no external takeover)", name)
	}
	if got := subtypeOf(t, probeErr); got != errs.SubtypeInvalidArgument {
		t.Fatalf("probe subtype = %q, want invalid_argument", got)
	}

	arbCP := credential.NewCredentialProvider(providers, nil, nil, nil)
	_, arbErr := arbCP.ResolveAccount(context.Background())
	if got := subtypeOf(t, arbErr); got != errs.SubtypeInvalidArgument {
		t.Fatalf("arbitration subtype = %q, want invalid_argument", got)
	}
	if probeErr.Error() != arbErr.Error() {
		t.Fatalf("probe and arbitration diverge:\n  probe: %v\n  arbitration: %v", probeErr, arbErr)
	}
}
