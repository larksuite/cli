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
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
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

func TestRootLong_AgentSkillsLinkTargetsReadmeSection(t *testing.T) {
	if !strings.Contains(rootLong, "https://github.com/larksuite/cli#agent-skills") {
		t.Fatalf("root help should link to the README Agent Skills section, got:\n%s", rootLong)
	}
	if strings.Contains(rootLong, "https://github.com/larksuite/cli#install-ai-agent-skills") {
		t.Fatalf("root help should not reference the removed install-ai-agent-skills anchor, got:\n%s", rootLong)
	}
}

func TestConfigureFlagCompletions(t *testing.T) {
	t.Cleanup(func() { cmdutil.SetFlagCompletionsDisabled(false) })

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
			cmdutil.SetFlagCompletionsDisabled(!tc.wantDisabled)
			configureFlagCompletions(tc.args)
			if got := cmdutil.FlagCompletionsDisabled(); got != tc.wantDisabled {
				t.Fatalf("FlagCompletionsDisabled() = %v, want %v", got, tc.wantDisabled)
			}
		})
	}
}
