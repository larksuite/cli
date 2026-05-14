// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/schema"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
)

// TestPersistentPreRunE_AuthCheckDisabledAnnotations verifies that
// auth, config, and schema commands have auth check disabled,
// while api does not.
func TestPersistentPreRunE_AuthCheckDisabledAnnotations(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	authCmd := auth.NewCmdAuth(f)
	if !cmdutil.IsAuthCheckDisabled(authCmd) {
		t.Error("expected auth command to have auth check disabled")
	}

	configCmd := cmdconfig.NewCmdConfig(f)
	if !cmdutil.IsAuthCheckDisabled(configCmd) {
		t.Error("expected config command to have auth check disabled")
	}

	schemaCmd := schema.NewCmdSchema(f, nil)
	if !cmdutil.IsAuthCheckDisabled(schemaCmd) {
		t.Error("expected schema command to have auth check disabled")
	}

	apiCmd := api.NewCmdApi(f, nil)
	if cmdutil.IsAuthCheckDisabled(apiCmd) {
		t.Error("expected api command to NOT have auth check disabled")
	}
}

func TestPersistentPreRunE_AuthSubcommands(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	authCmd := auth.NewCmdAuth(f)
	for _, sub := range authCmd.Commands() {
		if !cmdutil.IsAuthCheckDisabled(sub) {
			t.Errorf("expected auth subcommand %q to inherit disabled auth check", sub.Name())
		}
	}
}

func TestPersistentPreRunE_ConfigSubcommands(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	configCmd := cmdconfig.NewCmdConfig(f)
	for _, sub := range configCmd.Commands() {
		if !cmdutil.IsAuthCheckDisabled(sub) {
			t.Errorf("expected config subcommand %q to inherit disabled auth check", sub.Name())
		}
	}
}

func TestHandleRootError_RawError_SkipsEnrichmentButWritesEnvelope(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	// Create a permission error (would normally be enriched) and mark it Raw
	err := output.ErrAPI(output.LarkErrAppScopeNotEnabled, "API error: [99991672] scope not enabled", map[string]interface{}{
		"permission_violations": []interface{}{
			map[string]interface{}{"subject": "calendar:calendar:readonly"},
		},
	})
	err.Raw = true

	code := handleRootError(f, err)
	if code != output.ExitAPI {
		t.Errorf("expected exit code %d, got %d", output.ExitAPI, code)
	}
	// stderr should contain the error envelope
	if stderr.Len() == 0 {
		t.Error("expected non-empty stderr for Raw error — WriteErrorEnvelope should always run")
	}
	// The message should NOT have been enriched by enrichPermissionError
	// (ErrAPI sets "Permission denied [code]" but enrichment would replace it with "App scope not enabled: ...")
	if strings.Contains(err.Error(), "App scope not enabled") {
		t.Errorf("expected message not enriched, got: %s", err.Error())
	}
	// Detail.Detail should be preserved (enrichPermissionError clears it to nil)
	if err.Detail != nil && err.Detail.Detail == nil {
		t.Error("expected Detail.Detail to be preserved, but it was cleared")
	}
}

func TestHandleRootError_NonRawError_EnrichesAndWritesEnvelope(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	// Create a permission error without Raw — should be enriched
	err := output.ErrAPI(output.LarkErrAppScopeNotEnabled, "API error: [99991672] scope not enabled", map[string]interface{}{
		"permission_violations": []interface{}{
			map[string]interface{}{"subject": "calendar:calendar:readonly"},
		},
	})

	code := handleRootError(f, err)
	if code != output.ExitAPI {
		t.Errorf("expected exit code %d, got %d", output.ExitAPI, code)
	}
	// stderr should contain the error envelope
	if stderr.Len() == 0 {
		t.Error("expected non-empty stderr for non-Raw error")
	}
	// The message should have been enriched
	if !strings.Contains(err.Error(), "App scope not enabled") {
		t.Errorf("expected enriched message, got: %s", err.Error())
	}
}

