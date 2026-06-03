// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/hook"
	internalplatform "github.com/larksuite/cli/internal/platform"
	"github.com/larksuite/cli/internal/vfs"
)

// userPolicyFileName is the conventional filename for the user-layer Rule.
// Lives under ~/.lark-cli/ to match the rest of the CLI's user-state
// directory.
const userPolicyFileName = "policy.yml"

// applyUserPolicyPruning resolves the user-layer Rule from plugin
// contributions and/or ~/.lark-cli/policy.yml and installs denyStubs
// for commands it rejects.
//
// Missing yaml is not an error -- the CLI runs with no user-layer
// restriction. A malformed Rule (bad MaxRisk enum, malformed glob, etc.)
// surfaces via the returned error; the caller decides how to handle it.
//
// pluginRules carries Plugin.Restrict() contributions collected from
// the InstallAll phase; nil/empty is fine.
func applyUserPolicyPruning(rootCmd *cobra.Command, pluginRules []cmdpolicy.PluginRule) error {
	// Plugin rules shadow the yaml source entirely (Resolve: plugin >
	// yaml). When a plugin contributed rules we therefore do NOT even
	// read ~/.lark-cli/policy.yml: build.go fail-CLOSES on any policy
	// error once a plugin is present, so reading a malformed yaml here
	// would let an unrelated broken file on the user's machine abort a
	// plugin-governed binary -- exactly the file the plugin is supposed
	// to shadow. Skipping the read keeps the shadow contract honest.
	var (
		yamlRules []*platform.Rule
		yamlPath  string
	)
	if len(pluginRules) == 0 {
		p, perr := userPolicyPath()
		if perr != nil {
			// No user home dir means we cannot locate the policy. Treat
			// the same as "file missing": no pruning, no error. This keeps
			// non-interactive CI environments (no HOME set) running.
			p = ""
		}
		yamlPath = p
		loaded, lerr := cmdpolicy.LoadYAMLPolicy(yamlPath)
		if lerr != nil {
			// Yaml-only failures are fail-OPEN at the caller (warn and
			// continue), but the active-policy snapshot is process-global
			// and may still carry data from a previous build in long-lived
			// embedders / tests. Clear it explicitly so `config policy
			// show` reports "no policy" instead of a stale rule that
			// doesn't reflect the current command tree.
			cmdpolicy.SetActive(nil)
			return lerr
		}
		yamlRules = loaded
	}

	rules, source, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: pluginRules,
		YAMLRules:   yamlRules,
		YAMLPath:    yamlPath,
	})
	if err != nil {
		cmdpolicy.SetActive(nil)
		return err
	}
	if len(rules) == 0 {
		cmdpolicy.SetActive(&cmdpolicy.ActivePolicy{Source: source})
		return nil
	}

	// RuleName attributes a denial to a specific rule in the envelope.
	// With a single rule that is unambiguous and preserves the legacy
	// envelope verbatim; with several rules a denial means "no rule
	// granted it", which has no single owner, so the field is left empty
	// and reason_code=no_matching_rule carries the meaning instead.
	ruleName := ""
	if len(rules) == 1 {
		ruleName = rules[0].Name
	}

	engine := cmdpolicy.NewSet(rules)
	decisions := engine.EvaluateAll(rootCmd)
	denied := cmdpolicy.BuildDeniedByPath(rootCmd, decisions, source, ruleName)
	cmdpolicy.Apply(rootCmd, denied)

	cmdpolicy.SetActive(&cmdpolicy.ActivePolicy{
		Rules:       rules,
		Source:      source,
		DeniedPaths: len(denied),
	})
	return nil
}

// installPluginsAndHooks runs the InstallAll phase on the globally-
// registered plugins, returning the Plugin.Restrict contributions for
// cmdpolicy and the populated hook.Registry for the runtime wrapper.
// Errors from FailClosed plugins propagate; FailOpen failures are
// warned to errOut and the loop continues.
func installPluginsAndHooks(errOut io.Writer) (*internalplatform.InstallResult, error) {
	plugins := platform.RegisteredPlugins()
	if len(plugins) == 0 {
		return &internalplatform.InstallResult{Registry: nil}, nil
	}
	return internalplatform.InstallAll(plugins, errOut)
}

