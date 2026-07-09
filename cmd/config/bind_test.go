// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/output"
)

// wantErrDetail is the normalized comparison shape for a typed error's wire
// fields: Type is the error's Category string ("validation", "config", ...),
// alongside Message and Hint.
type wantErrDetail struct {
	Type    string
	Message string
	Hint    string
}

// assertExitError checks the full structured error in one assertion against a
// typed error (ValidationError or ConfigError), normalizing its Category /
// Message / Hint to wantDetail.
func assertExitError(t *testing.T, err error, wantCode int, wantDetail wantErrDetail) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) {
		if got := output.ExitCodeOf(err); got != wantCode {
			t.Errorf("exit code = %d, want %d", got, wantCode)
		}
		gotDetail := wantErrDetail{Type: string(ve.Category), Message: ve.Message, Hint: ve.Hint}
		if !reflect.DeepEqual(gotDetail, wantDetail) {
			t.Errorf("validation error mismatch:\n  got:  %+v\n  want: %+v", gotDetail, wantDetail)
		}
		return
	}
	var ce *errs.ConfigError
	if errors.As(err, &ce) {
		if got := output.ExitCodeOf(err); got != wantCode {
			t.Errorf("exit code = %d, want %d", got, wantCode)
		}
		gotDetail := wantErrDetail{Type: string(ce.Category), Message: ce.Message, Hint: ce.Hint}
		if !reflect.DeepEqual(gotDetail, wantDetail) {
			t.Errorf("config error mismatch:\n  got:  %+v\n  want: %+v", gotDetail, wantDetail)
		}
		return
	}
	t.Fatalf("error type = %T, want *errs.ValidationError / *errs.ConfigError; error = %v", err, err)
}

// assertEnvelope decodes stdout and checks it matches want exactly — every key
// present, no extras, values equal via reflect.DeepEqual. Future-proofs the
// JSON wire contract: new fields added by future work force test updates.
func assertEnvelope(t *testing.T, stdout []byte, want map[string]any) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(stdout, &got); err != nil {
		t.Fatalf("invalid JSON envelope: %v\nstdout: %s", err, stdout)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("envelope mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

// saveWorkspace saves the current workspace and returns a cleanup func to restore it.
// Must be called at the start of any test that may trigger configBindRun (which sets workspace).
func saveWorkspace(t *testing.T) {
	t.Helper()
	orig := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(orig) })
}

// ── Command flag parsing tests (aligned with config_test.go pattern) ──

func TestConfigBindCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *BindOptions
	cmd := NewCmdConfigBind(f, func(opts *BindOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--source", "openclaw", "--app-id", "cli_test", "--identity", "bot-only", "--lang", "en"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Source != "openclaw" {
		t.Errorf("Source = %q, want %q", gotOpts.Source, "openclaw")
	}
	if gotOpts.AppID != "cli_test" {
		t.Errorf("AppID = %q, want %q", gotOpts.AppID, "cli_test")
	}
	if gotOpts.Identity != "bot-only" {
		t.Errorf("Identity = %q, want %q", gotOpts.Identity, "bot-only")
	}
	if gotOpts.Lang != "en" {
		t.Errorf("Lang = %q, want %q", gotOpts.Lang, "en")
	}
	if !gotOpts.langExplicit {
		t.Error("expected langExplicit=true when --lang is passed")
	}
}

func TestConfigBindCmd_LangDefault(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	var gotOpts *BindOptions
	cmd := NewCmdConfigBind(f, func(opts *BindOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"--source", "hermes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Lang != "" {
		t.Errorf("Lang = %q, want default %q (unset)", gotOpts.Lang, "")
	}
	if gotOpts.langExplicit {
		t.Error("expected langExplicit=false when --lang not passed")
	}
}

// TestConfigBindRun_InvalidLang verifies a non-empty --lang is strictly
// validated: wrong case, typos, and removed codes all exit with
// ExitValidation (code 2) and a message identifying the offending value.
// (Empty is not invalid — see TestConfigBindRun_EmptyLangIsNoOp.)
func TestConfigBindRun_InvalidLang(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

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
			err := configBindRun(&BindOptions{
				Factory:      f,
				Source:       "hermes",
				Lang:         tc.lang,
				langExplicit: true,
			})
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
			if !strings.Contains(err.Error(), "invalid --lang") {
				t.Errorf("error message %q does not contain 'invalid --lang'", err.Error())
			}
		})
	}
}

// TestConfigBindRun_EmptyLangIsNoOp verifies that an empty --lang (omitted or
// explicit "") is unset: it neither errors nor persists a language, while a
// non-empty short code or Feishu locale both canonicalize to the same locale.
func TestConfigBindRun_EmptyLangIsNoOp(t *testing.T) {
	cases := []struct {
		name     string
		lang     string
		explicit bool
		wantLang i18n.Lang
	}{
		{"omitted", "", false, ""},
		{"explicit empty", "", true, ""},
		{"short code", "ja", true, i18n.LangJaJP},
		{"feishu locale", "ja_jp", true, i18n.LangJaJP},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saveWorkspace(t)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			hermesHome := t.TempDir()
			t.Setenv("HERMES_HOME", hermesHome)
			if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			f, _, _, _ := cmdutil.TestFactory(t, nil)
			if err := configBindRun(&BindOptions{
				Factory:      f,
				Source:       "hermes",
				Lang:         tc.lang,
				langExplicit: tc.explicit,
			}); err != nil {
				t.Fatalf("configBindRun(--lang %q) = %v, want nil", tc.lang, err)
			}

			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatalf("LoadMultiAppConfig: %v", err)
			}
			app := multi.CurrentAppConfig("")
			if app == nil {
				t.Fatal("no app persisted")
			}
			if app.Lang != tc.wantLang {
				t.Errorf("persisted Lang = %q, want %q", app.Lang, tc.wantLang)
			}
		})
	}
}

