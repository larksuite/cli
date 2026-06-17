// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/sidecar"
)

func NewCmdConfigAuthProxy(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth-proxy",
		Short: "Manage trusted auth proxy hosts",
	}
	cmd.AddCommand(newCmdConfigAuthProxyTrust(f))
	cmd.AddCommand(newCmdConfigAuthProxyUntrust(f))
	cmd.AddCommand(newCmdConfigAuthProxyList(f))
	return cmd
}

func newCmdConfigAuthProxyTrust(f *cmdutil.Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "trust https://host[:port]",
		Short: "Trust a remote HTTPS auth proxy host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigAuthProxyTrust(f, args[0], yes)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm trusting this remote auth proxy host")
	cmdutil.SetRisk(cmd, cmdutil.RiskHighRiskWrite)
	return cmd
}

func newCmdConfigAuthProxyUntrust(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "untrust host[:port]",
		Short: "Remove a trusted remote auth proxy host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigAuthProxyUntrust(f, args[0])
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func newCmdConfigAuthProxyList(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List trusted remote auth proxy hosts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := core.LoadAuthProxyConfig()
			if err != nil {
				return output.Errorf(output.ExitValidation, "config", "failed to load auth proxy config: %v", err)
			}
			output.PrintJson(f.IOStreams.Out, map[string]any{
				"trustedHosts": cfg.TrustedHosts,
			})
			return nil
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func runConfigAuthProxyTrust(f *cmdutil.Factory, rawHost string, confirmed bool) error {
	if err := rejectAgentAuthProxyTrust(); err != nil {
		return err
	}
	host, err := sidecar.NormalizeRemoteProxyTrustHost(rawHost)
	if err != nil {
		return output.ErrValidation("invalid auth proxy host: %v", err)
	}
	if !confirmed {
		return authProxyTrustConfirmationRequired(host)
	}

	changed := false
	if err := core.UpdateAuthProxyConfig(func(cfg *core.AuthProxyConfig) {
		for _, existing := range cfg.TrustedHosts {
			normalized, err := sidecar.NormalizeRemoteProxyTrustHost(existing)
			if err == nil && normalized == host {
				return
			}
		}
		cfg.TrustedHosts = append(cfg.TrustedHosts, host)
		changed = true
	}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save auth proxy trust config: %v", err)
	}

	if changed {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Trusted auth proxy host %q", host))
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Auth proxy host %q already trusted", host))
	}
	output.PrintJson(f.IOStreams.Out, map[string]any{
		"trustedHost": host,
		"changed":     changed,
	})
	return nil
}

func rejectAgentAuthProxyTrust() error {
	ws := core.DetectWorkspaceFromEnv(os.Getenv)
	if ws.IsLocal() {
		return nil
	}
	return &errs.ValidationError{Problem: errs.Problem{
		Category: errs.CategoryValidation,
		Subtype:  errs.SubtypeInvalidArgument,
		Message:  fmt.Sprintf("refusing to trust a remote auth proxy from %s agent workspace; run this command outside the agent environment", ws.Display()),
		Hint:     "run `lark-cli config auth-proxy trust https://host[:port] --yes` from a normal user shell outside the agent environment after verifying the host.",
	}}
}

func authProxyTrustConfirmationRequired(host string) error {
	return &errs.ConfirmationRequiredError{
		Problem: errs.Problem{
			Category: errs.CategoryConfirmation,
			Subtype:  errs.SubtypeConfirmationRequired,
			Message:  fmt.Sprintf("trusting remote auth proxy host %q requires confirmation", host),
			Hint:     "Trusting a remote auth proxy allows future lark-cli requests, proxy session material, and business request bodies to flow through that host. Rerun from a normal user shell with --yes only after verifying the host.",
		},
		Risk:   cmdutil.RiskHighRiskWrite,
		Action: "config auth-proxy trust",
	}
}

func runConfigAuthProxyUntrust(f *cmdutil.Factory, rawHost string) error {
	host, err := sidecar.NormalizeRemoteProxyTrustHost(rawHost)
	if err != nil {
		return output.ErrValidation("invalid auth proxy host: %v", err)
	}

	changed := false
	if err := core.UpdateAuthProxyConfig(func(cfg *core.AuthProxyConfig) {
		next := cfg.TrustedHosts[:0]
		for _, existing := range cfg.TrustedHosts {
			normalized, err := sidecar.NormalizeRemoteProxyTrustHost(existing)
			if err == nil && normalized == host {
				changed = true
				continue
			}
			next = append(next, existing)
		}
		cfg.TrustedHosts = next
	}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save auth proxy trust config: %v", err)
	}

	if changed {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Removed trusted auth proxy host %q", host))
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Auth proxy host %q was not trusted", host))
	}
	output.PrintJson(f.IOStreams.Out, map[string]any{
		"trustedHost": host,
		"changed":     changed,
	})
	return nil
}
