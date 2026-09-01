// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

type noopConfigKeychain struct{}

func (n *noopConfigKeychain) Get(service, account string) (string, error) { return "", nil }
func (n *noopConfigKeychain) Set(service, account, value string) error    { return nil }
func (n *noopConfigKeychain) Remove(service, account string) error        { return nil }

type recordingConfigKeychain struct {
	removed []string
}

func (r *recordingConfigKeychain) Get(service, account string) (string, error) { return "", nil }
func (r *recordingConfigKeychain) Set(service, account, value string) error    { return nil }
func (r *recordingConfigKeychain) Remove(service, account string) error {
	r.removed = append(r.removed, service+":"+account)
	return nil
}

func TestConfigInitCmd_FlagParsing(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret123\n")

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--app-id", "cli_test", "--app-secret-stdin", "--brand", "lark"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.AppID != "cli_test" {
		t.Errorf("expected AppID cli_test, got %s", gotOpts.AppID)
	}
	if !gotOpts.AppSecretStdin {
		t.Error("expected AppSecretStdin=true")
	}
	if gotOpts.Brand != "lark" {
		t.Errorf("expected Brand lark, got %s", gotOpts.Brand)
	}
}

func TestConfigShowCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *ConfigShowOptions
	cmd := NewCmdConfigShow(f, func(opts *ConfigShowOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Error("expected opts to be set")
	}
}

func TestConfigShowRun_NotConfiguredReturnsStructuredError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error")
	}

	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *errs.ConfigError", err)
	}
	// Config errors share ExitAuth (3), not ExitValidation.
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d (config category → ExitAuth)", got, output.ExitAuth)
	}
	if cfgErr.Subtype != errs.SubtypeNotConfigured || cfgErr.Message != "not configured" {
		t.Fatalf("detail = %+v, want not_configured/not configured", cfgErr)
	}
}

func TestConfigShowRun_NoActiveProfileReturnsStructuredError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	multi := &core.MultiAppConfig{
		CurrentApp: "missing",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error")
	}

	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitAuth {
		t.Errorf("exit code = %d, want %d", gotCode, output.ExitAuth)
	}
	// The dangling persisted reference must be named — the generic
	// "no active profile" wording hid which input was broken.
	if !strings.Contains(err.Error(), `profile "missing" not found`) {
		t.Fatalf("error = %v, want the dangling currentApp named", err)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T", err)
	}
	if cfgErr.Field != "currentApp" {
		t.Errorf("field = %q, want currentApp", cfgErr.Field)
	}
	if strings.Contains(cfgErr.Hint, "config init") {
		t.Errorf("hint = %q, must not suggest config init while intact profiles exist", cfgErr.Hint)
	}
}