// TestConfigBindRun_OmitLangPreservesPrior guards against a re-bind without
// --lang silently dropping a previously stored preference (appConfig is rebuilt
// fresh, so commitBinding must inherit the prior Lang).
func TestConfigBindRun_OmitLangPreservesPrior(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f1, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f1, Source: "hermes", Lang: "ja", langExplicit: true}); err != nil {
		t.Fatalf("first bind (--lang ja): %v", err)
	}
	f2, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f2, Source: "hermes", Lang: "", langExplicit: false}); err != nil {
		t.Fatalf("re-bind (no --lang): %v", err)
	}

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if app := multi.CurrentAppConfig(""); app == nil || app.Lang != i18n.LangJaJP {
		t.Errorf("Lang after re-bind = %v, want %q (preserved)", app, i18n.LangJaJP)
	}
}

// TestPriorLang_RespectsCurrentApp guards against priorLang scanning all apps
// and silently returning a non-current profile's Lang. In a multi-profile
// workspace (set up via `profile add` before a re-bind), the active profile's
// Lang must win over a sibling profile that happens to sit earlier in the slice.
func TestPriorLang_RespectsCurrentApp(t *testing.T) {
	multi := core.MultiAppConfig{
		CurrentApp: "active",
		Apps: []core.AppConfig{
			{Name: "stale", AppId: "cli_stale", Lang: i18n.LangJaJP},
			{Name: "active", AppId: "cli_active", Lang: i18n.LangEnUS},
		},
	}
	bytes, err := json.Marshal(multi)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := priorLang(bytes); got != i18n.LangEnUS {
		t.Errorf("priorLang = %q, want %q (must follow CurrentApp, not Apps[0])", got, i18n.LangEnUS)
	}
}

// TestPriorLang_FallsBackToFirstAppWhenCurrentUnset covers the legacy
// single-app shape (no CurrentApp): CurrentAppConfig falls back to Apps[0],
// so a bind-written config (which always has exactly one app and no
// CurrentApp field) still inherits its Lang.
func TestPriorLang_FallsBackToFirstAppWhenCurrentUnset(t *testing.T) {
	multi := core.MultiAppConfig{
		Apps: []core.AppConfig{
			{AppId: "cli_only", Lang: i18n.LangJaJP},
		},
	}
	bytes, err := json.Marshal(multi)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := priorLang(bytes); got != i18n.LangJaJP {
		t.Errorf("priorLang = %q, want %q", got, i18n.LangJaJP)
	}
}

// TestPriorLang_MalformedReturnsEmpty exercises the unparseable-bytes branch.
func TestPriorLang_MalformedReturnsEmpty(t *testing.T) {
	if got := priorLang([]byte("not json")); got != "" {
		t.Errorf("priorLang(malformed) = %q, want \"\"", got)
	}
}

// TestConfigBindRun_EnvelopeMessageFollowsInheritedLang guards the JSON envelope
// "message" field against regressing to opts.Lang: when --lang is omitted on
// re-bind, the inherited preference (appConfig.Lang) must drive the message
// language and the embedded brand display — otherwise an AI agent that set
// English on first bind sees Chinese in every subsequent re-bind envelope.
func TestConfigBindRun_EnvelopeMessageFollowsInheritedLang(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f1, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f1, Source: "hermes", Lang: "en", langExplicit: true}); err != nil {
		t.Fatalf("first bind (--lang en): %v", err)
	}

	f2, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f2, Source: "hermes", Lang: "", langExplicit: false}); err != nil {
		t.Fatalf("re-bind (no --lang): %v", err)
	}

	envelope := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	msg, _ := envelope["message"].(string)
	enMsg := getBindMsg(i18n.LangEnUS)
	wantMsg := fmt.Sprintf(enMsg.MessageBotOnly, "cli_abc", "Hermes", brandDisplay("feishu", i18n.LangEnUS))
	if msg != wantMsg {
		t.Errorf("envelope.message = %q,\nwant %q (must follow inherited appConfig.Lang=en_us, not raw opts.Lang)", msg, wantMsg)
	}
}

// ── Run function tests (aligned with TestConfigShowRun pattern) ──

func TestConfigBindRun_InvalidSource(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "invalid"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `invalid --source "invalid"; valid values: openclaw, hermes, lark-channel`,
	})
}

func TestConfigBindRun_MissingSourceNonTTY(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	// Ensure no Agent env signals leak in from the host shell and silently
	// trigger auto-detection; this test exercises the "no signals at all"
	// path, where flag mode must error out with an actionable hint.
	clearAgentEnv(t)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// TestFactory has IsTerminal=false by default
	err := configBindRun(&BindOptions{Factory: f, Source: ""})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: "cannot determine Agent source: no --source flag and no Agent environment detected",
		Hint:    "pass --source openclaw|hermes|lark-channel, or run this command inside the corresponding Agent context",
	})
}

// clearAgentEnv removes every env var that DetectWorkspaceFromEnv treats as
// an Agent signal, so tests exercising the "no signals" path stay isolated
// from whatever the host shell exported. Prefix-based instead of an explicit
// list — when DetectWorkspaceFromEnv gains a new OPENCLAW_* / HERMES_* signal,
// this helper does not need to be updated and tests do not silently misroute.
// t.Setenv restores the original values after the test returns.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		k := kv[:idx]
		if strings.HasPrefix(k, "OPENCLAW_") ||
			strings.HasPrefix(k, "HERMES_") ||
			k == "LARK_CHANNEL" {
			t.Setenv(k, "")
		}
	}
}

// --source openclaw specified while the env clearly identifies Hermes is
// almost always a user mistake (wrong Agent context); we fail loud.
func TestConfigBindRun_SourceEnvMismatch_OpenClawFlagInHermesEnv(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)
	t.Setenv("HERMES_HOME", t.TempDir()) // Hermes env signal

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `--source "openclaw" does not match detected Agent environment (hermes)`,
		Hint:    "remove --source to auto-detect, or run this command in the correct Agent context",
	})
}

