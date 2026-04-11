// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/service"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

// buildIntegrationRootCmd creates a root command with api, service, and shortcut
// subcommands wired to a test factory, simulating the real CLI command tree.
func buildIntegrationRootCmd(t *testing.T, f *cmdutil.Factory) *cobra.Command {
	t.Helper()
	rootCmd := &cobra.Command{Use: "lark-cli"}
	rootCmd.SilenceErrors = true
	rootCmd.SetOut(f.IOStreams.Out)
	rootCmd.SetErr(f.IOStreams.ErrOut)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}
	rootCmd.AddCommand(api.NewCmdApi(f, nil))
	service.RegisterServiceCommands(rootCmd, f)
	shortcuts.RegisterShortcuts(rootCmd, f)
	return rootCmd
}

// executeRootIntegration runs a command through the full command tree and
// handleRootError, returning the exit code matching real CLI behavior.
func executeRootIntegration(t *testing.T, f *cmdutil.Factory, rootCmd *cobra.Command, args []string) int {
	t.Helper()
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		return handleRootError(f, err)
	}
	return 0
}

// parseEnvelope parses stderr bytes into an ErrorEnvelope.
func parseEnvelope(t *testing.T, stderr *bytes.Buffer) output.ErrorEnvelope {
	t.Helper()
	if stderr.Len() == 0 {
		t.Fatal("expected non-empty stderr, got empty")
	}
	var env output.ErrorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse stderr as ErrorEnvelope: %v\nstderr: %s", err, stderr.String())
	}
	return env
}

// assertEnvelope verifies exit code, stdout is empty, and stderr matches the
// expected ErrorEnvelope exactly via reflect.DeepEqual.
func assertEnvelope(t *testing.T, code int, wantCode int, stdout *bytes.Buffer, stderr *bytes.Buffer, want output.ErrorEnvelope) {
	t.Helper()
	if code != wantCode {
		t.Errorf("exit code: got %d, want %d", code, wantCode)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got:\n%s", stdout.String())
	}
	got := parseEnvelope(t, stderr)
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("stderr envelope mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

func buildStrictModeIntegrationRootCmd(t *testing.T, f *cmdutil.Factory) *cobra.Command {
	t.Helper()
	rootCmd := &cobra.Command{Use: "lark-cli"}
	rootCmd.SilenceErrors = true
	rootCmd.SetOut(f.IOStreams.Out)
	rootCmd.SetErr(f.IOStreams.ErrOut)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}
	rootCmd.AddCommand(auth.NewCmdAuth(f))
	rootCmd.AddCommand(api.NewCmdApi(f, nil))
	service.RegisterServiceCommands(rootCmd, f)
	shortcuts.RegisterShortcuts(rootCmd, f)
	if mode := f.ResolveStrictMode(context.Background()); mode.IsActive() {
		pruneForStrictMode(rootCmd, mode)
	}
	return rootCmd
}

func newStrictModeDefaultFactory(t *testing.T, profile string, mode core.StrictMode) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliDefaultAs, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	targetMode := mode
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
			{
				Name:       "target",
				AppId:      "app-target",
				AppSecret:  core.PlainSecret("secret-target"),
				Brand:      core.BrandFeishu,
				StrictMode: &targetMode,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := cmdutil.NewDefault(nil, cmdutil.InvocationContext{Profile: profile})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	f.IOStreams = &cmdutil.IOStreams{In: nil, Out: stdout, ErrOut: stderr}
	return f, stdout, stderr
}

func resetBuffers(stdout *bytes.Buffer, stderr *bytes.Buffer) {
	stdout.Reset()
	stderr.Reset()
}

func parseDryRunJSON(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	out := stdout.String()
	const prefix = "=== Dry Run ===\n"
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("expected dry-run prefix, got:\n%s", out)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(out, prefix)), &payload); err != nil {
		t.Fatalf("failed to parse dry-run payload: %v\nstdout: %s", err, out)
	}
	return payload
}