func TestEnrichPermissionError_SpecialCharsEscaped(t *testing.T) {
	tests := []struct {
		name      string
		appID     string
		scope     string
		wantInURL string // substring that must appear in console_url
		denyInURL string // substring that must NOT appear raw in console_url
	}{
		{
			name:      "ampersand in scope",
			appID:     "cli_good",
			scope:     "scope&evil=injected",
			wantInURL: "scopes=scope%26evil%3Dinjected",
			denyInURL: "scopes=scope&evil=injected",
		},
		{
			name:      "hash in scope",
			appID:     "cli_good",
			scope:     "scope#fragment",
			wantInURL: "scopes=scope%23fragment",
			denyInURL: "scopes=scope#fragment",
		},
		{
			name:      "space in scope",
			appID:     "cli_good",
			scope:     "scope with spaces",
			wantInURL: "scopes=scope+with+spaces",
		},
		{
			name:      "special chars in appID",
			appID:     "app&id=bad",
			scope:     "calendar:calendar:readonly",
			wantInURL: "clientID=app%26id%3Dbad",
			denyInURL: "clientID=app&id=bad",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				AppID: tt.appID, AppSecret: "test-secret", Brand: core.BrandFeishu,
			})

			exitErr := output.ErrAPI(output.LarkErrAppScopeNotEnabled, "scope not enabled", map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": tt.scope},
				},
			})

			handleRootError(f, exitErr)

			consoleURL := exitErr.Detail.ConsoleURL
			if consoleURL == "" {
				t.Fatal("expected console_url to be set")
			}
			if !strings.Contains(consoleURL, tt.wantInURL) {
				t.Errorf("console_url missing expected escaped value\n  want substring: %s\n  got url:        %s", tt.wantInURL, consoleURL)
			}
			if tt.denyInURL != "" && strings.Contains(consoleURL, tt.denyInURL) {
				t.Errorf("console_url contains unescaped dangerous value\n  deny substring: %s\n  got url:        %s", tt.denyInURL, consoleURL)
			}
		})
	}
}