// Reverse direction: --source hermes while OpenClaw env is active.
func TestConfigBindRun_SourceEnvMismatch_HermesFlagInOpenClawEnv(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)
	t.Setenv("OPENCLAW_HOME", t.TempDir()) // OpenClaw env signal

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `--source "hermes" does not match detected Agent environment (openclaw)`,
		Hint:    "remove --source to auto-detect, or run this command in the correct Agent context",
	})
}

// With --source omitted and Hermes env present, auto-detect picks hermes.
// We only assert the source routing worked (config.json was written to the
// hermes workspace path); the bind command's own happy path is covered by
// other tests.
func TestConfigBindRun_AutoDetect_HermesFromEnv(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	clearAgentEnv(t)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_auto\nFEISHU_APP_SECRET=auto_secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	// Note: Source is empty — auto-detection should pick hermes.
	err := configBindRun(&BindOptions{Factory: f})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	envelope := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if envelope["workspace"] != "hermes" {
		t.Errorf("workspace = %v, want %q (auto-detection should pick hermes from HERMES_HOME)", envelope["workspace"], "hermes")
	}
}

// With --source omitted and OpenClaw env present, auto-detect picks openclaw.
func TestConfigBindRun_AutoDetect_OpenClawFromEnv(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	clearAgentEnv(t)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_auto_oc","appSecret":"auto_oc_secret","domain":"feishu"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	// Note: Source is empty — auto-detection should pick openclaw.
	err := configBindRun(&BindOptions{Factory: f})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	envelope := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if envelope["workspace"] != "openclaw" {
		t.Errorf("workspace = %v, want %q (auto-detection should pick openclaw from OPENCLAW_HOME)", envelope["workspace"], "openclaw")
	}
}

func TestConfigBindRun_FlagModeOverwrite(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	// Pre-create hermes workspace config to simulate an existing binding.
	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"), []byte(`{"apps":[{"appId":"old_app"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_new_app\nFEISHU_APP_SECRET=new_secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	if err != nil {
		t.Fatalf("expected flag-mode overwrite to succeed, got error: %v", err)
	}

	msg := getBindMsg("zh") // flag mode leaves Lang empty → zh default
	assertEnvelope(t, stdout.Bytes(), map[string]any{
		"ok":          true,
		"workspace":   "hermes",
		"app_id":      "cli_new_app",
		"config_path": filepath.Join(configDir, "hermes", "config.json"),
		"replaced":    true,
		"identity":    "bot-only",
		"message":     fmt.Sprintf(msg.MessageBotOnly, "cli_new_app", "Hermes", brandDisplay("feishu", "")),
	})
	// stderr carries only the bind-success header + one-time-sync notice;
	// the "replaced existing binding" suffix is intentionally dropped now
	// that `replaced:true` in the stdout envelope carries the same signal.
	if want := fmt.Sprintf(msg.BindSuccessHeader, "Hermes"); !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr missing bind-success header %q; got:\n%s", want, stderr.String())
	}
}

func TestConfigBindRun_HermesMissingEnvFile(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	hermesHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("HERMES_HOME", hermesHome)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "failed to read Hermes config: open " + envPath + ": no such file or directory",
		Hint:    "verify Hermes is installed and configured at " + envPath,
	})
}

func TestConfigBindRun_OpenClawMissingFile(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	openclawHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	configPath := filepath.Join(openclawHome, ".openclaw", "openclaw.json")
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "cannot read " + configPath + ": open " + configPath + ": no such file or directory",
		Hint:    "verify OpenClaw is installed and configured",
	})
}

// writeLarkChannelFixture writes a ~/.lark-channel/config.json under fakeHome
// and returns the config path. resolveLarkChannelConfigPath reads HOME via
// os.UserHomeDir, so callers must `t.Setenv("HOME", fakeHome)`.
func writeLarkChannelFixture(t *testing.T, fakeHome, body string) string {
	t.Helper()
	dir := filepath.Join(fakeHome, ".lark-channel")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// Happy-path: --source lark-channel reads ~/.lark-channel/config.json,
// writes the workspace config, emits a JSON envelope with workspace:
// "lark-channel" and brand from accounts.app.tenant.
func TestConfigBindRun_LarkChannel_Success(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	clearAgentEnv(t)

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeLarkChannelFixture(t, fakeHome, `{"accounts":{"app":{"id":"cli_lc_main","secret":"lc_secret","tenant":"feishu"}}}`)

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	envelope := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if envelope["workspace"] != "lark-channel" {
		t.Errorf("workspace = %v, want %q", envelope["workspace"], "lark-channel")
	}
	if envelope["app_id"] != "cli_lc_main" {
		t.Errorf("app_id = %v, want %q", envelope["app_id"], "cli_lc_main")
	}

	// Brand is not in the stdout envelope — read it back from the persisted
	// workspace config to verify accounts.app.tenant flowed through to the
	// stored AppConfig.Brand field.
	core.SetCurrentWorkspace(core.WorkspaceLarkChannel)
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if len(multi.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(multi.Apps))
	}
	if got := string(multi.Apps[0].Brand); got != "feishu" {
		t.Errorf("Brand = %q, want %q", got, "feishu")
	}
}

// Env template form: secret = "${VAR}" should resolve via the SecretInput
// pipeline (same path openclaw uses), so the keychain receives the env value
// not the literal template string.
func TestConfigBindRun_LarkChannel_EnvTemplate(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("LARK_APP_SECRET", "resolved_via_env")
	writeLarkChannelFixture(t, fakeHome,
		`{"accounts":{"app":{"id":"cli_lc_env","secret":"${LARK_APP_SECRET}","tenant":"feishu"}}}`)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

// tenant: "lark" should land as Brand("lark"), not normalized to "feishu".
func TestConfigBindRun_LarkChannel_LarkTenant(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeLarkChannelFixture(t, fakeHome, `{"accounts":{"app":{"id":"cli_lc_lark","secret":"s","tenant":"lark"}}}`)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	core.SetCurrentWorkspace(core.WorkspaceLarkChannel)
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("load workspace config: %v", err)
	}
	if got := string(multi.Apps[0].Brand); got != "lark" {
		t.Errorf("Brand = %q, want %q (tenant: lark must flow through to AppConfig.Brand)", got, "lark")
	}
}

