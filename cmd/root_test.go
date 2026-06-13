// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/larksuite/cli/internal/deprecation"
	"github.com/larksuite/cli/internal/output"
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

// TestHandleRootError_SecurityPolicyCanonicalEnvelope verifies that
// *errs.SecurityPolicyError flows through the canonical typed envelope
// (output.WriteTypedErrorEnvelope) — type=policy, numeric code, subtype,
// top-level identity, exit code 6 — after the dispatcher carve-out is removed.
func TestHandleRootError_SecurityPolicyCanonicalEnvelope(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	t.Run("21000 challenge_required", func(t *testing.T) {
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		errOut := &bytes.Buffer{}
		f.IOStreams.ErrOut = errOut

		spErr := &errs.SecurityPolicyError{
			Problem: errs.Problem{
				Category: errs.CategoryPolicy,
				Subtype:  errs.SubtypeChallengeRequired,
				Code:     21000,
				Message:  "blocked by access policy",
				Hint:     "complete challenge in your browser",
			},
			ChallengeURL: "https://example.com/challenge",
		}

		gotExit := handleRootError(f, spErr)
		if gotExit != int(output.ExitContentSafety) {
			t.Errorf("exit code = %d, want %d (ExitContentSafety)", gotExit, output.ExitContentSafety)
		}

		var env map[string]any
		if err := json.Unmarshal(errOut.Bytes(), &env); err != nil {
			t.Fatalf("envelope is not valid JSON: %v\n%s", err, errOut.String())
		}
		errObj, ok := env["error"].(map[string]any)
		if !ok {
			t.Fatalf("envelope missing top-level error object: %s", errOut.String())
		}
		if got := errObj["type"]; got != "policy" {
			t.Errorf("error.type = %v, want %q", got, "policy")
		}
		if got := errObj["subtype"]; got != "challenge_required" {
			t.Errorf("error.subtype = %v, want %q", got, "challenge_required")
		}
		if got, ok := errObj["code"].(float64); !ok || int(got) != 21000 {
			t.Errorf("error.code = %v (%T), want 21000 (number)", errObj["code"], errObj["code"])
		}
		if got := errObj["challenge_url"]; got != "https://example.com/challenge" {
			t.Errorf("error.challenge_url = %v, want challenge url", got)
		}
		if got := errObj["hint"]; got != "complete challenge in your browser" {
			t.Errorf("error.hint = %v, want hint message", got)
		}
		if _, exists := errObj["retryable"]; exists {
			t.Errorf("error.retryable leaked into canonical envelope: %v", errObj["retryable"])
		}
	})

	t.Run("21001 access_denied", func(t *testing.T) {
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		errOut := &bytes.Buffer{}
		f.IOStreams.ErrOut = errOut

		spErr := &errs.SecurityPolicyError{
			Problem: errs.Problem{
				Category: errs.CategoryPolicy,
				Subtype:  errs.SubtypeAccessDenied,
				Code:     21001,
				Message:  "access denied",
			},
		}

		gotExit := handleRootError(f, spErr)
		if gotExit != int(output.ExitContentSafety) {
			t.Errorf("exit code = %d, want %d", gotExit, output.ExitContentSafety)
		}

		var env map[string]any
		if err := json.Unmarshal(errOut.Bytes(), &env); err != nil {
			t.Fatalf("envelope is not valid JSON: %v\n%s", err, errOut.String())
		}
		errObj := env["error"].(map[string]any)
		if got := errObj["type"]; got != "policy" {
			t.Errorf("error.type = %v, want %q", got, "policy")
		}
		if got := errObj["subtype"]; got != "access_denied" {
			t.Errorf("error.subtype = %v, want %q", got, "access_denied")
		}
		if got, ok := errObj["code"].(float64); !ok || int(got) != 21001 {
			t.Errorf("error.code = %v, want 21001 (number)", errObj["code"])
		}
	})
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

// failingWriter writes up to limit bytes then returns io.ErrShortWrite on
// the write that would push past the limit. Used to simulate a stderr that
// dies mid-envelope.
type failingWriter struct {
	limit int
	n     int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.n+len(p) > f.limit {
		canWrite := f.limit - f.n
		if canWrite < 0 {
			canWrite = 0
		}
		f.n += canWrite
		return canWrite, io.ErrShortWrite
	}
	f.n += len(p)
	return len(p), nil
}

// TestHandleRootError_DeprecatedAliasMissingFlagStructured pins that a
// backward-compat alias failing on a cobra-level required flag (which
// short-circuits before RunE) routes through the structured envelope, so the
// deprecation notice OnInvoke records in PreRunE is carried on the wire instead
// of being dropped on a plain "Error:" line.
func TestHandleRootError_DeprecatedAliasMissingFlagStructured(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() { deprecation.SetPending(nil) })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	deprecation.SetPending(&deprecation.Notice{
		Command: "+write", Replacement: "+cells-set", Skill: "lark-sheets",
	})
	// The bare error shape cobra's ValidateRequiredFlags produces: not a typed
	// errs.* error, so it reaches the deprecation fallback.
	exit := handleRootError(f, fmt.Errorf(`required flag(s) %q not set`, "values"))

	out := errOut.String()
	if strings.HasPrefix(strings.TrimSpace(out), "Error:") {
		t.Fatalf("deprecation pending: want a structured envelope, got a plain Error: line:\n%s", out)
	}
	if !strings.Contains(out, `"message"`) || !strings.Contains(out, "values") {
		t.Errorf("expected a JSON error envelope carrying the failure message; got:\n%s", out)
	}
	// The envelope is typed validation, so the exit code must derive from that
	// category (2) — the wire type and the exit code must not disagree.
	if exit != int(output.ExitValidation) {
		t.Errorf("exit = %d, want %d (validation envelope → category-derived exit)", exit, int(output.ExitValidation))
	}
}

