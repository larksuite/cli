// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

func NewCmdConfigPolicy(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "policy",
		Hidden: true,
		Short:  "Inspect the user-layer command policy",
		// Override parent's RequireBuiltinCredentialProvider check; this
		// group is read-only diagnostic and must work under any provider.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			c.SilenceUsage = true
			return nil
		},
	}
	cmd.AddCommand(newCmdConfigPolicyShow(f))
	return cmd
}

func newCmdConfigPolicyShow(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "show",
		Hidden: true,
		Short:  "Show the active user-layer policy (plugin / yaml / none)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPolicyShow(f)
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func runConfigPolicyShow(f *cmdutil.Factory) error {
	active := cmdpolicy.GetActive()
	if active == nil {
		output.PrintJson(f.IOStreams.Out, map[string]any{
			"source": string(cmdpolicy.SourceNone),
			"note":   "no policy recorded; bootstrap did not run pruning",
		})
		return nil
	}

	sourceName := ""
	if active.Source.Kind == cmdpolicy.SourcePlugin {
		sourceName = active.Source.Name
	}
	out := map[string]any{
		"source":       string(active.Source.Kind),
		"source_name":  sourceName,
		"denied_paths": active.DeniedPaths,
	}
	if active.Rule != nil {
		out["rule"] = map[string]any{
			"name":              active.Rule.Name,
			"description":       active.Rule.Description,
			"allow":             active.Rule.Allow,
			"deny":              active.Rule.Deny,
			"max_risk":          active.Rule.MaxRisk,
			"identities":        active.Rule.Identities,
			"allow_unannotated": active.Rule.AllowUnannotated,
		}
	}
	output.PrintJson(f.IOStreams.Out, out)
	return nil
}