// LARK_CHANNEL=1 alone (no --source) auto-detects to the lark-channel
// workspace, mirroring the OpenClaw/Hermes auto-detect flow.
func TestConfigBindRun_AutoDetect_LarkChannelFromEnv(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)
	t.Setenv("LARK_CHANNEL", "1")

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeLarkChannelFixture(t, fakeHome, `{"accounts":{"app":{"id":"cli_auto_lc","secret":"s","tenant":"feishu"}}}`)

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f}); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	envelope := map[string]any{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if envelope["workspace"] != "lark-channel" {
		t.Errorf("workspace = %v, want %q (auto-detection should pick lark-channel from LARK_CHANNEL=1)", envelope["workspace"], "lark-channel")
	}
}

// --source lark-channel while the env signals OpenClaw must fail loud, same
// rule as OpenClaw/Hermes mismatch (running in the wrong Agent context).
func TestConfigBindRun_SourceEnvMismatch_LarkChannelFlagInOpenClawEnv(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)
	t.Setenv("OPENCLAW_HOME", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `--source "lark-channel" does not match detected Agent environment (openclaw)`,
		Hint:    "remove --source to auto-detect, or run this command in the correct Agent context",
	})
}

// Missing config.json → typed error with a hint pointing at bridge setup.
func TestConfigBindRun_LarkChannelMissingFile(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)

	fakeHome := t.TempDir() // empty — no .lark-channel/config.json
	t.Setenv("HOME", fakeHome)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"})
	configPath := filepath.Join(fakeHome, ".lark-channel", "config.json")
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "cannot read " + configPath + ": open " + configPath + ": no such file or directory",
		Hint:    "verify lark-channel-bridge is installed and configured",
	})
}

// Empty accounts.app.id → typed error pointing at bridge setup. Distinct
// from "missing file" so users know whether to install or to re-run setup.
func TestConfigBindRun_LarkChannelEmptyAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	configPath := writeLarkChannelFixture(t, fakeHome, `{"accounts":{"app":{"id":"","secret":"","tenant":"feishu"}}}`)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"})
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "accounts.app.id missing in " + configPath,
		Hint:    "run lark-channel-bridge's setup to populate the app credential",
	})
}

// app.id present but app.secret missing → typed error at the Build step.
func TestConfigBindRun_LarkChannelEmptySecret(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	clearAgentEnv(t)

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	configPath := writeLarkChannelFixture(t, fakeHome, `{"accounts":{"app":{"id":"cli_no_secret","secret":"","tenant":"feishu"}}}`)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "lark-channel"})
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "accounts.app.secret is empty in " + configPath,
		Hint:    "run lark-channel-bridge's setup to populate the app credential",
	})
}

func TestConfigShowRun_WorkspaceField(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "cli_local_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("save: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err != nil {
		t.Fatalf("configShowRun error: %v", err)
	}
	// If we get here without error, show succeeded.
	// Workspace field in JSON output is verified by e2e tests (real binary output).
}

func TestConfigShowRun_AgentWorkspaceNotBound(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configShowRun(&ConfigShowOptions{Factory: f})
	if err == nil {
		t.Fatal("expected error for unbound workspace")
	}
	// Should be a structured ConfigError suggesting config bind, not config init.
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *errs.ConfigError", err)
	}
	// Config errors share ExitAuth (3); the workspace is detected but no
	// binding exists yet, which is a config error.
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Errorf("exit code = %d, want %d (config category → ExitAuth)", got, output.ExitAuth)
	}
	// The workspace name stays out of the wire subtype; it only appears in
	// the message.
	if cfgErr.Subtype != errs.SubtypeNotConfigured {
		t.Errorf("subtype = %q, want not_configured", cfgErr.Subtype)
	}
	if !strings.Contains(cfgErr.Message, "openclaw context detected") {
		t.Errorf("message missing 'openclaw context detected': %q", cfgErr.Message)
	}
	// Hint must point at config bind --help (NOT a ready-to-run bind command):
	// AI must read the help and confirm identity preset with the user first.
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("hint must point at `config bind --help`; got %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "config init") {
		t.Errorf("agent hint must not mention config init; got %q", cfgErr.Hint)
	}
}

// ── Helper function tests (dotenv, brand, path resolution) ──

func TestReadDotenv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	content := "# Hermes config\nFEISHU_APP_ID=cli_abc123\nFEISHU_APP_SECRET=supersecret\nFEISHU_DOMAIN=lark\n\nFEISHU_CONNECTION_MODE=websocket\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	got, err := readDotenv(envPath)
	if err != nil {
		t.Fatalf("readDotenv() error: %v", err)
	}

	checks := map[string]string{
		"FEISHU_APP_ID":          "cli_abc123",
		"FEISHU_APP_SECRET":      "supersecret",
		"FEISHU_DOMAIN":          "lark",
		"FEISHU_CONNECTION_MODE": "websocket",
	}
	for key, want := range checks {
		if got[key] != want {
			t.Errorf("key %q = %q, want %q", key, got[key], want)
		}
	}
}