// recordInventory builds and stores the plugin inventory snapshot for
// diagnostic commands (config plugins show) to read at runtime. Called
// once from build.go after applyUserPolicyPruning + wireHooks succeed.
func recordInventory(installResult *internalplatform.InstallResult) {
	if installResult == nil {
		internalplatform.SetActiveInventory(nil)
		return
	}
	pluginSrcs := make([]internalplatform.PluginInventorySource, 0, len(installResult.Plugins))
	for _, p := range installResult.Plugins {
		pluginSrcs = append(pluginSrcs, internalplatform.PluginInventorySource{
			Name:         p.Name,
			Version:      p.Version,
			Capabilities: p.Capabilities,
		})
	}
	ruleSrcs := make([]internalplatform.RuleInventorySource, 0, len(installResult.PluginRules))
	for _, r := range installResult.PluginRules {
		if r.Rule == nil {
			continue
		}
		idents := make([]string, len(r.Rule.Identities))
		for i, id := range r.Rule.Identities {
			idents[i] = string(id)
		}
		ruleSrcs = append(ruleSrcs, internalplatform.RuleInventorySource{
			PluginName:       r.PluginName,
			Allow:            r.Rule.Allow,
			Deny:             r.Rule.Deny,
			MaxRisk:          string(r.Rule.MaxRisk),
			Identities:       idents,
			RuleName:         r.Rule.Name,
			Desc:             r.Rule.Description,
			AllowUnannotated: r.Rule.AllowUnannotated,
		})
	}
	internalplatform.SetActiveInventory(internalplatform.BuildInventory(pluginSrcs, installResult.Registry, ruleSrcs))
}

// wireHooks installs Observer/Wrapper hooks onto every runnable command
// and emits the Startup lifecycle event. The registry may be nil when
// no plugin contributed any hook -- the function short-circuits in
// that case to avoid useless RunE wrapping.
func wireHooks(ctx context.Context, rootCmd *cobra.Command, reg *hook.Registry) error {
	if reg == nil {
		return nil
	}
	hook.Install(rootCmd, reg, cobraCommandViewSource{})
	return hook.Emit(ctx, reg, platform.Startup, nil)
}

// cobraCommandViewSource is the default CommandViewSource: it returns a
// live view over the *cobra.Command. Strict-mode's Remove+Add stub
// (cmd/prune.go::strictModeStubFrom) explicitly forwards the original
// annotations + Short/Long so the live view keeps reporting Risk /
// Identities / Domain through the replacement. User-layer policy
// (cmdpolicy/apply.go::installDenyStub) mutates in place, preserving
// metadata trivially.
type cobraCommandViewSource struct{}

func (cobraCommandViewSource) View(cmd *cobra.Command) platform.CommandView {
	return cobraCommandView{cmd: cmd}
}

// cobraCommandView adapts *cobra.Command to the CommandView interface.
type cobraCommandView struct {
	cmd *cobra.Command
}

func (v cobraCommandView) Path() string {
	return cmdpolicy.CanonicalPath(v.cmd)
}

func (v cobraCommandView) Domain() string {
	for c := v.cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if v, ok := c.Annotations["cmdmeta.domain"]; ok && v != "" {
			return v
		}
	}
	return ""
}

func (v cobraCommandView) Risk() (platform.Risk, bool) {
	for c := v.cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if r, ok := c.Annotations["risk_level"]; ok && r != "" {
			return platform.Risk(r), true
		}
	}
	return "", false
}

func (v cobraCommandView) Identities() []platform.Identity {
	for c := v.cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if raw, ok := c.Annotations["lark:supportedIdentities"]; ok && raw != "" {
			parts := splitCSV(raw)
			out := make([]platform.Identity, len(parts))
			for i, p := range parts {
				out[i] = platform.Identity(p)
			}
			return out
		}
	}
	return nil
}

func (v cobraCommandView) Annotation(key string) (string, bool) {
	if v.cmd.Annotations == nil {
		return "", false
	}
	s, ok := v.cmd.Annotations[key]
	return s, ok
}

// splitCSV is a tiny csv-without-quotes helper. The
// lark:supportedIdentities annotation is always plain
// "user" / "bot" / "user,bot" without escaping.
func splitCSV(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// userPolicyPath returns the path of <baseConfigDir>/policy.yml.
//
// The base directory honours LARKSUITE_CLI_CONFIG_DIR (via
// core.GetBaseConfigDir) so that test isolation, container deployments
// and per-Agent config overrides all see a consistent policy location.
// Using vfs.UserHomeDir directly here would silently bypass the env
// override and route every test through the real ~/.lark-cli.
//
// The error return is retained for caller compatibility but is always
// nil today: GetBaseConfigDir falls back to a relative ".lark-cli" when
// the home dir can't be resolved, and the resolver already treats a
// missing file as "no policy".
func userPolicyPath() (string, error) {
	return filepath.Join(core.GetBaseConfigDir(), userPolicyFileName), nil
}

// warnPolicyError writes a one-line stderr warning when the user policy
// fails to load. V1 yaml errors are fail-OPEN -- the CLI keeps running
// without policy enforcement so the user can fix the typo. Plugin-supplied
// rules are fail-CLOSED instead because integrators take a code-level
// responsibility for them.
//
// Wrapped errors may carry the absolute policy path (os.PathError); fold
// the home prefix to "~" before emitting so stderr piped into agents /
// CI logs does not leak the user's home directory.
func warnPolicyError(errOut io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(errOut, "warning: user policy not applied: %s\n", redactHome(err.Error()))
}

func redactHome(s string) string {
	if home, err := vfs.UserHomeDir(); err == nil && home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	return s
}
