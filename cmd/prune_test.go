// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

func newTestTree() *cobra.Command {
	root := &cobra.Command{Use: "root"}

	svc := &cobra.Command{Use: "im"}
	root.AddCommand(svc)

	noop := func(*cobra.Command, []string) error { return nil }

	userOnly := &cobra.Command{Use: "+search", Short: "user only", RunE: noop}
	cmdutil.SetSupportedIdentities(userOnly, []string{"user"})
	svc.AddCommand(userOnly)

	botOnly := &cobra.Command{Use: "+subscribe", Short: "bot only", RunE: noop}
	cmdutil.SetSupportedIdentities(botOnly, []string{"bot"})
	svc.AddCommand(botOnly)

	dual := &cobra.Command{Use: "+send", Short: "dual", RunE: noop}
	cmdutil.SetSupportedIdentities(dual, []string{"user", "bot"})
	svc.AddCommand(dual)

	noAnnotation := &cobra.Command{Use: "+legacy", Short: "no annotation", RunE: noop}
	svc.AddCommand(noAnnotation)

	res := &cobra.Command{Use: "messages"}
	svc.AddCommand(res)
	userMethod := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error { return nil }}
	cmdutil.SetSupportedIdentities(userMethod, []string{"user"})
	res.AddCommand(userMethod)

	auth := &cobra.Command{Use: "auth"}
	root.AddCommand(auth)
	login := &cobra.Command{Use: "login", RunE: noop}
	cmdutil.SetSupportedIdentities(login, []string{"user"})
	auth.AddCommand(login)

	return root
}

func findCmd(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == name {
				cmd = c
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cmd
}

func TestPruneForStrictMode_Bot(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	if cmd := findCmd(root, "im", "+search"); cmd == nil || !cmd.Hidden {
		t.Error("+search (user-only) should be replaced by a hidden stub in bot mode")
	}
	if findCmd(root, "im", "+subscribe") == nil {
		t.Error("+subscribe (bot-only) should be kept in bot mode")
	}
	if findCmd(root, "im", "+send") == nil {
		t.Error("+send (dual) should be kept in bot mode")
	}
	if findCmd(root, "im", "+legacy") == nil {
		t.Error("+legacy (no annotation) should be kept")
	}
	if cmd := findCmd(root, "im", "messages", "search"); cmd == nil || !cmd.Hidden {
		t.Error("search (user-only method) should be replaced by a hidden stub in bot mode")
	}
	if cmd := findCmd(root, "auth", "login"); cmd == nil || !cmd.Hidden {
		t.Error("auth login should be replaced by a hidden stub in bot mode")
	}
}

func TestPruneForStrictMode_User(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeUser)

	if findCmd(root, "im", "+search") == nil {
		t.Error("+search (user-only) should be kept in user mode")
	}
	if cmd := findCmd(root, "im", "+subscribe"); cmd == nil || !cmd.Hidden {
		t.Error("+subscribe (bot-only) should be replaced by a hidden stub in user mode")
	}
	if findCmd(root, "im", "+send") == nil {
		t.Error("+send (dual) should be kept in user mode")
	}
	if cmd := findCmd(root, "auth", "login"); cmd == nil || cmd.Hidden {
		t.Error("auth login should be kept in user mode")
	}
}

func TestPruneEmpty(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	if cmd := findCmd(root, "im", "messages"); cmd == nil || !cmd.Hidden {
		t.Error("resource 'messages' should be kept hidden when only hidden stubs remain")
	}
}

func TestPruneEmpty_PreservesOriginallyHiddenGroup(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	hidden := &cobra.Command{Use: "hidden", Hidden: true}
	root.AddCommand(hidden)
	hidden.AddCommand(&cobra.Command{
		Use:  "visible",
		RunE: func(*cobra.Command, []string) error { return nil },
	})

	pruneEmpty(root)

	if !hidden.Hidden {
		t.Fatal("expected originally hidden group to remain hidden")
	}
}