func TestReadDotenv_FileNotFound(t *testing.T) {
	_, err := readDotenv("/nonexistent/path/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadDotenv_ValueWithEquals(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `DATABASE_URL=postgres://user:pass@host:5432/db?sslmode=require`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	got, err := readDotenv(envPath)
	if err != nil {
		t.Fatalf("readDotenv() error: %v", err)
	}
	want := "postgres://user:pass@host:5432/db?sslmode=require"
	if got["DATABASE_URL"] != want {
		t.Errorf("DATABASE_URL = %q, want %q", got["DATABASE_URL"], want)
	}
}

func TestNormalizeBrand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "feishu"},
		{"feishu", "feishu"},
		{"lark", "lark"},
		{"LARK", "lark"},
		{" lark ", "lark"},
		{"Lark", "lark"},
	}
	for _, tt := range tests {
		if got := normalizeBrand(tt.input); got != tt.want {
			t.Errorf("normalizeBrand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveOpenClawConfigPath_Overrides(t *testing.T) {
	t.Run("OPENCLAW_CONFIG_PATH wins", func(t *testing.T) {
		custom := filepath.Join(t.TempDir(), "custom.json")
		t.Setenv("OPENCLAW_CONFIG_PATH", custom)
		t.Setenv("OPENCLAW_STATE_DIR", "")
		t.Setenv("OPENCLAW_HOME", "")
		if got := resolveOpenClawConfigPath(); got != custom {
			t.Errorf("got %q, want %q", got, custom)
		}
	})

	t.Run("OPENCLAW_STATE_DIR", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("OPENCLAW_CONFIG_PATH", "")
		t.Setenv("OPENCLAW_STATE_DIR", dir)
		t.Setenv("OPENCLAW_HOME", "")
		want := filepath.Join(dir, "openclaw.json")
		if got := resolveOpenClawConfigPath(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("OPENCLAW_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("OPENCLAW_CONFIG_PATH", "")
		t.Setenv("OPENCLAW_STATE_DIR", "")
		t.Setenv("OPENCLAW_HOME", dir)
		want := filepath.Join(dir, ".openclaw", "openclaw.json")
		if got := resolveOpenClawConfigPath(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestResolveHermesEnvPath_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HERMES_HOME", tmp)
	want := filepath.Join(tmp, ".env")
	if got := resolveHermesEnvPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── Success path tests (Hermes bind flow) ──

func TestConfigBindRun_HermesSuccess(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	envContent := "FEISHU_APP_ID=cli_hermes_abc\nFEISHU_APP_SECRET=hermes_secret_123\nFEISHU_DOMAIN=lark\n"
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", Lang: "en"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	if result["workspace"] != "hermes" {
		t.Errorf("workspace = %v, want %q", result["workspace"], "hermes")
	}
	if result["app_id"] != "cli_hermes_abc" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_hermes_abc")
	}

	targetPath := filepath.Join(configDir, "hermes", "config.json")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		t.Fatalf("unmarshal config.json: %v", err)
	}
	if len(multi.Apps) != 1 {
		t.Fatalf("apps count = %d, want 1", len(multi.Apps))
	}
	if multi.Apps[0].AppId != "cli_hermes_abc" {
		t.Errorf("appId = %q, want %q", multi.Apps[0].AppId, "cli_hermes_abc")
	}
	if multi.Apps[0].Brand != core.BrandLark {
		t.Errorf("brand = %q, want %q", multi.Apps[0].Brand, core.BrandLark)
	}
}

func TestConfigBindRun_OpenClawSuccess_SingleAccount(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_oc_123","appSecret":"oc_secret_456","domain":"feishu"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", Lang: "zh"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	if result["workspace"] != "openclaw" {
		t.Errorf("workspace = %v, want %q", result["workspace"], "openclaw")
	}
	if result["app_id"] != "cli_oc_123" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_oc_123")
	}
}

func TestConfigBindRun_OpenClawMultiAccount_WithAppID(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work","domain":"feishu"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal","domain":"lark"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", AppID: "cli_personal_222"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["app_id"] != "cli_personal_222" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_personal_222")
	}
}

func TestConfigBindRun_OpenClawMultiAccount_MissingAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	if err == nil {
		t.Fatal("expected error for multi-account without --app-id, got nil")
	}
	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", gotCode, output.ExitValidation)
	}
}

// TestConfigBindRun_OpenClawMultiAccount_TTYFlagMode asserts the end-to-end
// contract: passing --source on a real terminal is flag-mode. With multiple
// candidates and no --app-id, the command must error with the candidate list
// instead of opening an interactive prompt just because stdin is a TTY.
func TestConfigBindRun_OpenClawMultiAccount_TTYFlagMode(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{
		"channels":{"feishu":{
			"accounts":{
				"work":{"appId":"cli_work_111","appSecret":"secret_work"},
				"personal":{"appId":"cli_personal_222","appSecret":"secret_personal"}
			}
		}}
	}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Simulate a real terminal. Because --source is explicit, opts.IsTUI is
	// still false, so selectCandidate must refuse the multi-candidate case
	// with a validation error rather than opening the huh prompt.
	f.IOStreams.IsTerminal = true

	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})

	// The hint's candidate list comes from openclaw.ListCandidateApps, which
	// iterates a map — ordering is non-deterministic. DeepEqual inline against
	// each accepted variant so every ErrDetail field (Type, Code, Message,
	// Hint, ConsoleURL, Detail, and any future addition) is still compared.
	base := wantErrDetail{
		Type:    "validation",
		Message: "multiple accounts in openclaw.json; pass --app-id <id>",
	}
	wantWorkFirst := base
	wantWorkFirst.Hint = "available app IDs:\n  cli_work_111 (work)\n  cli_personal_222 (personal)"
	wantPersonalFirst := base
	wantPersonalFirst.Hint = "available app IDs:\n  cli_personal_222 (personal)\n  cli_work_111 (work)"

	if gotCode := output.ExitCodeOf(err); gotCode != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", gotCode, output.ExitValidation)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *errs.ValidationError; err = %v", err, err)
	}
	got := wantErrDetail{Type: string(ve.Category), Message: ve.Message, Hint: ve.Hint}
	if !reflect.DeepEqual(got, wantWorkFirst) && !reflect.DeepEqual(got, wantPersonalFirst) {
		t.Errorf("error detail did not match any accepted variant:\n  got:  %+v\n  want: %+v OR %+v",
			got, wantWorkFirst, wantPersonalFirst)
	}
}