func TestConfigInitCmd_LangFlag(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	f.IOStreams.In = strings.NewReader("y\n")
	cmd.SetArgs([]string{"--app-id", "x", "--app-secret-stdin", "--lang", "en"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// --lang en is canonicalized to en_us in RunE before runF captures opts.
	if gotOpts.Lang != string(i18n.LangEnUS) {
		t.Errorf("expected Lang en_us, got %s", gotOpts.Lang)
	}
	if !gotOpts.langExplicit {
		t.Error("expected langExplicit=true when --lang is passed")
	}
}

func TestConfigInitCmd_LangDefault(t *testing.T) {
	clearAgentEnv(t) // assumes local workspace; guard refuses init in agent contexts
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigInitOptions
	cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
		gotOpts = opts
		return nil
	})
	f.IOStreams.In = strings.NewReader("y\n")
	cmd.SetArgs([]string{"--app-id", "x", "--app-secret-stdin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Lang != "" {
		t.Errorf("expected default Lang to be unset (\"\"), got %q", gotOpts.Lang)
	}
	if gotOpts.langExplicit {
		t.Error("expected langExplicit=false when --lang is not passed")
	}
}

// TestSaveInitConfig_OmitLangPreservesPrior guards the single-app replace path:
// re-running init without --lang must inherit the prior preference, not clear it.
func TestSaveInitConfig_OmitLangPreservesPrior(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	existing := &core.MultiAppConfig{Apps: []core.AppConfig{
		{AppId: "cli_x", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu, Lang: i18n.LangJaJP},
	}}
	if err := core.SaveMultiAppConfig(existing); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := saveInitConfig("", existing, f, "cli_x", core.PlainSecret("s2"), core.BrandFeishu, ""); err != nil {
		t.Fatalf("saveInitConfig (no --lang): %v", err)
	}

	got, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if app := got.CurrentAppConfig(""); app == nil || app.Lang != i18n.LangJaJP {
		t.Errorf("Lang after re-init = %v, want %q (preserved)", app, i18n.LangJaJP)
	}
}

// TestConfigInitCmd_InvalidLang verifies a non-empty --lang on config init is
// strictly validated the same way bind validates: wrong-case / typo / removed
// codes / hyphen form all exit with ExitValidation. (Empty is a no-op.)
func TestConfigInitCmd_InvalidLang(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cases := []struct {
		name string
		lang string
	}{
		{"wrong case ZH", "ZH"},
		{"typo frr", "frr"},
		{"removed code ar", "ar"},
		{"unknown xx", "xx"},
		{"hyphen form zh-CN", "zh-CN"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			cmd := NewCmdConfigInit(f, nil)
			f.IOStreams.In = strings.NewReader("sec\n")
			cmd.SetArgs([]string{"--lang", tc.lang, "--app-id", "x", "--app-secret-stdin"})
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected validation error for --lang %q, got nil", tc.lang)
			}
			var valErr *errs.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
			}
			if valErr.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("subtype = %q, want %q", valErr.Subtype, errs.SubtypeInvalidArgument)
			}
			if valErr.Param != "--lang" {
				t.Errorf("param = %q, want %q", valErr.Param, "--lang")
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Errorf("exit code = %d, want %d (validation)", got, output.ExitValidation)
			}
			assertLangErrorGuidance(t, err)
		})
	}
}

// assertLangErrorGuidance pins what a rejected --lang value must tell the user:
// which flag was wrong, that short codes are accepted too, and that the match
// is case-sensitive — the two facts that otherwise send a user who typed "EN"
// on a detour through "en_US".
func assertLangErrorGuidance(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	for _, want := range []string{"invalid --lang", "zh_cn (zh)", "en_us (en)", "case-sensitive"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not contain %q", msg, want)
		}
	}
}