// --- api command ---

func TestIntegration_Api_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-api-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages",
		Body: map[string]interface{}{
			"code": 230002,
			"msg":  "Bot/User can NOT be out of the chat.",
			"error": map[string]interface{}{
				"log_id": "test-log-id-001",
			},
		},
	})

	rootCmd := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, rootCmd, []string{
		"api", "--as", "bot", "POST", "/open-apis/im/v1/messages",
		"--params", `{"receive_id_type":"chat_id"}`,
		"--data", `{"receive_id":"oc_xxx","msg_type":"text","content":"{\"text\":\"test\"}"}`,
	})

	// api uses MarkRaw: detail preserved, no enrichment
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    230002,
			Message: "API error: [230002] Bot/User can NOT be out of the chat.",
			Detail: map[string]interface{}{
				"log_id": "test-log-id-001",
			},
		},
	})
}

func TestIntegration_Api_PermissionError_NotEnriched(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-api-perm", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/perm",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled for this app",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "calendar:calendar:readonly"},
				},
				"log_id": "test-log-id-perm",
			},
		},
	})

	rootCmd := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, rootCmd, []string{
		"api", "--as", "bot", "GET", "/open-apis/test/perm",
	})

	// api uses MarkRaw: enrichment skipped, detail preserved, no console_url
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "permission",
			Code:    99991672,
			Message: "Permission denied [99991672]",
			Hint:    "check app permissions or re-authorize: lark-cli auth login",
			Detail: map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "calendar:calendar:readonly"},
				},
				"log_id": "test-log-id-perm",
			},
		},
	})
}

// --- service command ---

func TestIntegration_Service_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-svc-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_fake",
		Body: map[string]interface{}{
			"code": 99992356,
			"msg":  "id not exist",
			"error": map[string]interface{}{
				"log_id": "test-log-id-svc",
			},
		},
	})

	rootCmd := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "chats", "get", "--params", `{"chat_id":"oc_fake"}`, "--as", "bot",
	})

	// service: no MarkRaw, non-permission error — detail preserved
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    99992356,
			Message: "API error: [99992356] id not exist",
			Detail: map[string]interface{}{
				"log_id": "test-log-id-svc",
			},
		},
	})
}

func TestIntegration_Service_PermissionError_Enriched(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-svc-perm", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_test",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "im:chat:readonly"},
				},
			},
		},
	})

	rootCmd := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "chats", "get", "--params", `{"chat_id":"oc_test"}`, "--as", "bot",
	})

	// service: no MarkRaw — enrichment applied, detail cleared, console_url set
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:       "permission",
			Code:       99991672,
			Message:    "App scope not enabled: required scope im:chat:readonly [99991672]",
			Hint:       "enable the scope in developer console (see console_url)",
			ConsoleURL: "https://open.feishu.cn/page/scope-apply?clientID=e2e-svc-perm&scopes=im%3Achat%3Areadonly",
		},
	})
}

func TestIntegration_StrictModeBot_ProfileOverride_HidesCommandsInHelp(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeBot)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{"auth", "--help"})
	if code != 0 {
		t.Fatalf("auth --help exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "login") {
		t.Fatalf("auth --help should hide login in bot mode, got:\n%s", stdout.String())
	}

	resetBuffers(stdout, stderr)
	rootCmd = buildStrictModeIntegrationRootCmd(t, f)
	code = executeRootIntegration(t, f, rootCmd, []string{"im", "--help"})
	if code != 0 {
		t.Fatalf("im --help exit code = %d, want 0", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "+messages-search") {
		t.Fatalf("im --help should hide +messages-search in bot mode, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "+chat-create") {
		t.Fatalf("im --help should keep +chat-create in bot mode, got:\n%s", stdout.String())
	}
}

func TestIntegration_StrictModeBot_ProfileOverride_DirectAuthLoginReturnsEnvelope(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeBot)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"auth", "login", "--json", "--scope", "im:message.send_as_user",
	})

	assertEnvelope(t, code, output.ExitValidation, stdout, stderr, output.ErrorEnvelope{
		OK: false,
		Error: &output.ErrDetail{
			Type:    "strict_mode",
			Message: `strict mode is "bot", only bot identity is allowed. This setting is managed by the administrator and must not be modified by AI agents.`,
		},
	})
}

