// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
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
		{"__completeNoDesc request", []string{"__completeNoDesc", "im", "+send", ""}, false},
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

// isCompletionCommand must classify BOTH cobra completion aliases as
// completion requests so the Shutdown emit and update-notice paths skip
// shell-completion invocations. __completeNoDesc is an Alias of
// __complete (cobra/completions.go ShellCompNoDescRequestCmd) and
// dispatches the same RunE; bash/zsh completion typically calls the
// NoDesc variant.
func TestIsCompletionCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"plain command", []string{"im", "+send"}, false},
		{"__complete", []string{"__complete", "im"}, true},
		{"__completeNoDesc", []string{"__completeNoDesc", "im"}, true},
		{"completion subcommand", []string{"completion", "bash"}, true},
		{"completion in tail", []string{"foo", "bar", "completion"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCompletionCommand(tc.args); got != tc.want {
				t.Fatalf("isCompletionCommand(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestPromoteConfigError_* lives with the implementation in
// internal/errcompat/promote_test.go.

// TestHandleRootError_SecurityPolicyKeepsLegacyEnvelope pins the carve-out
// for *errs.SecurityPolicyError: it does NOT go through the typed envelope
// writer. Downstream OAuth/policy consumers parse a wire format that
// predates the typed taxonomy and depend on:
//   - error.type == "auth_error" (not the Category literal "policy")
//   - error.code is a string ("challenge_required" / "access_denied"), not a number
//   - error.retryable is present at the top of the error object
//   - exit code 1 (not ExitContentSafety 6)
//
// Migration of this category to the typed envelope is deferred to a later PR.
func TestHandleRootError_SecurityPolicyKeepsLegacyEnvelope(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cases := []struct {
		name     string
		subtype  errs.Subtype
		code     int
		wantCode string
	}{
		{"challenge_required", errs.SubtypeChallengeRequired, 21000, "challenge_required"},
		{"access_denied", errs.SubtypeAccessDenied, 21001, "access_denied"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			errOut := &bytes.Buffer{}
			f.IOStreams.ErrOut = errOut

			spErr := &errs.SecurityPolicyError{
				Problem: errs.Problem{
					Category: errs.CategoryPolicy,
					Subtype:  tc.subtype,
					Code:     tc.code,
					Message:  "blocked by access policy",
					Hint:     "complete challenge in your browser",
				},
				ChallengeURL: "https://example.com/challenge",
			}

			gotExit := handleRootError(f, spErr)
			if gotExit != 1 {
				t.Errorf("exit code = %d, want 1 (legacy carve-out)", gotExit)
			}

			var env map[string]any
			if err := json.Unmarshal(errOut.Bytes(), &env); err != nil {
				t.Fatalf("envelope is not valid JSON: %v\n%s", err, errOut.String())
			}
			errObj, ok := env["error"].(map[string]any)
			if !ok {
				t.Fatalf("envelope missing top-level error object: %s", errOut.String())
			}
			if got := errObj["type"]; got != "auth_error" {
				t.Errorf("error.type = %v, want %q", got, "auth_error")
			}
			if got := errObj["code"]; got != tc.wantCode {
				t.Errorf("error.code = %v (%T), want %q (string)", got, got, tc.wantCode)
			}
			if got, ok := errObj["retryable"].(bool); !ok || got {
				t.Errorf("error.retryable = %v (%T), want false (bool)", errObj["retryable"], errObj["retryable"])
			}
			if got := errObj["challenge_url"]; got != "https://example.com/challenge" {
				t.Errorf("error.challenge_url = %v, want challenge url", got)
			}
			if got := errObj["hint"]; got != "complete challenge in your browser" {
				t.Errorf("error.hint = %v, want hint message", got)
			}
			// And the typed-only fields must NOT appear on this envelope.
			for _, leaked := range []string{"subtype", "missing_scopes", "console_url"} {
				if _, exists := errObj[leaked]; exists {
					t.Errorf("error.%s leaked into legacy security envelope: %v", leaked, errObj[leaked])
				}
			}
		})
	}
}

// newAuthErrorWithNeedAuthMarker builds a typed *errs.AuthenticationError whose Message
// contains the need_user_authorization marker — the same shape that
// resolveAccessToken now produces when the credential chain returns
// *internalauth.NeedAuthorizationError.
func newAuthErrorWithNeedAuthMarker() *errs.AuthenticationError {
	cause := &internalauth.NeedAuthorizationError{UserOpenId: "u_xxx"}
	return &errs.AuthenticationError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthentication,
			Subtype:  errs.SubtypeUnknown,
			Message:  fmt.Sprintf("API call failed: %s", cause),
		},
		Cause: cause,
	}
}

// TestApplyNeedAuthorizationHint_ServiceMethodUsesLocalScopesWhenNoUAT pins
// that a typed AuthenticationError carrying the need_user_authorization marker gets a
// declared-scopes Hint appended when the current command is a registered
// service method.
func TestApplyNeedAuthorizationHint_ServiceMethodUsesLocalScopesWhenNoUAT(t *testing.T) {
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

	authErr := newAuthErrorWithNeedAuthMarker()
	applyNeedAuthorizationHint(f, authErr)

	if authErr.Category != errs.CategoryAuthentication {
		t.Errorf("Category = %q, want authentication", authErr.Category)
	}
	if !strings.Contains(authErr.Message, "need_user_authorization") {
		t.Errorf("Message should preserve need_user_authorization marker; got %q", authErr.Message)
	}
	if !strings.Contains(authErr.Hint, "current command requires scope(s): calendar:calendar.event:create") {
		t.Errorf("expected declared-scope hint, got %q", authErr.Hint)
	}
}

// TestApplyNeedAuthorizationHint_ShortcutUsesDeclaredScopesWhenNoUAT pins the
// same hint behavior for mounted shortcut commands.
func TestApplyNeedAuthorizationHint_ShortcutUsesDeclaredScopesWhenNoUAT(t *testing.T) {
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

	authErr := newAuthErrorWithNeedAuthMarker()
	applyNeedAuthorizationHint(f, authErr)

	if !strings.Contains(authErr.Hint, "current command requires scope(s): docx:document:create") {
		t.Errorf("expected shortcut scope hint, got %q", authErr.Hint)
	}
}

// TestApplyNeedAuthorizationHint_ShortcutIncludesConditionalScopes pins that
// conditional scopes declared on a shortcut surface in the hint.
func TestApplyNeedAuthorizationHint_ShortcutIncludesConditionalScopes(t *testing.T) {
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

	authErr := newAuthErrorWithNeedAuthMarker()
	applyNeedAuthorizationHint(f, authErr)

	if !strings.Contains(authErr.Hint, "current command requires scope(s): drive:drive.metadata:readonly, drive:file:download") {
		t.Errorf("expected conditional scope hint for drive +status, got %q", authErr.Hint)
	}
}

// TestApplyNeedAuthorizationHint_AppendsExistingHint pins that the
// declared-scopes guidance is appended (separated by newline) when the typed
// AuthenticationError already carries a Hint from elsewhere.
func TestApplyNeedAuthorizationHint_AppendsExistingHint(t *testing.T) {
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

	authErr := newAuthErrorWithNeedAuthMarker()
	authErr.Hint = "existing hint"
	applyNeedAuthorizationHint(f, authErr)

	want := "existing hint\ncurrent command requires scope(s): docx:document:create"
	if authErr.Hint != want {
		t.Errorf("expected appended hint %q, got %q", want, authErr.Hint)
	}
}
