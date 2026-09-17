// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/agentsupport"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
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

// Pins the strict-mode typed envelope: a failed_precondition
// *errs.ValidationError (exit 2) carrying the short historical Message,
// a Hint that still surfaces the policy layer + reason code (the
// safety-critical recovery info that lived in the legacy detail map),
// and the wrapped *platform.CommandDeniedError so external agents can
// still inspect the structured denial taxonomy via errors.As.
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

	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err is not *errs.ValidationError: %T", err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	if code := output.ExitCodeOf(err); code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (ExitValidation)", code, output.ExitValidation)
	}
	// Short historical Message is preserved verbatim.
	if verr.Message != `strict mode is "bot", only bot-identity commands are available` {
		t.Errorf("Message = %q, want short historical form", verr.Message)
	}
	// The denial layer + reason code remain user-readable in the hint, and
	// the historical switch-policy guidance is still appended.
	if !strings.Contains(verr.Hint, cmdpolicy.LayerStrictMode) {
		t.Errorf("Hint = %q, want substring %q (policy layer)", verr.Hint, cmdpolicy.LayerStrictMode)
	}
	if !strings.Contains(verr.Hint, "identity_not_supported") {
		t.Errorf("Hint = %q, want substring identity_not_supported (reason code)", verr.Hint)
	}
	if !strings.Contains(verr.Hint, "if the user explicitly wants to switch policy") {
		t.Errorf("Hint = %q, want historical switch-policy guidance", verr.Hint)
	}

	// The structured denial taxonomy survives on the wrapped cause.
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
	if cd.PolicySource != "strict-mode" {
		t.Errorf("CommandDeniedError.PolicySource = %q, want strict-mode", cd.PolicySource)
	}
	if !strings.Contains(cd.Reason, `strict mode is "bot"`) {
		t.Errorf("CommandDeniedError.Reason = %q, want substring 'strict mode is \"bot\"'", cd.Reason)
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

// The strict-mode stub carries a targeted config/strict-mode action alongside
// non-command policy context. Rendering for a concealed tree drops only the
// dead pointer and does not mutate the source error.
func TestStrictModeStub_ConfigHintUsesBuildLocalSurface(t *testing.T) {
	child := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error { return nil }}
	stub := strictModeStubFrom(child, core.StrictModeBot)
	source := stub.RunE(stub, nil)
	var original *errs.ValidationError
	if !errors.As(source, &original) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", source, source)
	}
	if original.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want failed_precondition", original.Subtype)
	}
	if !strings.Contains(original.Hint, "config strict-mode") {
		t.Fatalf("producer hint = %q, want config strict-mode", original.Hint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandConfigStrictMode: surface.CommandConcealed,
	})
	var concealed *errs.ValidationError
	if rendered := recovery.Render(source, plan); !errors.As(rendered, &concealed) {
		t.Fatalf("rendered error = %T, want *errs.ValidationError", rendered)
	}
	if concealed == original {
		t.Fatal("Render must clone the typed error")
	}
	if strings.Contains(concealed.Hint, "config strict-mode") {
		t.Errorf("concealed hint still contains config strict-mode: %q", concealed.Hint)
	}
	if !strings.Contains(concealed.Hint, "reason_code identity_not_supported") {
		t.Errorf("non-command policy guidance was lost: %q", concealed.Hint)
	}

	var visible *errs.ValidationError
	if !errors.As(recovery.Render(source, nil), &visible) ||
		!strings.Contains(visible.Hint, "config strict-mode") {
		t.Errorf("visible render must keep config strict-mode, got %+v", visible)
	}
	if !strings.Contains(original.Hint, "config strict-mode") {
		t.Errorf("concealed render mutated source hint: %q", original.Hint)
	}
}