func TestConfigBindRun_OpenClawMultiAccount_WrongAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_only_one","appSecret":"secret_only"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw", AppID: "nonexistent"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `--app-id "nonexistent" not found in openclaw.json`,
		Hint:    "available app IDs:\n  cli_only_one",
	})
}

func TestConfigBindRun_InvalidIdentity(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes", Identity: "invalid"})
	assertExitError(t, err, output.ExitValidation, wantErrDetail{
		Type:    "validation",
		Message: `invalid --identity "invalid"; valid values: bot-only, user-default`,
	})
}

// TestConfigBindRun_Identity_BotOnly_Applied verifies the bot-only preset:
// full envelope contract on stdout, plus the disk-side StrictMode/DefaultAs
// expansion that the preset is responsible for.
func TestConfigBindRun_Identity_BotOnly_Applied(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: "bot-only",
		Lang:     "en",
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	msg := getBindMsg("en")
	assertEnvelope(t, stdout.Bytes(), map[string]any{
		"ok":          true,
		"workspace":   "hermes",
		"app_id":      "cli_abc",
		"config_path": filepath.Join(configDir, "hermes", "config.json"),
		"replaced":    false,
		"identity":    "bot-only",
		"message":     fmt.Sprintf(msg.MessageBotOnly, "cli_abc", "Hermes", brandDisplay("feishu", "en")),
	})
	assertPresetApplied(t, filepath.Join(configDir, "hermes", "config.json"),
		core.StrictModeBot, core.AsBot)
}

// TestConfigBindRun_FlagModeDefaultsToBotOnly verifies the flag-mode default
// (no --identity → bot-only) both on-wire and on-disk. Flag mode defaults to
// the safer preset — bot acts under its own identity, no impersonation risk.
// Covers the bot-only preset expansion end-to-end.
func TestConfigBindRun_FlagModeDefaultsToBotOnly(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	msg := getBindMsg("zh") // flag mode leaves Lang empty → zh default
	assertEnvelope(t, stdout.Bytes(), map[string]any{
		"ok":          true,
		"workspace":   "hermes",
		"app_id":      "cli_abc",
		"config_path": filepath.Join(configDir, "hermes", "config.json"),
		"replaced":    false,
		"identity":    "bot-only",
		"message":     fmt.Sprintf(msg.MessageBotOnly, "cli_abc", "Hermes", brandDisplay("feishu", "")),
	})
	assertPresetApplied(t, filepath.Join(configDir, "hermes", "config.json"),
		core.StrictModeBot, core.AsBot)
}

// TestConfigBindRun_WarnsOnIdentityEscalationWithoutForce verifies the
// risk-warning gate: when a workspace is already bound to bot-only and a
// flag-mode caller tries to rebind with --identity user-default, the CLI
// refuses and returns structured guidance telling the Agent to surface the
// risk to the user and re-run with --force after getting confirmation.
func TestConfigBindRun_WarnsOnIdentityEscalationWithoutForce(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := []byte(`{"apps":[{"appId":"cli_old","strictMode":"bot","defaultAs":"bot"}]}`)
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"), existing, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_new\nFEISHU_APP_SECRET=new\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: "user-default",
	})
	msg := getBindMsg("zh") // flag mode leaves Lang empty → zh default
	var ce *errs.ConfirmationRequiredError
	if !errors.As(err, &ce) {
		t.Fatalf("error type = %T, want *errs.ConfirmationRequiredError; error = %v", err, err)
	}
	if ce.Risk != errs.RiskHighRiskWrite {
		t.Errorf("Risk = %q, want %q", ce.Risk, errs.RiskHighRiskWrite)
	}
	if ce.Message != msg.IdentityEscalationMessage {
		t.Errorf("Message mismatch:\ngot:  %q\nwant: %q", ce.Message, msg.IdentityEscalationMessage)
	}
	if ce.Hint != msg.IdentityEscalationHint {
		t.Errorf("Hint mismatch:\ngot:  %q\nwant: %q", ce.Hint, msg.IdentityEscalationHint)
	}

	// Config on disk must remain untouched — the gate runs before
	// commitBinding writes anything.
	after, readErr := os.ReadFile(filepath.Join(hermesDir, "config.json"))
	if readErr != nil {
		t.Fatalf("read post-reject config: %v", readErr)
	}
	if string(after) != string(existing) {
		t.Errorf("config was modified despite rejection; got:\n%s", after)
	}
}

