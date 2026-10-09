// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
)

// NewCmdConfigSet creates the allowlisted generic config setter.
func NewCmdConfigSet(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a supported configuration value",
		Long: `Set a supported configuration value.

Supported keys:
  dpop  disabled | preferred | required

dpop applies only to credentials issued by the built-in local provider.
disabled issues new local credentials as Bearer. preferred tries DPoP first and
may fall back during new token issuance. required requires DPoP and fails
closed. Existing DPoP tokens always keep their original key regardless of the
configured mode.`,
		Args: validateConfigSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "dpop":
				multi, err := core.LoadOrNotConfigured()
				if err != nil {
					return err
				}
				app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
				if err != nil {
					return err
				}
				return setConfigDPoPMode(cmd.Context(), f, multi, app, args[1])
			default:
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"unsupported config key %q, supported keys: dpop", args[0]).
					WithParam("key")
			}
		},
	}
	cmdutil.SetRisk(cmd, cmdutil.RiskWrite)
	return cmd
}

func validateConfigSetArgs(_ *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"config set requires <key> and <value>").
			WithParam("args")
	}
	return nil
}

func setConfigDPoPMode(ctx context.Context, f *cmdutil.Factory, multi *core.MultiAppConfig, app *core.AppConfig, value string) error {
	mode, err := core.ParseDPoPMode(value)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid DPoP value %q, valid values: disabled | preferred | required", value).
			WithCause(err)
	}
	if mode == core.DPoPModeRequired {
		if err := dpop.NewKeyStore(nil).ProbeWritableContext(ctx); err != nil {
			return errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
				"DPoP key storage is unavailable: %v", err).
				WithCause(err).
				WithHint("%s", dpop.KeyStorePreExchangeUnavailableHint)
		}
	}
	app.SetDPoPMode(mode)
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to save DPoP policy: %v", err).WithCause(err)
	}
	fmt.Fprintf(f.IOStreams.ErrOut,
		"DPoP set to %s for local credentials in profile %q; existing DPoP bindings are unchanged\n",
		mode, app.ProfileName())
	return nil
}
