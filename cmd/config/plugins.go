// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// NewCmdConfigPlugins exposes the plugin inventory diagnostic command.
//
// `config policy show` is intentionally focused on the user-layer Rule
// (Restrict). Plugins also contribute hooks (Observe / Wrap / Lifecycle)
// that are not policy gates but still mutate the CLI's runtime behaviour.
// This command surfaces both halves so an operator can answer "what is
// this binary doing differently from stock lark-cli?" in one place.
//
// Like config policy show, the dispatch path is exempt from policy
// enforcement (see internal/cmdpolicy/diagnostic.go) so it remains
// usable under any Rule.
func NewCmdConfigPlugins(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "plugins",
		Hidden: true, // diagnostic-only; kept callable, omitted from --help so it stays out of AI-agent context
		Short:  "Inspect installed plugins and their hook contributions",
		// Same leaf-level no-op as config policy: the parent `config`
		// group's PersistentPreRunE requires builtin credential, but
		// this is a read-only diagnostic that must work everywhere.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			c.SilenceUsage = true
			return nil
		},
	}
	cmd.AddCommand(newCmdConfigPluginsShow(f))
	return cmd
}

func newCmdConfigPluginsShow(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "List successfully installed plugins, their rules, and registered hooks",
		Long: `Print every plugin that committed during bootstrap, including:

  - name / version / capabilities (FailurePolicy, Restricts, RequiredCLIVersion)
  - rule (when the plugin called r.Restrict)
  - hooks: observers (Before / After), wrappers, lifecycle handlers

Hooks are attributed by their namespaced name -- the framework prepends
the plugin name as the prefix at registration time, so an entry
"secaudit.audit-pre" belongs to plugin "secaudit".`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPluginsShow(f)
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func runConfigPluginsShow(f *cmdutil.Factory) error {
	inv := internalplatform.GetActiveInventory()
	if inv == nil {
		// Always emit the same field set as the populated branch so
		// AI agents and CI scripts don't have to branch on whether
		// `total` is present. `note` makes the unusual state explicit
		// for human readers.
		output.PrintJson(f.IOStreams.Out, map[string]any{
			"plugins": []any{},
			"total":   0,
			"note":    "no inventory recorded; bootstrap did not finish",
		})
		return nil
	}

	plugins := make([]map[string]any, 0, len(inv.Plugins))
	for _, p := range inv.Plugins {
		entry := map[string]any{
			"name":         p.Name,
			"version":      p.Version,
			"capabilities": p.Capabilities,
		}
		if p.Rule != nil {
			entry["rule"] = p.Rule
		}
		entry["hooks"] = map[string]any{
			"observers": p.Observers,
			"wrappers":  p.Wrappers,
			"lifecycle": p.Lifecycles,
			"count":     len(p.Observers) + len(p.Wrappers) + len(p.Lifecycles),
		}
		plugins = append(plugins, entry)
	}
	output.PrintJson(f.IOStreams.Out, map[string]any{
		"plugins": plugins,
		"total":   len(plugins),
	})
	return nil
}