func TestHasAnyNonInteractiveFlag(t *testing.T) {
	tests := []struct {
		name string
		opts ConfigInitOptions
		want bool
	}{
		{"empty", ConfigInitOptions{}, false},
		{"new", ConfigInitOptions{New: true}, true},
		{"app-id", ConfigInitOptions{AppID: "x"}, true},
		{"app-secret-stdin", ConfigInitOptions{AppSecretStdin: true}, true},
		{"app-id+secret-stdin", ConfigInitOptions{AppID: "x", AppSecretStdin: true}, true},
		{"lang-only", ConfigInitOptions{Lang: "en"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.hasAnyNonInteractiveFlag()
			if got != tt.want {
				t.Errorf("hasAnyNonInteractiveFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigInitRun_NonTerminal_NoFlags_RejectsWithHint(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// TestFactory has IsTerminal=false by default
	opts := &ConfigInitOptions{Factory: f, Ctx: context.Background(), Lang: "zh"}
	err := configInitRun(opts)
	if err == nil {
		t.Fatal("expected error for non-terminal without flags")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--new") {
		t.Errorf("expected error to mention --new, got: %s", msg)
	}
	if !strings.Contains(msg, "terminal") {
		t.Errorf("expected error to mention terminal, got: %s", msg)
	}
}

func TestConfigRemoveCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *ConfigRemoveOptions
	cmd := NewCmdConfigRemove(f, func(opts *ConfigRemoveOptions) error {
		gotOpts = opts
		return nil
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts == nil {
		t.Fatal("expected opts to be set")
	}
	if gotOpts.Factory != f {
		t.Fatal("expected factory to be preserved in options")
	}
}

func TestConfigRemoveRun_SaveFailurePreservesExistingConfigAndSecrets(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId: "app-test",
			AppSecret: core.SecretInput{
				Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:app-test"},
			},
			Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_1", UserName: "Tester"}},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	kc := &recordingConfigKeychain{}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc

	// Make subsequent config saves fail while keeping the existing config readable.
	if err := os.Chmod(configDir, 0500); err != nil {
		t.Fatalf("Chmod(%s) error = %v", configDir, err)
	}
	defer os.Chmod(configDir, 0700)

	err := configRemoveRun(&ConfigRemoveOptions{Factory: f})
	if err == nil {
		t.Fatal("expected save failure")
	}
	if !strings.Contains(err.Error(), "failed to save config") {
		t.Fatalf("error = %v, want failed to save config", err)
	}
	if len(kc.removed) != 0 {
		t.Fatalf("expected no keychain cleanup before successful save, got removals: %v", kc.removed)
	}

	// Restore permissions and confirm the original config is still intact.
	if err := os.Chmod(configDir, 0700); err != nil {
		t.Fatalf("restore Chmod(%s) error = %v", configDir, err)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved == nil || len(saved.Apps) != 1 || saved.Apps[0].AppId != "app-test" {
		t.Fatalf("saved config = %#v, want original single app preserved", saved)
	}
	if got := saved.Apps[0].AppSecret.Ref; got == nil || got.ID != "appsecret:app-test" {
		t.Fatalf("saved app secret ref = %#v, want preserved keychain ref", got)
	}

	configPath := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected existing config file to remain, stat error = %v", err)
	}
}

func TestSaveAsProfile_RejectsProfileNameCollisionWithExistingAppID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	existing := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{
				Name:      "prod",
				AppId:     "cli_prod",
				AppSecret: core.PlainSecret("secret"),
				Brand:     core.BrandFeishu,
			},
		},
	}

	err := saveAsProfile(existing, keychain.KeychainAccess(&noopConfigKeychain{}), "cli_prod", "app-new", core.PlainSecret("new-secret"), core.BrandLark, "en")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	// A name/appId conflict is user input — a typed validation error naming the
	// offending flag, not a system storage failure.
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error type = %T, want *errs.ValidationError; err=%v", err, err)
	}
	if verr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want invalid_argument", verr.Subtype)
	}
	if verr.Param != "--name" {
		t.Errorf("param = %q, want --name", verr.Param)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (validation)", output.ExitCodeOf(err), output.ExitValidation)
	}
	if !strings.Contains(verr.Message, "conflicts with existing appId") {
		t.Errorf("message = %q, want conflict description", verr.Message)
	}
}

// TestWrapSaveConfigError_PassesTypedValidationThrough pins that a user-input
// validation error (e.g. the --name conflict) is not reclassified as an
// internal storage failure on its way up through the save call sites.
func TestWrapSaveConfigError_PassesTypedValidationThrough(t *testing.T) {
	conflict := errs.NewValidationError(errs.SubtypeInvalidArgument, "name conflict").WithParam("--name")
	var verr *errs.ValidationError
	if !errors.As(wrapSaveConfigError(conflict), &verr) {
		t.Fatalf("typed validation must pass through unchanged, got %T", wrapSaveConfigError(conflict))
	}
	var ierr *errs.InternalError
	if !errors.As(wrapSaveConfigError(errors.New("disk full")), &ierr) || ierr.Subtype != errs.SubtypeStorage {
		t.Fatalf("untyped failure must become internal/storage")
	}
}

func TestUpdateExistingProfileWithoutSecret_RejectsAppIDChange(t *testing.T) {
	multi := &core.MultiAppConfig{
		CurrentApp: "prod",
		Apps: []core.AppConfig{
			{
				Name:      "prod",
				AppId:     "app-old",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{Source: "keychain", ID: "appsecret:app-old"}},
				Brand:     core.BrandFeishu,
				Lang:      "zh",
				Users:     []core.AppUser{{UserOpenId: "ou_1", UserName: "User"}},
			},
		},
	}

	err := updateExistingProfileWithoutSecret(multi, "", "app-new", core.BrandLark, "en")
	if err == nil {
		t.Fatal("expected error when changing app ID without a new secret")
	}
	if !strings.Contains(err.Error(), "App Secret") {
		t.Fatalf("error = %v, want mention of App Secret", err)
	}
}

// stubConfigExtProvider simulates env/sidecar credential mode for config guard tests.
type stubConfigExtProvider struct{ name string }