// TestHandleRootError_AuthConfigWireGolden is the wire-consistency regression
// baseline for auth/config errors: it pins the typed envelope and exit code the
// dispatcher produces for the two source-of-truth shapes, which are constructed
// typed at their origin in internal/auth and internal/core.
func TestHandleRootError_AuthConfigWireGolden(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	t.Run("token missing exits 3 with token_missing authentication envelope", func(t *testing.T) {
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		errOut := &bytes.Buffer{}
		f.IOStreams.ErrOut = errOut

		exit := handleRootError(f, internalauth.NewNeedUserAuthorizationError("u_golden"))
		if exit != int(output.ExitAuth) {
			t.Errorf("exit = %d, want %d (ExitAuth)", exit, int(output.ExitAuth))
		}

		errObj := decodeErrorEnvelope(t, errOut.Bytes())
		if got := errObj["type"]; got != "authentication" {
			t.Errorf("error.type = %v, want %q", got, "authentication")
		}
		if got := errObj["subtype"]; got != "token_missing" {
			t.Errorf("error.subtype = %v, want %q", got, "token_missing")
		}
		if got, _ := errObj["message"].(string); !strings.Contains(got, "need_user_authorization") {
			t.Errorf("error.message = %q, must keep the need_user_authorization marker", got)
		}
		if got, _ := errObj["message"].(string); !strings.Contains(got, "u_golden") {
			t.Errorf("error.message = %q, must carry the user open id", got)
		}
		if got, _ := errObj["hint"].(string); !strings.Contains(got, "auth login") {
			t.Errorf("error.hint = %q, must point at auth login", got)
		}
		if got := errObj["user_open_id"]; got != "u_golden" {
			t.Errorf("error.user_open_id = %v, want %q", got, "u_golden")
		}
	})

	t.Run("not configured exits 3 with not_configured config envelope", func(t *testing.T) {
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		errOut := &bytes.Buffer{}
		f.IOStreams.ErrOut = errOut

		exit := handleRootError(f, core.NotConfiguredError())
		if exit != int(output.ExitAuth) {
			t.Errorf("exit = %d, want %d (config shares ExitAuth)", exit, int(output.ExitAuth))
		}

		errObj := decodeErrorEnvelope(t, errOut.Bytes())
		if got := errObj["type"]; got != "config" {
			t.Errorf("error.type = %v, want %q", got, "config")
		}
		if got := errObj["subtype"]; got != "not_configured" {
			t.Errorf("error.subtype = %v, want %q", got, "not_configured")
		}
		if got, _ := errObj["message"].(string); !strings.Contains(got, "not configured") {
			t.Errorf("error.message = %q, want the not-configured message", got)
		}
		if got, _ := errObj["hint"].(string); !strings.Contains(got, "config init") {
			t.Errorf("error.hint = %q, must point at config init", got)
		}
	})
}

// decodeErrorEnvelope unmarshals a typed error envelope and returns its
// top-level "error" object, failing the test if the shape is unexpected.
func decodeErrorEnvelope(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\n%s", err, raw)
	}
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing top-level error object: %s", raw)
	}
	return errObj
}

