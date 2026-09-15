// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
)

// NewCmdConfigDPoP manages the per-profile DPoP policy for local credentials.
func NewCmdConfigDPoP(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dpop [disabled|preferred|required]",
		Short: "View or set DPoP token protection for the active profile",
		Long: `View or set DPoP token protection for the active profile.

The setting applies only to credentials issued by the built-in local provider.
Environment and extension credentials keep their existing Bearer behavior, and
the sandbox side of an auth sidecar delegates DPoP to the trusted sidecar.

disabled issues new local credentials as Bearer. preferred tries DPoP first and
may fall back before the token request when no supported signer exists. required
requires DPoP and fails closed. Existing DPoP tokens always keep their original
key regardless of the configured mode.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			multi, err := core.LoadOrNotConfigured()
			if err != nil {
				return err
			}
			app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				mode, modeErr := app.EffectiveDPoPMode()
				if modeErr != nil {
					return errs.NewConfigError(errs.SubtypeInvalidConfig, "%s", modeErr.Error()).WithCause(modeErr)
				}
				fmt.Fprintf(f.IOStreams.Out, "dpop: %s (profile %q, scope local)\n", mode, app.ProfileName())
				return nil
			}
			mode, err := core.ParseDPoPMode(args[0])
			if err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"invalid DPoP value %q, valid values: disabled | preferred | required", args[0]).
					WithCause(err)
			}
			if mode == core.DPoPModeRequired {
				if err := dpop.NewKeyStore(f.Keychain).ProbeWritableContext(cmd.Context()); err != nil {
					return errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
						"DPoP key storage is unavailable: %v", err).
						WithCause(err).
						WithHint("%s", dpop.KeyStoreUnavailableHint)
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
		},
	}
	cmdutil.SetRisk(cmd, cmdutil.RiskWrite)
	return cmd
}
