// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bufio"
	"context"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
)

// ImportTenantTokenOptions holds inputs for auth import-tenant-token.
type ImportTenantTokenOptions struct {
	Factory    *cmdutil.Factory
	Ctx        context.Context
	AppID      string
	TokenStdin bool
}

// NewCmdAuthImportTenantToken creates the local TAT import command.
func NewCmdAuthImportTenantToken(f *cmdutil.Factory, runF func(*ImportTenantTokenOptions) error) *cobra.Command {
	opts := &ImportTenantTokenOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "import-tenant-token",
		Short: "Import a tenant access token into secure local storage",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"positional arguments are not accepted; provide the tenant access token via stdin").
				WithHint("pipe the token to stdin and pass --token-stdin")
		},
		Long: `Import a caller-supplied tenant access token into lark-cli's cross-platform
secure local storage. The token must be read from stdin so it does not appear in
the process argument list. Existing values for the same App ID are overwritten.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// This command intentionally imports an external credential into local
			// storage, so it is the sole auth child that bypasses the parent
			// RequireBuiltinCredentialProvider guard.
			cmd.SilenceUsage = true
			f.CurrentCommand = cmd
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authImportTenantTokenRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID whose tenant token is being imported")
	cmd.Flags().BoolVar(&opts.TokenStdin, "token-stdin", false, "read the tenant access token from stdin")
	_ = cmd.MarkFlagRequired("app-id")
	cmdutil.MarkSensitiveFlag(cmd, "token-stdin")
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func authImportTenantTokenRun(opts *ImportTenantTokenOptions) error {
	if opts == nil || opts.Factory == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "tenant token import is unavailable")
	}
	if !credential.IsSafeInjectedTenantAppID(opts.AppID) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--app-id must use only lowercase letters, digits, '.', '_', or '-'").
			WithParam("--app-id")
	}
	if !opts.TokenStdin {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"tenant access token must be provided via stdin").
			WithHint("use --token-stdin and pipe the token").
			WithParam("--token-stdin")
	}
	if opts.Factory.IOStreams == nil || opts.Factory.IOStreams.In == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"stdin is empty, expected tenant access token").
			WithParam("--token-stdin")
	}

	scanner := bufio.NewScanner(opts.Factory.IOStreams.In)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"failed to read tenant access token from stdin: %v", err).
				WithCause(err).
				WithParam("--token-stdin")
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"stdin is empty, expected tenant access token").
			WithParam("--token-stdin")
	}
	token := scanner.Text()
	if !isValidTenantToken(token) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"tenant access token must be one non-empty line of visible ASCII characters").
			WithParam("--token-stdin")
	}
	if scanner.Scan() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"tenant access token must be provided as exactly one line").
			WithParam("--token-stdin")
	}
	if err := scanner.Err(); err != nil {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"failed to read tenant access token from stdin: %v", err).
			WithCause(err).
			WithParam("--token-stdin")
	}

	if err := credential.StoreInjectedTenantAccessToken(opts.Factory.Keychain, opts.AppID, token); err != nil {
		return err
	}
	return output.WriteSuccessEnvelope(map[string]any{
		"appId":  opts.AppID,
		"stored": true,
	}, output.SuccessEnvelopeOptions{
		CommandPath: "lark-cli auth import-tenant-token",
		Out:         opts.Factory.IOStreams.Out,
		ErrOut:      opts.Factory.IOStreams.ErrOut,
	})
}

func isValidTenantToken(token string) bool {
	if token == "" {
		return false
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x21 || token[i] > 0x7e {
			return false
		}
	}
	return true
}