func TestIntegration_StrictModeBot_ProfileOverride_DirectUserShortcutReturnsEnvelope(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeBot)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "+messages-search", "--chat-id", "oc_xxx", "--query", "hello",
	})

	assertEnvelope(t, code, output.ExitValidation, stdout, stderr, output.ErrorEnvelope{
		OK: false,
		Error: &output.ErrDetail{
			Type:    "strict_mode",
			Message: `strict mode is "bot", only bot identity is allowed. This setting is managed by the administrator and must not be modified by AI agents.`,
		},
	})
}

func TestIntegration_StrictModeUser_ProfileOverride_ChatCreateDryRunSucceeds(t *testing.T) {
	// +chat-create supports both user and bot identities, so strict mode user
	// should allow it and force user identity.
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeUser)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "+chat-create", "--name", "probe", "--dry-run",
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if out == "" {
		t.Fatal("expected non-empty stdout for dry-run")
	}
}

func TestIntegration_StrictModeBot_ProfileOverride_ServiceDryRunForcesBotIdentity(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeBot)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "chats", "get", "--params", `{"chat_id":"oc_test"}`, "--as", "user", "--dry-run",
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", stderr.String())
	}
	payload := parseDryRunJSON(t, stdout)
	if got := payload["as"]; got != "bot" {
		t.Fatalf("dry-run as = %v, want bot", got)
	}
}

func TestIntegration_StrictModeUser_ProfileOverride_ServiceBotOnlyMethodReturnsEnvelope(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeUser)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "images", "create", "--data", `{"image_type":"message","image":"x"}`, "--dry-run",
	})

	assertEnvelope(t, code, output.ExitValidation, stdout, stderr, output.ErrorEnvelope{
		OK: false,
		Error: &output.ErrDetail{
			Type:    "strict_mode",
			Message: `strict mode is "user", only user identity is allowed. This setting is managed by the administrator and must not be modified by AI agents.`,
		},
	})
}

func TestIntegration_StrictModeBot_ProfileOverride_APIDryRunForcesBotIdentity(t *testing.T) {
	f, stdout, stderr := newStrictModeDefaultFactory(t, "target", core.StrictModeBot)
	rootCmd := buildStrictModeIntegrationRootCmd(t, f)

	code := executeRootIntegration(t, f, rootCmd, []string{
		"api", "--as", "user", "GET", "/open-apis/im/v1/chats/oc_test", "--dry-run",
	})

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got: %s", stderr.String())
	}
	payload := parseDryRunJSON(t, stdout)
	if got := payload["as"]; got != "bot" {
		t.Fatalf("dry-run as = %v, want bot", got)
	}
}

// --- shortcut command ---

func TestIntegration_Shortcut_BusinessError_OutputsEnvelope(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "e2e-sc-err", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		URL:    "/open-apis/im/v1/messages",
		Status: 400,
		Body: map[string]interface{}{
			"code": 230002,
			"msg":  "Bot/User can NOT be out of the chat.",
		},
	})

	rootCmd := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, rootCmd, []string{
		"im", "+messages-send", "--as", "bot", "--chat-id", "oc_xxx", "--text", "test",
	})

	// shortcut: no MarkRaw, no HandleResponse — error via DoAPIJSON path
	assertEnvelope(t, code, output.ExitAPI, stdout, stderr, output.ErrorEnvelope{
		OK:       false,
		Identity: "bot",
		Error: &output.ErrDetail{
			Type:    "api_error",
			Code:    230002,
			Message: "HTTP 400: Bot/User can NOT be out of the chat.",
		},
	})
}
