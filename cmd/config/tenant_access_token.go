// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bufio"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

func newCmdConfigTenantAccessToken(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant-access-token",
		Short: "Manage tenant access tokens in secure local storage",
		// Credential provisioning must work while the env provider is active, so
		// this group intentionally bypasses config's builtin-provider guard.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			c.SilenceUsage = true
			f.CurrentCommand = c
			return nil
		},
	}
	cmd.AddCommand(newCmdConfigTenantAccessTokenSet(f))
	cmd.AddCommand(newCmdConfigTenantAccessTokenRemove(f))
	return cmd
}

func newCmdConfigTenantAccessTokenSet(f *cmdutil.Factory) *cobra.Command {
	var appID string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Store a tenant access token read from stdin",
		Long: "Store a tenant access token for an App ID in secure local storage.\n" +
			"The token is always read from stdin and is never accepted as an argument or flag.\n" +
			"Set LARKSUITE_CLI_TENANT_ACCESS_TOKEN_SOURCE=credential-store to select the stored token\n" +
			"for bot requests in an env-selected account.",
		Args: rejectTenantTokenPositionals,
		RunE: func(_ *cobra.Command, _ []string) error {
			return configTenantAccessTokenSetRun(f, appID)
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "App ID whose tenant access token is stored")
	_ = cmd.MarkFlagRequired("app-id")
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func newCmdConfigTenantAccessTokenRemove(f *cmdutil.Factory) *cobra.Command {
	var appID string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a stored tenant access token",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return configTenantAccessTokenRemoveRun(f, appID)
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "App ID whose stored tenant access token is removed")
	_ = cmd.MarkFlagRequired("app-id")
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func rejectTenantTokenPositionals(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"positional arguments are not accepted; pipe the tenant access token to stdin").
		WithHint("pipe exactly one token line to stdin")
}

func configTenantAccessTokenSetRun(f *cmdutil.Factory, appID string) error {
	if f == nil || f.IOStreams == nil || f.IOStreams.In == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"stdin is empty, expected tenant access token")
	}
	if appID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--app-id is required").WithParam("--app-id")
	}

	scanner := bufio.NewScanner(f.IOStreams.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"failed to read tenant access token from stdin: %v", err).WithCause(err)
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"stdin is empty, expected tenant access token")
	}
	token := scanner.Text()
	if token == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"tenant access token must be provided as one non-empty line")
	}
	if scanner.Scan() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"tenant access token must be provided as exactly one line")
	}
	if err := scanner.Err(); err != nil {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"failed to read tenant access token from stdin: %v", err).WithCause(err)
	}

	store := credential.NewTenantTokenStore(func() keychain.KeychainAccess {
		return f.Keychain
	})
	if err := store.Set(appID, token); err != nil {
		return err
	}
	return output.WriteSuccessEnvelope(map[string]any{
		"appId":  appID,
		"stored": true,
	}, output.SuccessEnvelopeOptions{
		CommandPath: "lark-cli config tenant-access-token set",
		Out:         f.IOStreams.Out,
		ErrOut:      f.IOStreams.ErrOut,
	})
}

func configTenantAccessTokenRemoveRun(f *cmdutil.Factory, appID string) error {
	if f == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "tenant access token storage is unavailable")
	}
	if appID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--app-id is required").WithParam("--app-id")
	}
	store := credential.NewTenantTokenStore(func() keychain.KeychainAccess {
		return f.Keychain
	})
	if err := store.Remove(appID); err != nil {
		return err
	}
	return output.WriteSuccessEnvelope(map[string]any{
		"appId":   appID,
		"removed": true,
	}, output.SuccessEnvelopeOptions{
		CommandPath: "lark-cli config tenant-access-token remove",
		Out:         f.IOStreams.Out,
		ErrOut:      f.IOStreams.ErrOut,
	})
}