func TestEnrichMissingScopeError_ServiceMethodUsesLocalScopesWhenNoUAT(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	f.ResolvedIdentity = core.AsUser

	var target registry.CommandEntry
	for _, entry := range registry.CollectCommandScopes([]string{"calendar"}, "user") {
		if len(entry.Scopes) == 1 && entry.Scopes[0] == "calendar:calendar.event:create" {
			target = entry
			break
		}
	}
	if target.Command == "" {
		t.Fatal("failed to locate a calendar create command in local registry metadata")
	}
	parts := strings.Split(target.Command, " ")
	if len(parts) != 2 {
		t.Fatalf("expected resource/method command, got %q", target.Command)
	}

	root := &cobra.Command{Use: "lark-cli"}
	serviceCmd := &cobra.Command{Use: "calendar"}
	resourceCmd := &cobra.Command{Use: parts[0]}
	methodCmd := &cobra.Command{Use: parts[1]}
	root.AddCommand(serviceCmd)
	serviceCmd.AddCommand(resourceCmd)
	resourceCmd.AddCommand(methodCmd)
	f.CurrentCommand = methodCmd

	exitErr := output.Errorf(output.ExitAPI, "api_error", "API call failed: %s", &internalauth.NeedAuthorizationError{})
	enrichMissingScopeError(f, exitErr)

	if exitErr.Code != output.ExitAPI {
		t.Fatalf("expected exit code %d, got %d", output.ExitAPI, exitErr.Code)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "api_error" {
		t.Fatalf("expected api_error detail, got %+v", exitErr.Detail)
	}
	if !strings.Contains(exitErr.Detail.Message, "need_user_authorization") {
		t.Fatalf("expected original need_user_authorization message, got %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "current command requires scope(s): calendar:calendar.event:create") {
		t.Fatalf("expected scope guidance in hint, got %q", exitErr.Detail.Hint)
	}
	if strings.Contains(exitErr.Detail.Hint, "lark-cli auth login --scope") {
		t.Fatalf("expected hint without auth login command, got %q", exitErr.Detail.Hint)
	}
	if exitErr.Detail.Detail != nil {
		t.Fatalf("expected detail to remain nil, got %#v", exitErr.Detail.Detail)
	}
}

func TestEnrichMissingScopeError_ShortcutUsesDeclaredScopesWhenNoUAT(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	f.ResolvedIdentity = core.AsUser

	root := &cobra.Command{Use: "lark-cli"}
	serviceCmd := &cobra.Command{Use: "docs"}
	shortcutCmd := &cobra.Command{Use: "+create"}
	root.AddCommand(serviceCmd)
	serviceCmd.AddCommand(shortcutCmd)
	f.CurrentCommand = shortcutCmd

	exitErr := output.ErrNetwork("API call failed: %s", &internalauth.NeedAuthorizationError{})
	enrichMissingScopeError(f, exitErr)

	if exitErr.Code != output.ExitNetwork {
		t.Fatalf("expected exit code %d, got %d", output.ExitNetwork, exitErr.Code)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "network" {
		t.Fatalf("expected network detail, got %+v", exitErr.Detail)
	}
	if !strings.Contains(exitErr.Detail.Message, "need_user_authorization") {
		t.Fatalf("expected original need_user_authorization message, got %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "current command requires scope(s): docx:document:create") {
		t.Fatalf("expected shortcut scope hint, got %q", exitErr.Detail.Hint)
	}
	if strings.Contains(exitErr.Detail.Hint, "lark-cli auth login --scope") {
		t.Fatalf("expected hint without auth login command, got %q", exitErr.Detail.Hint)
	}
	if exitErr.Detail.Detail != nil {
		t.Fatalf("expected detail to remain nil, got %#v", exitErr.Detail.Detail)
	}
}

func TestEnrichMissingScopeError_ShortcutIncludesConditionalScopes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	f.ResolvedIdentity = core.AsUser

	root := &cobra.Command{Use: "lark-cli"}
	serviceCmd := &cobra.Command{Use: "drive"}
	shortcutCmd := &cobra.Command{Use: "+status"}
	root.AddCommand(serviceCmd)
	serviceCmd.AddCommand(shortcutCmd)
	f.CurrentCommand = shortcutCmd

	exitErr := output.ErrNetwork("API call failed: %s", &internalauth.NeedAuthorizationError{})
	enrichMissingScopeError(f, exitErr)

	if exitErr.Detail == nil {
		t.Fatal("expected error detail")
	}
	if !strings.Contains(exitErr.Detail.Hint, "current command requires scope(s): drive:drive.metadata:readonly, drive:file:download") {
		t.Fatalf("expected conditional scope hint for drive +status, got %q", exitErr.Detail.Hint)
	}
}

func TestEnrichMissingScopeError_AppendsExistingHint(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	f.ResolvedIdentity = core.AsUser

	root := &cobra.Command{Use: "lark-cli"}
	serviceCmd := &cobra.Command{Use: "docs"}
	shortcutCmd := &cobra.Command{Use: "+create"}
	root.AddCommand(serviceCmd)
	serviceCmd.AddCommand(shortcutCmd)
	f.CurrentCommand = shortcutCmd

	exitErr := output.ErrNetwork("API call failed: %s", &internalauth.NeedAuthorizationError{})
	exitErr.Detail.Hint = "existing hint"
	enrichMissingScopeError(f, exitErr)

	want := "existing hint\ncurrent command requires scope(s): docx:document:create"
	if exitErr.Detail.Hint != want {
		t.Fatalf("expected appended hint %q, got %q", want, exitErr.Detail.Hint)
	}
}

func TestRootLong_AgentSkillsLinkTargetsReadmeSection(t *testing.T) {
	if !strings.Contains(rootLong, "https://github.com/larksuite/cli#agent-skills") {
		t.Fatalf("root help should link to the README Agent Skills section, got:\n%s", rootLong)
	}
	if strings.Contains(rootLong, "https://github.com/larksuite/cli#install-ai-agent-skills") {
		t.Fatalf("root help should not reference the removed install-ai-agent-skills anchor, got:\n%s", rootLong)
	}
}

func TestConfigureFlagCompletions(t *testing.T) {
	t.Cleanup(func() { cmdutil.SetFlagCompletionsEnabled(false) })

	tests := []struct {
		name         string
		args         []string
		wantDisabled bool
	}{
		{"plain command", []string{"im", "+send"}, true},
		{"help flag", []string{"im", "--help"}, true},
		{"no args", []string{}, true},
		{"__complete request", []string{"__complete", "im", "+send", ""}, false},
		{"completion subcommand", []string{"completion", "bash"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmdutil.SetFlagCompletionsEnabled(tc.wantDisabled)
			configureFlagCompletions(tc.args)
			if got := !cmdutil.FlagCompletionsEnabled(); got != tc.wantDisabled {
				t.Fatalf("FlagCompletionsEnabled() = %v, want disabled=%v", !got, tc.wantDisabled)
			}
		})
	}
}