func (s *stubConfigExtProvider) Name() string { return s.name }
func (s *stubConfigExtProvider) ResolveAccount(_ context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "test-app"}, nil
}
func (s *stubConfigExtProvider) ResolveToken(_ context.Context, _ extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func newConfigFactoryWithExternalProvider(t *testing.T) *cmdutil.Factory {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	stub := &stubConfigExtProvider{name: "env"}
	cred := credential.NewCredentialProvider([]extcred.Provider{stub}, nil, nil, nil)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Credential = cred
	return f
}

func TestConfigBlockedByExternalProvider(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)

	tests := []struct {
		name string
		args []string
	}{
		{"init", []string{"init", "--app-id", "x", "--app-secret-stdin"}},
		{"remove", []string{"remove"}},
		{"show", []string{"show"}},
		{"default-as", []string{"default-as", "user"}},
		{"strict-mode", []string{"strict-mode", "off"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCmdConfig(f)
			cmd.SilenceErrors = true
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)

			// Locate the subcommand before execution (PersistentPreRunE receives it as cmd).
			matched, _, _ := cmd.Find(tt.args)

			err := cmd.Execute()

			// PersistentPreRunE sets SilenceUsage on the matched subcommand, not the parent.
			if matched != nil && matched != cmd && !matched.SilenceUsage {
				t.Error("expected PersistentPreRunE to set SilenceUsage on matched subcommand")
			}
			if gotCode := output.ExitCodeOf(err); gotCode != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", gotCode, output.ExitValidation)
			}
		})
	}
}

// TestValidateInitLang covers the --lang contract: empty (omitted or explicit)
// is a no-op leaving Lang unset; a short code or Feishu locale canonicalizes to
// the same locale; an unrecognized value errors.
func TestValidateInitLang(t *testing.T) {
	t.Run("empty is a no-op", func(t *testing.T) {
		for _, explicit := range []bool{false, true} {
			opts := &ConfigInitOptions{Lang: "", langExplicit: explicit}
			if err := validateInitLang(opts); err != nil {
				t.Fatalf("explicit=%v: expected nil error, got %v", explicit, err)
			}
			if opts.Lang != "" {
				t.Errorf("explicit=%v: Lang = %q, want \"\" (unset)", explicit, opts.Lang)
			}
		}
	})
	t.Run("short and locale canonicalize alike", func(t *testing.T) {
		for _, in := range []string{"ja", "ja_jp"} {
			opts := &ConfigInitOptions{Lang: in, langExplicit: true}
			if err := validateInitLang(opts); err != nil {
				t.Fatalf("--lang %q: unexpected error %v", in, err)
			}
			if opts.Lang != string(i18n.LangJaJP) {
				t.Errorf("--lang %q normalized to %q, want %q", in, opts.Lang, i18n.LangJaJP)
			}
		}
	})
}

// TestPrintLangPreferenceConfirmation covers the confirmation helper: it prints
// to stderr only when --lang explicitly set a non-empty preference.
func TestPrintLangPreferenceConfirmation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	// UILang matches Lang in every case below: resolveInitUILang derives the
	// display language from the same --lang value, so a combination like
	// Lang=en_us with a Chinese UI cannot occur in the real flow and would hide
	// a regression in that resolution chain.
	t.Run("explicit non-empty prints confirmation", func(t *testing.T) {
		f, _, stderr, _ := cmdutil.TestFactory(t, nil)
		printLangPreferenceConfirmation(&ConfigInitOptions{Factory: f, Lang: "en_us", UILang: i18n.LangEnUS, langExplicit: true})
		got := stderr.String()
		if !strings.Contains(got, "Language preference set to") || !strings.Contains(got, "en_us") {
			t.Errorf("stderr = %q, want confirmation mentioning the preference and en_us", got)
		}
	})
	t.Run("implicit prints nothing", func(t *testing.T) {
		f, _, stderr, _ := cmdutil.TestFactory(t, nil)
		printLangPreferenceConfirmation(&ConfigInitOptions{Factory: f, Lang: "en_us", UILang: i18n.LangEnUS, langExplicit: false})
		if got := stderr.String(); got != "" {
			t.Errorf("stderr = %q, want empty when --lang is implicit", got)
		}
	})
	t.Run("explicit empty prints nothing", func(t *testing.T) {
		f, _, stderr, _ := cmdutil.TestFactory(t, nil)
		printLangPreferenceConfirmation(&ConfigInitOptions{Factory: f, Lang: "", UILang: "", langExplicit: true})
		if got := stderr.String(); got != "" {
			t.Errorf("stderr = %q, want empty when --lang is empty", got)
		}
	})
}