// TestHandleRootError_NoDeprecationTypesUsageError pins that a residual cobra
// usage error (missing required flag) is typed as invalid_argument with exit 2
// even with no deprecation pending — never cobra's plain "Error:" line.
func TestHandleRootError_NoDeprecationTypesUsageError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() { deprecation.SetPending(nil) })
	deprecation.SetPending(nil)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	exit := handleRootError(f, fmt.Errorf(`required flag(s) %q not set`, "values"))

	out := errOut.String()
	if strings.HasPrefix(strings.TrimSpace(out), "Error:") {
		t.Fatalf("want a structured envelope, got a plain Error: line:\n%s", out)
	}
	errObj := decodeErrorEnvelope(t, errOut.Bytes())
	if got := errObj["type"]; got != "validation" {
		t.Errorf("error.type = %v, want %q", got, "validation")
	}
	if got, _ := errObj["message"].(string); !strings.Contains(got, "values") {
		t.Errorf("error.message = %q, must carry the failing flag name", got)
	}
	if exit != int(output.ExitValidation) {
		t.Errorf("exit = %d, want %d (validation envelope → category-derived exit)", exit, int(output.ExitValidation))
	}
}

// TestHandleRootError_LeakedUntypedErrorBecomesInternal pins that an untyped
// error that does NOT match a cobra usage shape (i.e. one that leaked past the
// typed boundary from a helper) is classified as an internal fault (exit 5),
// not blamed on the user's input as a validation error.
func TestHandleRootError_LeakedUntypedErrorBecomesInternal(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() { deprecation.SetPending(nil) })
	deprecation.SetPending(nil)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	exit := handleRootError(f, fmt.Errorf("upstream helper exploded: %w", io.ErrUnexpectedEOF))

	errObj := decodeErrorEnvelope(t, errOut.Bytes())
	if got := errObj["type"]; got != "internal" {
		t.Errorf("error.type = %v, want %q (leaked untyped error must not be mislabeled validation)", got, "internal")
	}
	if exit != int(output.ExitInternal) {
		t.Errorf("exit = %d, want %d (internal envelope → category-derived exit)", exit, int(output.ExitInternal))
	}
}

// TestHandleRootError_PartialWritePreservesExitCode pins that when the
// stderr write fails mid-envelope, handleRootError still returns the typed
// exit code (ExitAuth=3 for AuthenticationError), not fall through to the
// plain "Error:" path with exit 1. ExitCodeOf is computed from the typed
// err BEFORE the envelope write so the exit code is preserved even when
// the consumer's stderr pipe dies.
func TestHandleRootError_PartialWritePreservesExitCode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	w := &failingWriter{limit: 20}
	f.IOStreams.ErrOut = w

	err := errs.NewAuthenticationError(errs.SubtypeTokenExpired, "token expired")
	exit := handleRootError(f, err)
	if exit != int(output.ExitAuth) {
		t.Errorf("exit = %d, want %d (typed exit code preserved despite write failure)", exit, int(output.ExitAuth))
	}
}

// TestHandleRootError_BareErrorExitCodeNoStderr pins the silent-exit
// contract: a *output.BareError is honored for its exit code while stderr stays
// empty (stdout already carries the result, so the dispatcher must not layer a
// second envelope on top).
func TestHandleRootError_BareErrorExitCodeNoStderr(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	exit := handleRootError(f, output.ErrBare(output.ExitAuth))
	if exit != int(output.ExitAuth) {
		t.Errorf("exit = %d, want %d (BareError code propagated)", exit, int(output.ExitAuth))
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr must stay empty for a bare predicate signal, got:\n%s", errOut.String())
	}
}

// TestHandleRootError_TypedAuthErrorWithLegacyCausePreserved pins that a typed
// *errs.AuthenticationError carrying a legacy *NeedAuthorizationError in its
// Cause chain renders the producer's TokenExpired subtype + custom hint
// verbatim — the legacy sentinel in the Cause chain never coarsens the wire
// shape.
func TestHandleRootError_TypedAuthErrorWithLegacyCausePreserved(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	innerLegacy := &internalauth.NeedAuthorizationError{UserOpenId: "u_123"}
	outer := errs.NewAuthenticationError(errs.SubtypeTokenExpired, "token expired").
		WithHint("custom producer hint").
		WithCause(innerLegacy)

	exit := handleRootError(f, outer)
	if exit != int(output.ExitAuth) {
		t.Errorf("exit = %d, want %d (ExitAuth)", exit, int(output.ExitAuth))
	}
	got := errOut.String()
	if !strings.Contains(got, `"subtype": "token_expired"`) {
		t.Errorf("envelope lost producer Subtype TokenExpired; got %s", got)
	}
	if !strings.Contains(got, "custom producer hint") {
		t.Errorf("envelope lost producer Hint; got %s", got)
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
