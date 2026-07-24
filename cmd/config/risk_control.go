// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// NewCmdConfigRiskControl creates the workspace risk-control policy command.
func NewCmdConfigRiskControl(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "risk-control [on|off|default]",
		Short: "Manage workspace account-protection policy",
		Long: `View or set the account-protection risk-control policy for this workspace.

Account protection is on by default. Use off to opt this workspace out, on to
opt it back in explicitly, or default to remove the explicit preference.`,
		Args: cobra.MaximumNArgs(1),
		// This is persistent workspace policy, not credential management.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := core.LoadOrNotConfigured()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				printRiskControl(f, config)
				return nil
			}

			switch args[0] {
			case "on":
				enabled := true
				config.RiskControl = &enabled
			case "off":
				enabled := false
				config.RiskControl = &enabled
			case "default":
				config.RiskControl = nil
			default:
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"invalid risk-control value %q, valid values: on | off | default", args[0])
			}

			if err := core.SaveMultiAppConfig(config); err != nil {
				return errs.NewInternalError(errs.SubtypeStorage,
					"failed to save risk-control policy: %v", err).WithCause(err)
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "Risk control set to %s (workspace)\n", args[0])
			return nil
		},
	}
	cmdutil.SetRisk(cmd, cmdutil.RiskWrite)
	return cmd
}

func printRiskControl(f *cmdutil.Factory, config *core.MultiAppConfig) {
	source := "default"
	if config.RiskControl != nil {
		source = "workspace"
	}
	fmt.Fprintf(f.IOStreams.Out, "risk-control: %s (source: %s)\n", riskControlState(config.RiskControlEnabled()), source)
}

func riskControlState(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
