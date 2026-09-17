// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/hook"
	"github.com/spf13/cobra"
)

func TestBuildWithoutPluginsStillBuildsBuiltinCommands(t *testing.T) {
	root := Build(context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	if root == nil {
		t.Fatal("Build returned nil root")
	}
	if findCommand(root, "api") == nil {
		t.Fatal("builtin api command missing")
	}
	if findCommand(root, "docs +fetch") == nil {
		t.Fatal("builtin docs +fetch shortcut missing")
	}
}

func buildInternalForTest(
	t testing.TB,
	ctx context.Context,
	inv cmdutil.InvocationContext,
	opts ...BuildOption,
) (*cmdutil.Factory, *cobra.Command, *hook.Registry) {
	t.Helper()
	runtime, root, reg, err := assembleInternal(ctx, inv, assemblyRequest{}, resolveBuildConfig(opts))
	if err != nil {
		t.Fatalf("assemble full tree: %v", err)
	}
	return runtime.Factory, root, reg
}

func findCommand(root *cobra.Command, path string) *cobra.Command {
	parts := strings.Fields(path)
	cmd := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cmd = next
	}
	return cmd
}

// writeAgentProfileConfig points the CLI at a config whose active profile is an
// Agent Employee. Only the credential kind matters here: the prune pass reads
// the classification and never mints or sends anything.
func writeAgentProfileConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	config := `{
  "currentApp": "agent",
  "apps": [{"name": "agent", "appId": "cli_build_test_agent", "appSecret": "s",
            "brand": "feishu", "credentialKind": "aat",
            "principalOpenId": "ou_build_test", "users": []}]
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
}

// writeAgentProfileConfigWithUserStrictMode is writeAgentProfileConfig with the
// config-level strict mode set to user, the SupportedIdentities shape an
// injected AAT also produces.
func writeAgentProfileConfigWithUserStrictMode(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	config := `{
  "currentApp": "agent",
  "strictMode": "user",
  "apps": [{"name": "agent", "appId": "cli_build_test_agent", "appSecret": "s",
            "brand": "feishu", "credentialKind": "aat",
            "principalOpenId": "ou_build_test", "users": []}]
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
}

// TestBuildForAgentProfileIgnoresStrictModeForAllowedBotOnlyShortcut pins that
// the strict prune does not run for an Agent Employee. `im +messages-edit`
// declares a bot lane and is allowed by the verdict; under a user strict mode
// the strict pass would replace it with a stub before the agent pass ever saw
// it, and the refusal would talk about strict mode to a profile that has no
// lane to switch to.
func TestBuildForAgentProfileIgnoresStrictModeForAllowedBotOnlyShortcut(t *testing.T) {
	writeAgentProfileConfigWithUserStrictMode(t)
	_, root, _ := buildInternalForTest(t, context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	edit := findCommand(root, "im +messages-edit")
	if edit == nil {
		t.Fatal("im +messages-edit must be in the tree for an Agent Employee")
	}
	if layer := edit.Annotations[cmdpolicy.AnnotationDenialLayer]; layer != "" {
		t.Fatalf("im +messages-edit was replaced by a %s stub; the verdict allows it", layer)
	}
	if edit.Hidden {
		t.Fatal("im +messages-edit must stay visible in help for an Agent Employee")
	}
}

// writeStrictBotProfileConfig points the CLI at a standard app whose
// config-level strict mode is bot.
func writeStrictBotProfileConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	config := `{
  "currentApp": "bot",
  "strictMode": "bot",
  "apps": [{"name": "bot", "appId": "cli_build_test_bot", "appSecret": "s",
            "brand": "feishu", "users": []}]
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
}

// TestBuildForStrictBotLeavesNoEmptyServiceShells is the strict-mode twin of
// the Agent Employee case: a service with nothing left to run must not be
// advertised. approval is the case to pin -- every one of its Catalog methods
// is user-only and it has no shortcuts, so under bot strict mode nothing under
// it is reachable. The two prune passes share one emptiness rule now that the
// unknown-subcommand guard is installed after them rather than before.
func TestBuildForStrictBotLeavesNoEmptyServiceShells(t *testing.T) {
	writeStrictBotProfileConfig(t)
	_, root, _ := buildInternalForTest(t, context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	approval := findCommand(root, "approval")
	if approval == nil {
		t.Fatal("approval should stay dispatchable so a direct call still explains itself")
	}
	if !approval.Hidden {
		t.Errorf("approval has nothing a bot can run, so help must not offer it; visible children: %v",
			visibleSubcommands(approval))
	}
	if got := visibleSubcommands(approval); len(got) > 0 {
		t.Errorf("approval must not list resource groups that are themselves empty, got: %v", got)
	}
}

// visibleSubcommands lists the children cobra would print under a command.
func visibleSubcommands(cmd *cobra.Command) []string {
	var names []string
	for _, child := range cmd.Commands() {
		if child.IsAvailableCommand() {
			names = append(names, child.Name())
		}
	}
	return names
}

// TestBuildForAgentProfileLeavesNoEmptyServiceShells assembles the real tree for
// an Agent Employee profile, which is the only layer where this can be checked:
// the prune pass runs after installUnknownSubcommandGuard has given every pure
// group a RunE, and a unit test that calls the pass directly never sees that.
//
// okr is the case to pin. Every one of its shortcuts and every one of its
// Catalog methods is denied to an Agent Employee, so nothing under it is
// reachable; before this was fixed, help still offered `okr` and, inside it,
// eleven resource groups that were themselves empty.
func TestBuildForAgentProfileLeavesNoEmptyServiceShells(t *testing.T) {
	writeAgentProfileConfig(t)
	_, root, _ := buildInternalForTest(t, context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	okr := findCommand(root, "okr")
	if okr == nil {
		t.Fatal("okr should stay dispatchable so a direct call still explains itself")
	}
	if !okr.Hidden {
		t.Errorf("okr has nothing an Agent Employee can run, so help must not offer it; "+
			"visible children: %v", visibleSubcommands(okr))
	}
	if got := visibleSubcommands(okr); len(got) > 0 {
		t.Errorf("okr must not list resource groups that are themselves empty, got: %v", got)
	}
	for _, group := range []string{"cycles", "objectives", "alignments"} {
		child := findCommand(root, "okr "+group)
		if child == nil {
			continue
		}
		if !child.Hidden {
			t.Errorf("okr %s is empty for an Agent Employee and must be hidden; visible children: %v",
				group, visibleSubcommands(child))
		}
	}
}

// TestBuildForAgentProfileKeepsPartiallySupportedServices guards the other
// direction: a service that keeps even one runnable command stays visible, so
// the pass cannot degrade into hiding everything.
func TestBuildForAgentProfileKeepsPartiallySupportedServices(t *testing.T) {
	writeAgentProfileConfig(t)
	_, root, _ := buildInternalForTest(t, context.Background(), cmdutil.InvocationContext{}, WithoutPlugins())

	im := findCommand(root, "im")
	if im == nil || im.Hidden {
		t.Fatal("im keeps most of its commands for an Agent Employee and must stay visible")
	}
	if len(visibleSubcommands(im)) == 0 {
		t.Error("im should still offer its allowed commands")
	}
	// A denied shortcut is withheld from help but still dispatchable.
	if denied := findCommand(root, "im +feed-shortcut-create"); denied == nil || !denied.Hidden {
		t.Error("a denied shortcut should remain as a hidden stub, not vanish from the tree")
	}
}