// TestConfigBindRun_IdentityEscalationWithForceAllowed verifies the --force
// override: the same bot-only → user-default transition that the previous
// test rejects succeeds when the caller explicitly opts in.
func TestConfigBindRun_IdentityEscalationWithForceAllowed(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"),
		[]byte(`{"apps":[{"appId":"cli_old","strictMode":"bot","defaultAs":"bot"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_new\nFEISHU_APP_SECRET=new\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: "user-default",
		Force:    true,
	})
	if err != nil {
		t.Fatalf("expected --force to allow the escalation, got: %v", err)
	}
	assertPresetApplied(t, filepath.Join(hermesDir, "config.json"),
		core.StrictModeOff, core.AsUser)
}

// TestConfigBindRun_AllowsRebindSameBotOnly verifies re-binding the same
// bot-only identity is NOT blocked — only bot→user escalation is gated.
func TestConfigBindRun_AllowsRebindSameBotOnly(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"),
		[]byte(`{"apps":[{"appId":"cli_old","strictMode":"bot","defaultAs":"bot"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_new\nFEISHU_APP_SECRET=new\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: "bot-only",
	})
	if err != nil {
		t.Fatalf("expected rebind to same bot-only identity to succeed, got: %v", err)
	}
	assertPresetApplied(t, filepath.Join(hermesDir, "config.json"),
		core.StrictModeBot, core.AsBot)
}

// TestConfigBindRun_AllowsUserDefaultOnUserDefaultConfig verifies that if the
// existing binding is already user-default, another user-default bind passes
// through (no lock to fire, only bot→user is escalation).
func TestConfigBindRun_AllowsUserDefaultOnUserDefaultConfig(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesDir := filepath.Join(configDir, "hermes")
	if err := os.MkdirAll(hermesDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hermesDir, "config.json"),
		[]byte(`{"apps":[{"appId":"cli_old","strictMode":"off","defaultAs":"user"}]}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_ID=cli_new\nFEISHU_APP_SECRET=new\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:  f,
		Source:   "hermes",
		Identity: "user-default",
	})
	if err != nil {
		t.Fatalf("expected user-default→user-default rebind to succeed, got: %v", err)
	}
	assertPresetApplied(t, filepath.Join(hermesDir, "config.json"),
		core.StrictModeOff, core.AsUser)
}

// assertPresetApplied verifies the on-disk config.json applied the identity
// preset's StrictMode + DefaultAs expansion.
func assertPresetApplied(t *testing.T, configPath string, wantStrict core.StrictMode, wantDefault core.Identity) {
	t.Helper()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		t.Fatalf("unmarshal %s: %v", configPath, err)
	}
	if len(multi.Apps) == 0 {
		t.Fatalf("no apps in %s", configPath)
	}
	app := multi.Apps[0]
	if app.StrictMode == nil || *app.StrictMode != wantStrict {
		t.Errorf("StrictMode = %v, want %q", app.StrictMode, wantStrict)
	}
	if app.DefaultAs != wantDefault {
		t.Errorf("DefaultAs = %q, want %q", app.DefaultAs, wantDefault)
	}
}

func TestConfigBindRun_HermesMissingAppID(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_SECRET=secret_only\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "FEISHU_APP_ID not found in " + envPath,
		Hint:    "run 'hermes setup' to configure Feishu credentials",
	})
}

func TestConfigBindRun_HermesMissingAppSecret(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "hermes"})
	envPath := filepath.Join(hermesHome, ".env")
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "FEISHU_APP_SECRET not found in " + envPath,
		Hint:    "run 'hermes setup' to configure Feishu credentials",
	})
}

func TestConfigBindRun_OpenClawMissingFeishu(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(`{"channels":{}}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "openclaw.json missing channels.feishu section",
		Hint:    "configure Feishu in OpenClaw first",
	})
}

func TestConfigBindRun_OpenClawEmptyAppSecret(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_no_secret","appSecret":""}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	openclawPath := filepath.Join(openclawDir, "openclaw.json")
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "appSecret is empty for app cli_no_secret in " + openclawPath,
		Hint:    "configure channels.feishu.appSecret in openclaw.json",
	})
}

func TestConfigBindRun_OpenClawEnvTemplate(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("MY_OC_SECRET", "resolved_env_secret")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"appId":"cli_env_test","appSecret":"${MY_OC_SECRET}","domain":"lark"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["app_id"] != "cli_env_test" {
		t.Errorf("app_id = %v, want %q", result["app_id"], "cli_env_test")
	}
}

func TestConfigBindRun_OpenClawDisabledAccount(t *testing.T) {
	saveWorkspace(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	openclawCfg := `{"channels":{"feishu":{"accounts":{"work":{"appId":"cli_disabled","appSecret":"secret","enabled":false}}}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(openclawCfg), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"})
	assertExitError(t, err, output.ExitAuth, wantErrDetail{
		Type:    "config",
		Message: "no Feishu app configured in openclaw.json",
		Hint:    "configure channels.feishu.appId in openclaw.json",
	})
}

// ── getBindMsg tests ──

func TestGetBindMsg_Zh(t *testing.T) {
	msg := getBindMsg("zh")
	if want := "你想在哪个 Agent 中使用 lark-cli?"; msg.SelectSource != want {
		t.Errorf("zh SelectSource = %q, want %q", msg.SelectSource, want)
	}
	if want := "你希望 AI 如何与你协作？"; msg.SelectIdentity != want {
		t.Errorf("zh SelectIdentity = %q, want %q", msg.SelectIdentity, want)
	}
	if want := "以机器人身份"; msg.IdentityBotOnly != want {
		t.Errorf("zh IdentityBotOnly = %q, want %q", msg.IdentityBotOnly, want)
	}
}

func TestGetBindMsg_En(t *testing.T) {
	msg := getBindMsg("en")
	if want := "Which Agent are you running?"; msg.SelectSource != want {
		t.Errorf("en SelectSource = %q, want %q", msg.SelectSource, want)
	}
	if want := "As bot"; msg.IdentityBotOnly != want {
		t.Errorf("en IdentityBotOnly = %q, want %q", msg.IdentityBotOnly, want)
	}
}

func TestGetBindMsg_NonEnLang_FallsBackToZh(t *testing.T) {
	// Only zh and en TUI bundles exist; any non-English language (canonical
	// locale, short code, or unrecognized value) falls back to zh.
	for _, lang := range []i18n.Lang{"fr_fr", "ja_jp", "ko", "unknown", ""} {
		msg := getBindMsg(lang)
		if want := "你想在哪个 Agent 中使用 lark-cli?"; msg.SelectSource != want {
			t.Errorf("getBindMsg(%q) SelectSource = %q, want %q (zh fallback)", lang, msg.SelectSource, want)
		}
	}
}

// ── Resolve path edge case tests ──

func TestResolveOpenClawConfigPath_LegacyFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("OPENCLAW_HOME", home)

	legacyDir := filepath.Join(home, ".clawdbot")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "clawdbot.json")
	if err := os.WriteFile(legacyFile, []byte(`{}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := resolveOpenClawConfigPath()
	if got != legacyFile {
		t.Errorf("got %q, want legacy fallback %q", got, legacyFile)
	}
}

func TestResolveOpenClawConfigPath_DefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	t.Setenv("OPENCLAW_HOME", home)

	want := filepath.Join(home, ".openclaw", "openclaw.json")
	got := resolveOpenClawConfigPath()
	if got != want {
		t.Errorf("got %q, want default %q", got, want)
	}
}