// The "no active profile" producer annotates its profile/list recovery target.
// Rendering against one build's surface filters a clone without mutating the
// value another command tree may render.
func TestConfigShowRun_ProfileHintUsesBuildLocalSurface(t *testing.T) {
	multi := &core.MultiAppConfig{
		CurrentApp: "missing",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "app-default",
			AppSecret: core.PlainSecret("secret-default"),
			Brand:     core.BrandFeishu,
		}},
	}

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	source := configShowRun(&ConfigShowOptions{Factory: f})
	var original *errs.ConfigError
	if !errors.As(source, &original) {
		t.Fatalf("expected *errs.ConfigError, got %T %v", source, source)
	}
	if original.Subtype != errs.SubtypeNotConfigured {
		t.Fatalf("subtype = %q, want not_configured", original.Subtype)
	}
	if !strings.Contains(original.Hint, "lark-cli profile list") {
		t.Fatalf("producer hint = %q, want profile list", original.Hint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandProfileList: surface.CommandConcealed,
	})
	var concealed *errs.ConfigError
	if rendered := recovery.Render(source, plan); !errors.As(rendered, &concealed) {
		t.Fatalf("rendered error = %T, want *errs.ConfigError", rendered)
	}
	if concealed == original {
		t.Fatal("Render must clone the typed error")
	}
	if strings.Contains(concealed.Hint, "profile list") ||
		!strings.Contains(concealed.Hint, "select an available profile") {
		t.Errorf("concealed hint = %q, want target-free profile recovery", concealed.Hint)
	}

	var visible *errs.ConfigError
	if !errors.As(recovery.Render(source, nil), &visible) ||
		!strings.Contains(visible.Hint, "lark-cli profile list") {
		t.Errorf("visible render must keep profile list, got %+v", visible)
	}
	if !strings.Contains(original.Hint, "lark-cli profile list") {
		t.Errorf("concealed render mutated source hint: %q", original.Hint)
	}
}

func TestPriorInitLang(t *testing.T) {
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{AppId: "cli_main", Lang: i18n.LangEnUS},
			{Name: "work", AppId: "cli_work", Lang: i18n.LangJaJP},
		},
	}
	tests := []struct {
		name        string
		existing    *core.MultiAppConfig
		profileName string
		want        i18n.Lang
	}{
		{"nil config", nil, "", ""},
		{"current app", multi, "", i18n.LangEnUS},
		{"named profile", multi, "work", i18n.LangJaJP},
		{"named profile missing", multi, "absent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := priorInitLang(tt.existing, tt.profileName); got != tt.want {
				t.Errorf("priorInitLang() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveInitUILang(t *testing.T) {
	stored := &core.MultiAppConfig{
		Apps: []core.AppConfig{{AppId: "cli_x", Lang: i18n.LangEnUS}},
	}
	tests := []struct {
		name     string
		lang     string
		existing *core.MultiAppConfig
		want     i18n.Lang
	}{
		{"flag wins over stored", "zh_cn", stored, i18n.LangZhCN},
		{"flag with no stored", "en_us", nil, i18n.LangEnUS},
		{"stored used when flag absent", "", stored, i18n.LangEnUS},
		{"nothing set", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ConfigInitOptions{Lang: tt.lang}
			resolveInitUILang(opts, tt.existing)
			if opts.UILang != tt.want {
				t.Errorf("UILang = %q, want %q", opts.UILang, tt.want)
			}
		})
	}
}

// TestInitLangChain_ResolveToBundle walks the three steps config init runs in
// order — resolve, picker gate, bundle lookup — instead of checking each in
// isolation. The regression this branch fixes lived in the seam: every step was
// individually defensible while the chain still ended on the Chinese bundle.
// isTerminal is true throughout, so a picker that should not run would show up
// as a gate returning true.
func TestInitLangChain_ResolveToBundle(t *testing.T) {
	stored := &core.MultiAppConfig{
		Apps: []core.AppConfig{{AppId: "cli_x", Lang: i18n.LangEnUS}},
	}
	tests := []struct {
		name       string
		opts       ConfigInitOptions
		existing   *core.MultiAppConfig
		wantPrompt bool
	}{
		// --lang answers the question outright, so nothing is asked.
		{"--lang en_us, nothing stored", ConfigInitOptions{Lang: "en_us", langExplicit: true}, nil, false},
		// A stored preference decides what renders, but the picker still runs
		// with it pre-selected — that is the only interactive way to change it.
		{"no --lang, en_us already stored", ConfigInitOptions{}, stored, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			resolveInitUILang(&opts, tt.existing)
			if opts.UILang != i18n.LangEnUS {
				t.Fatalf("UILang = %q, want %q", opts.UILang, i18n.LangEnUS)
			}
			if got := shouldPromptInitLang(&opts, true); got != tt.wantPrompt {
				t.Errorf("shouldPromptInitLang() = %v, want %v", got, tt.wantPrompt)
			}
			// Whatever the picker does next, the language in effect right now
			// already selects the English bundle — nothing renders in Chinese
			// while an English preference is live.
			if msg := getInitMsg(opts.UILang); msg != initMsgEn {
				t.Errorf("getInitMsg(%q) returned the non-English bundle", opts.UILang)
			}
		})
	}
}