// newAgentTestTree mirrors newTestTree but annotates the shipped verdict
// instead of the declared lane, since that is what the agent pass reads.
// Lanes are annotated too, so the cases that assert independence from the lane
// have something to be independent of.
func newAgentTestTree() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	noop := func(*cobra.Command, []string) error { return nil }

	svc := &cobra.Command{Use: "im"}
	root.AddCommand(svc)

	allowed := &cobra.Command{Use: "+send", Short: "allowed", RunE: noop}
	cmdutil.SetSupportedIdentities(allowed, []string{"user", "bot"})
	cmdutil.SetAgentSupport(allowed, "allowed")
	svc.AddCommand(allowed)

	// allowed + bot-only lane: the combination the run-time gate now lets
	// through, so the build pass must not withhold it either.
	allowedBotLane := &cobra.Command{Use: "+messages-edit", Short: "allowed, bot lane", RunE: noop}
	cmdutil.SetSupportedIdentities(allowedBotLane, []string{"bot"})
	cmdutil.SetAgentSupport(allowedBotLane, "allowed")
	svc.AddCommand(allowedBotLane)

	denied := &cobra.Command{Use: "+feed-shortcut-create", Short: "denied", RunE: noop}
	cmdutil.SetSupportedIdentities(denied, []string{"user"})
	cmdutil.SetAgentSupport(denied, "denied")
	svc.AddCommand(denied)

	unknown := &cobra.Command{Use: "+legacy", Short: "no verdict", RunE: noop}
	cmdutil.SetSupportedIdentities(unknown, []string{"user"})
	svc.AddCommand(unknown)

	// A denied leaf nested under a resource group, to cover the recursion.
	res := &cobra.Command{Use: "messages"}
	svc.AddCommand(res)
	nestedDenied := &cobra.Command{Use: "subscribe", RunE: noop}
	cmdutil.SetAgentSupport(nestedDenied, "denied")
	res.AddCommand(nestedDenied)

	return root
}

func TestPruneForAgentPrincipal(t *testing.T) {
	root := newAgentTestTree()
	pruneForAgentPrincipal(root)

	if cmd := findCmd(root, "im", "+feed-shortcut-create"); cmd == nil || !cmd.Hidden {
		t.Error("a denied command should be replaced by a hidden stub")
	}
	if cmd := findCmd(root, "im", "+send"); cmd == nil || cmd.Hidden {
		t.Error("an allowed command should be kept visible")
	}
	// The whole point of the run-time change, carried into the tree: the lane
	// no longer speaks for an Agent Employee, so a bot-only lane with an
	// allowed verdict stays available.
	if cmd := findCmd(root, "im", "+messages-edit"); cmd == nil || cmd.Hidden {
		t.Error("an allowed command on a bot-only lane should be kept visible")
	}
	if cmd := findCmd(root, "im", "+legacy"); cmd == nil || cmd.Hidden {
		t.Error("an unrecorded command should be kept visible, matching the run-time gate")
	}
	if cmd := findCmd(root, "im", "messages", "subscribe"); cmd == nil || !cmd.Hidden {
		t.Error("a denied command nested under a group should be replaced by a hidden stub")
	}
}

func TestPruneForAgentPrincipal_DirectDeniedCommandExplainsItself(t *testing.T) {
	root := newAgentTestTree()
	root.SilenceErrors = true
	root.SilenceUsage = true
	pruneForAgentPrincipal(root)
	root.SetArgs([]string{"im", "+feed-shortcut-create", "--name", "x"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected the stub to refuse the command")
	}
	if !strings.Contains(err.Error(), "not available for an Agent Employee profile") {
		t.Fatalf("refusal should name the reason, got: %v", err)
	}
	// Strict mode points at `config strict-mode`; this boundary has no such
	// exit, so the hint must not send an agent to a config command. Read the
	// hint off the typed error -- Error() does not carry it, which would make
	// this check pass no matter what the hint said.
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want a ValidationError, got %T: %v", err, err)
	}
	if vErr.Hint == "" {
		t.Fatal("refusal carried no hint")
	}
	if strings.Contains(vErr.Hint, "config strict-mode") {
		t.Fatalf("hint must not point at a config command an agent cannot use: %q", vErr.Hint)
	}
	var denied *platform.CommandDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("want a CommandDeniedError in the chain, got %T", err)
	}
	if denied.Layer != cmdpolicy.LayerAgentPrincipal {
		t.Errorf("layer = %q, want %q", denied.Layer, cmdpolicy.LayerAgentPrincipal)
	}
}