func TestPruneForStrictMode_Bot_DirectUserShortcutReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"im", "+search", "--query", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_Bot_DirectNestedUserMethodReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"im", "messages", "search", "--query", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_Bot_DirectAuthLoginReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeBot)
	root.SetArgs([]string{"auth", "login", "--json", "--scope", "im:message.send_as_user"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "bot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPruneForStrictMode_User_DirectBotShortcutReturnsStrictMode(t *testing.T) {
	root := newTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForStrictMode(root, core.StrictModeUser)
	root.SetArgs([]string{"im", "+subscribe", "--topic", "x"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected strict-mode error")
	}
	if !strings.Contains(err.Error(), `strict mode is "user"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Regression for codex C13: a strict-mode stub whose PARENT declares
// a PersistentPreRunE (e.g. cmd/auth/auth.go's external_provider
// check on env credentials) must surface the strict_mode envelope,
// not the parent's error. Cobra's "first PersistentPreRunE wins
// walking up from leaf" semantics will pick the parent's unless the
// stub itself carries its own.
//
// Fix: strictModeStubFrom installs a no-op PersistentPreRunE so cobra
// stops at the stub and proceeds to its RunE.
func TestStrictModeStub_BypassesParentPersistentPreRunE(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)
	stub := findCmd(root, "auth", "login")
	if stub == nil {
		t.Fatal("auth/login stub should exist after StrictModeBot")
	}
	if stub.PersistentPreRunE == nil {
		t.Fatal("strict-mode stub must declare PersistentPreRunE on leaf")
	}
	if err := stub.PersistentPreRunE(stub, nil); err != nil {
		t.Errorf("strict-mode stub PersistentPreRunE should be no-op, got %v", err)
	}
}

// Regression for codex H13: strict-mode stub must accept arbitrary
// positional args. With DisableFlagParsing=true, a user passing
// `auth login --scope ...` looks like 4 positional args; the original
// cobra.Args validator would surface a usage error BEFORE strict-mode
// stub's RunE.
func TestStrictModeStub_BypassesArgsValidator(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)
	stub := findCmd(root, "auth", "login")
	if stub == nil {
		t.Fatal("auth/login stub should exist after StrictModeBot")
	}
	if stub.Args == nil {
		t.Fatal("strict-mode stub must declare Args validator")
	}
	if err := stub.Args(stub, []string{"--scope", "im.message", "--profile", "default"}); err != nil {
		t.Errorf("strict-mode stub Args should accept flag-like args, got %v", err)
	}
}

// Pins the strict-mode envelope shape: structured detail.* / wrapped
// CommandDeniedError for external agents, AND the historical short
// Message + independent Hint for existing consumers.
func TestStrictModeStub_StructuredEnvelope(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)
	stub := findCmd(root, "im", "+search")
	if stub == nil {
		t.Fatalf("expected im/+search stub")
	}
	err := stub.RunE(stub, nil)
	if err == nil {
		t.Fatalf("strict-mode stub RunE should return error")
	}

	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *output.ExitError: %T", err)
	}
	if ee.Detail == nil {
		t.Fatalf("ExitError.Detail is nil; envelope writer cannot emit JSON")
	}
	if ee.Detail.Type != "command_denied" {
		t.Errorf("Detail.Type = %q, want command_denied", ee.Detail.Type)
	}
	dm, ok := ee.Detail.Detail.(map[string]any)
	if !ok {
		t.Fatalf("Detail.Detail = %T, want map[string]any", ee.Detail.Detail)
	}
	if got, _ := dm["layer"].(string); got != cmdpolicy.LayerStrictMode {
		t.Errorf("Detail.Detail[layer] = %q, want %q", got, cmdpolicy.LayerStrictMode)
	}
	if got, _ := dm["reason_code"].(string); got != "identity_not_supported" {
		t.Errorf("Detail.Detail[reason_code] = %q, want identity_not_supported", got)
	}
	if got, _ := dm["policy_source"].(string); got != "strict-mode" {
		t.Errorf("Detail.Detail[policy_source] = %q, want strict-mode", got)
	}

	var cd *platform.CommandDeniedError
	if !errors.As(err, &cd) {
		t.Fatalf("err does not unwrap to *platform.CommandDeniedError")
	}
	if cd.Layer != cmdpolicy.LayerStrictMode {
		t.Errorf("CommandDeniedError.Layer = %q, want %q", cd.Layer, cmdpolicy.LayerStrictMode)
	}
	if cd.ReasonCode != "identity_not_supported" {
		t.Errorf("CommandDeniedError.ReasonCode = %q, want identity_not_supported", cd.ReasonCode)
	}
	if !strings.Contains(cd.Reason, `strict mode is "bot"`) {
		t.Errorf("CommandDeniedError.Reason = %q, want substring 'strict mode is \"bot\"'", cd.Reason)
	}
	if ee.Detail.Message != `strict mode is "bot", only bot-identity commands are available` {
		t.Errorf("Detail.Message = %q, want short historical form", ee.Detail.Message)
	}
	if !strings.HasPrefix(ee.Detail.Hint, "if the user explicitly wants to switch policy") {
		t.Errorf("Detail.Hint = %q, want historical hint", ee.Detail.Hint)
	}
}

// strictModeStubFrom must write the denial annotations so the hook
// layer's populateInvocationDenial recognises the command as denied
// and physically isolates the Wrap chain. Without this, a plugin
// Wrapper registered against platform.All() could intercept the stub
// and silently return nil, swallowing the strict-mode error.
func TestStrictModeStub_HasDenialAnnotation(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, core.StrictModeBot)

	// im/+search is user-only -> replaced by a stub in StrictModeBot.
	stub := findCmd(root, "im", "+search")
	if stub == nil {
		t.Fatalf("expected im/+search stub to exist")
	}
	got := stub.Annotations[cmdpolicy.AnnotationDenialLayer]
	if got != cmdpolicy.LayerStrictMode {
		t.Errorf("stub annotation %q = %q, want %q",
			cmdpolicy.AnnotationDenialLayer, got, cmdpolicy.LayerStrictMode)
	}
	if src := stub.Annotations[cmdpolicy.AnnotationDenialSource]; src != "strict-mode" {
		t.Errorf("stub annotation %q = %q, want %q",
			cmdpolicy.AnnotationDenialSource, src, "strict-mode")
	}
}

// Audit / compliance observers fire even for strict-mode-denied commands
// and rely on CommandView.Risk() / Identities() / etc. The stub must
// carry the original command's annotations so those accessors keep
// returning meaningful values; the Short/Long are preserved so `--help`
// on a denied command still describes the original intent (parity with
// cmdpolicy/apply.go::installDenyStub).
func TestStrictModeStub_PreservesOriginalMetadata(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	svc := &cobra.Command{Use: "im"}
	root.AddCommand(svc)
	userOnly := &cobra.Command{
		Use:   "+search",
		Short: "search messages",
		Long:  "Search across IM history.",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	cmdutil.SetSupportedIdentities(userOnly, []string{"user"})
	cmdutil.SetRisk(userOnly, "read")
	svc.AddCommand(userOnly)

	pruneForStrictMode(root, core.StrictModeBot)

	stub := findCmd(root, "im", "+search")
	if stub == nil {
		t.Fatalf("expected im/+search stub")
	}
	if got := stub.Annotations["risk_level"]; got != "read" {
		t.Errorf("stub risk_level = %q, want %q (lost in replacement)", got, "read")
	}
	if got := stub.Annotations["lark:supportedIdentities"]; got != "user" {
		t.Errorf("stub supportedIdentities = %q, want %q", got, "user")
	}
	if stub.Short != "search messages" {
		t.Errorf("stub Short = %q, want preserved Short", stub.Short)
	}
	if stub.Long != "Search across IM history." {
		t.Errorf("stub Long = %q, want preserved Long", stub.Long)
	}
	// Denial stamps must still be present.
	if stub.Annotations[cmdpolicy.AnnotationDenialLayer] != cmdpolicy.LayerStrictMode {
		t.Errorf("denial annotation overwritten or missing")
	}
}