func TestShouldPromptInitLang(t *testing.T) {
	tests := []struct {
		name       string
		opts       ConfigInitOptions
		isTerminal bool
		want       bool
	}{
		{"terminal, nothing resolved", ConfigInitOptions{}, true, true},
		{"not a terminal", ConfigInitOptions{}, false, false},
		// A stored zh_cn/en_us still asks — the picker pre-selects it, so
		// re-running init remains the way to change the language.
		{"stored en_us still asks", ConfigInitOptions{UILang: i18n.LangEnUS}, true, true},
		{"stored zh_cn still asks", ConfigInitOptions{UILang: i18n.LangZhCN}, true, true},
		// The picker has no option for these, so a bare Enter would silently
		// rewrite the preference. Skip it and keep what --lang set.
		{"stored ja_jp skips the picker", ConfigInitOptions{UILang: i18n.LangJaJP}, true, false},
		// An unrecognized value renders Chinese because it expresses no usable
		// preference; the picker must stay reachable so it can be changed.
		{"unrecognized stored value still asks", ConfigInitOptions{UILang: "klingon"}, true, true},
		{"--lang was explicit", ConfigInitOptions{langExplicit: true}, true, false},
		{"--new pins the flow", ConfigInitOptions{New: true}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPromptInitLang(&tt.opts, tt.isTerminal); got != tt.want {
				t.Errorf("shouldPromptInitLang() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestConfigInitRun_LangFlagDrivesRenderedOutput drives configInitRun itself
// rather than replaying the order it calls its own helpers in. It is the only
// test that fails if the orchestrator stops resolving the display language
// before it renders, which is exactly the reported regression: --lang was
// accepted and persisted while every rendered line stayed Chinese.
//
// Hermetic despite going through the whole command: the probe's token fetch
// gets an untyped transport error from the http mock, which runProbe swallows,
// and the language confirmation is written before the probe runs.
func TestConfigInitRun_LangFlagDrivesRenderedOutput(t *testing.T) {
	// GetConfigPath is workspace-scoped, so a stray Agent env var on the host
	// would send the write somewhere LARKSUITE_CLI_CONFIG_DIR does not cover.
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("test-secret\n")

	opts := &ConfigInitOptions{
		Factory:        f,
		Ctx:            context.Background(),
		AppID:          "cli_lang_probe",
		AppSecretStdin: true,
		Brand:          "feishu",
		Lang:           "en_us",
		langExplicit:   true,
	}
	if err := configInitRun(opts); err != nil {
		t.Fatalf("configInitRun: %v", err)
	}

	got := stderr.String()
	if want := fmt.Sprintf(initMsgEn.LangPreferenceSet, "en_us"); !strings.Contains(got, want) {
		t.Errorf("stderr = %q, want it to contain %q", got, want)
	}
	if unwanted := fmt.Sprintf(initMsgZh.LangPreferenceSet, "en_us"); strings.Contains(got, unwanted) {
		t.Errorf("stderr rendered the Chinese bundle despite --lang en_us: %q", got)
	}

	// The preference reaches disk as well as the screen; the regression left
	// these two disagreeing.
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if app := saved.CurrentAppConfig(""); app == nil || app.Lang != i18n.LangEnUS {
		t.Errorf("persisted Lang = %v, want %q", app, i18n.LangEnUS)
	}
}