// TestPruneForAgentPrincipal_StubResistsUserPolicyRelabel pins that a user-layer
// rule cannot overwrite this boundary, the same protection strict mode has.
func TestPruneForAgentPrincipal_StubResistsUserPolicyRelabel(t *testing.T) {
	root := newAgentTestTree()
	pruneForAgentPrincipal(root)

	stub := findCmd(root, "im", "+feed-shortcut-create")
	if stub == nil {
		t.Fatal("expected the denied command to still resolve to a stub")
	}
	modified := cmdpolicy.Apply(root, map[string]cmdpolicy.Denial{
		cmdpolicy.CanonicalPath(stub): {
			Layer:      cmdpolicy.LayerPolicy,
			ReasonCode: "blocked_by_rule",
			Reason:     "a user rule matched the same path",
		},
	})
	if modified != 0 {
		t.Errorf("user-layer Apply modified %d commands, want 0", modified)
	}
	if got := stub.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerAgentPrincipal {
		t.Errorf("denial layer = %q, want it left at %q", got, cmdpolicy.LayerAgentPrincipal)
	}
}

// TestAgentPrincipalStubPicksHintByServiceAnnotation pins that the build-time
// refusal distinguishes the two situations CheckAgentEligibility distinguishes:
// a capability grant can plausibly unlock one and nothing local can unlock the
// other. Getting this backwards sends an agent to go ask for a grant that will
// never be enough, or tells it to go lobby a service team when a grant would
// have done.
func TestAgentPrincipalStubPicksHintByServiceAnnotation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		use           string
		shortcut      bool
		annotated     bool
		wantFragment  string
		avoidFragment string
	}{
		{
			name:          "service has agent-marked methods: point at a capability grant",
			use:           "list",
			annotated:     true,
			wantFragment:  "grant this Agent Employee the capability it needs",
			avoidFragment: "not yet marked as Agent-Employee-callable",
		},
		{
			name:          "service has no agent marker at all: say no local grant helps",
			use:           "list",
			annotated:     false,
			wantFragment:  "not yet marked as Agent-Employee-callable",
			avoidFragment: "grant this Agent Employee the capability it needs",
		},
		{
			// A shortcut is denied because one of the OpenAPIs it calls does
			// not take an agent token. No grant on the Agent platform changes
			// that, so sending the agent there is advice it cannot act on.
			name:          "shortcut: name the dependency, not a grant",
			use:           "+list",
			shortcut:      true,
			annotated:     true,
			wantFragment:  "does not accept an Agent Employee token",
			avoidFragment: "grant this Agent Employee the capability it needs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "root"}
			root.SilenceErrors = true
			root.SilenceUsage = true
			svc := &cobra.Command{Use: "okr"}
			root.AddCommand(svc)
			method := &cobra.Command{Use: tc.use, RunE: func(*cobra.Command, []string) error { return nil }}
			cmdutil.SetAgentSupport(method, agentsupport.Denied)
			cmdutil.SetAgentServiceAnnotated(method, tc.annotated)
			if tc.shortcut {
				cmdmeta.SetSource(method, cmdmeta.SourceShortcut, false)
			}
			svc.AddCommand(method)

			pruneForAgentPrincipal(root)
			root.SetArgs([]string{"okr", tc.use})
			err := root.Execute()
			if err == nil {
				t.Fatal("expected the stub to refuse the command")
			}
			// The hint lives on the typed error, not in Error(); asserting
			// against Error() would make every "must not contain" check pass
			// for the wrong reason.
			var vErr *errs.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("want a ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(vErr.Hint, tc.wantFragment) {
				t.Errorf("hint should contain %q, got: %q", tc.wantFragment, vErr.Hint)
			}
			if strings.Contains(vErr.Hint, tc.avoidFragment) {
				t.Errorf("hint should not contain %q, got: %q", tc.avoidFragment, vErr.Hint)
			}
		})
	}
}