// ── cleanupKeychainFromData ──

func TestCleanupKeychainFromData_InvalidJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Should not panic on invalid JSON
	cleanupKeychainFromData(f.Keychain, []byte("not json"), nil)
}

func TestCleanupKeychainFromData_ValidConfig(t *testing.T) {
	configData := []byte(`{"apps":[{"appId":"test_app","appSecret":{"ref":{"source":"keychain","id":"test_key"}}}]}`)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// Should not panic even when there is no new-app to keep.
	cleanupKeychainFromData(f.Keychain, configData, nil)
}

// statefulKeychain is a local in-memory KeychainAccess used only by the
// cleanup tests below. The package-wide noopKeychain in internal/cmdutil is
// intentionally untouched (it is pre-existing stable code) — this local mock
// gives the cleanup tests real Set/Get roundtrip semantics without changing
// any existing test infrastructure.
type statefulKeychain struct{ items map[string]string }

func newStatefulKeychain() *statefulKeychain {
	return &statefulKeychain{items: map[string]string{}}
}
func (k *statefulKeychain) key(service, account string) string {
	return service + "\x00" + account
}
func (k *statefulKeychain) Get(service, account string) (string, error) {
	return k.items[k.key(service, account)], nil
}
func (k *statefulKeychain) Set(service, account, value string) error {
	k.items[k.key(service, account)] = value
	return nil
}
func (k *statefulKeychain) Remove(service, account string) error {
	delete(k.items, k.key(service, account))
	return nil
}

// Rebinding the same appId MUST NOT delete the secret that ForStorage just
// wrote. This regression was observed in real use: the old config's secret
// key is identical to the new one (both derive from appId), and the
// indiscriminate cleanup clobbered it.
func TestCleanupKeychainFromData_KeepsSecretSharedWithNewApp(t *testing.T) {
	kc := newStatefulKeychain()

	const sharedID = "appsecret:cli_shared"
	if err := kc.Set("lark-cli", sharedID, "top-secret"); err != nil {
		t.Fatalf("seed keychain: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_shared","appSecret":{"source":"keychain","id":"` + sharedID + `"}}]}`)
	newApp := &core.AppConfig{
		AppId: "cli_shared",
		AppSecret: core.SecretInput{
			Ref: &core.SecretRef{Source: "keychain", ID: sharedID},
		},
	}

	cleanupKeychainFromData(kc, oldConfig, newApp)

	got, err := kc.Get("lark-cli", sharedID)
	if err != nil {
		t.Fatalf("keychain read after cleanup: %v", err)
	}
	if got != "top-secret" {
		t.Fatalf("shared secret was deleted; got %q, want %q", got, "top-secret")
	}
}

// When the new app uses a different keychain ID, the old app's secret still
// gets removed (that's the point of cleanup — reclaim stale entries).
func TestCleanupKeychainFromData_RemovesStaleSecretWhenAppIDChanges(t *testing.T) {
	kc := newStatefulKeychain()

	const oldID = "appsecret:cli_old"
	const newID = "appsecret:cli_new"
	if err := kc.Set("lark-cli", oldID, "old-secret"); err != nil {
		t.Fatalf("seed keychain: %v", err)
	}

	oldConfig := []byte(`{"apps":[{"appId":"cli_old","appSecret":{"source":"keychain","id":"` + oldID + `"}}]}`)
	newApp := &core.AppConfig{
		AppId: "cli_new",
		AppSecret: core.SecretInput{
			Ref: &core.SecretRef{Source: "keychain", ID: newID},
		},
	}

	cleanupKeychainFromData(kc, oldConfig, newApp)

	got, _ := kc.Get("lark-cli", oldID)
	if got != "" {
		t.Fatalf("stale secret should have been removed; still got %q", got)
	}
}

// TestHasStrictBotLock locks down the predicate's contract across every
// branch that warnIdentityEscalation depends on. Corrupt JSON is
// intentionally treated as "no lock" — commitBinding will overwrite the
// bad bytes anyway, matching the rest of the bind flow's lenient handling.
func TestHasStrictBotLock(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"bot lock present", `{"apps":[{"appId":"a","strictMode":"bot"}]}`, true},
		{"no strictMode field", `{"apps":[{"appId":"a"}]}`, false},
		{"explicit off", `{"apps":[{"appId":"a","strictMode":"off"}]}`, false},
		{"multi-app, one locked", `{"apps":[{"appId":"a"},{"appId":"b","strictMode":"bot"}]}`, true},
		{"empty apps array", `{"apps":[]}`, false},
		{"corrupt JSON → no lock", `{not-json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasStrictBotLock([]byte(c.in)); got != c.want {
				t.Errorf("hasStrictBotLock(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestConfigBindRun_LangExplicit_PrintsConfirmation covers the flag-mode
// confirmation line: when --lang is explicit, bind prints "language preference
// set" to stderr (rendered in the TUI language, embedding the preference value).
func TestConfigBindRun_LangExplicit_PrintsConfirmation(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	hermesHome := t.TempDir()
	t.Setenv("HERMES_HOME", hermesHome)
	if err := os.WriteFile(filepath.Join(hermesHome, ".env"), []byte("FEISHU_APP_ID=cli_abc\nFEISHU_APP_SECRET=secret\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	err := configBindRun(&BindOptions{
		Factory:      f,
		Source:       "hermes",
		Identity:     "bot-only",
		Lang:         "en",
		langExplicit: true,
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	// The short --lang en is canonicalized to en_us before the confirmation
	// echoes it back; the TUI language stays zh (flag mode, no picker).
	want := fmt.Sprintf(getBindMsg(i18n.LangZhCN).LangPreferenceSet, "en_us")
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Errorf("stderr = %q, want it to contain confirmation %q", got, want)
	}
}
