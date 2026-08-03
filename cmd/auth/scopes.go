// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// ScopesOptions holds all inputs for auth scopes.
type ScopesOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	Format  string
	JSON    bool
}

// NewCmdAuthScopes creates the auth scopes subcommand.
func NewCmdAuthScopes(f *cmdutil.Factory, runF func(*ScopesOptions) error) *cobra.Command {
	opts := &ScopesOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "Query scopes enabled for the app",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			if opts.JSON {
				opts.Format = "json"
			} else {
				opts.Format = strings.ToLower(strings.TrimSpace(opts.Format))
				if opts.Format != "json" && opts.Format != "pretty" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument,
						"unknown output format %q (want json or pretty)", opts.Format).
						WithParam("--format")
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return authScopesRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json (default) | pretty")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func authScopesRun(opts *ScopesOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "Querying app scopes...\n\n")
	appInfo, err := getAppInfoFn(opts.Ctx, f, config.AppID)
	if err != nil {
		// Discriminate by error type so transport / parse failures are not
		// reclassified as PermissionError(MissingScope) — re-auth does not
		// fix network / 5xx / JSON parse errors and misclassifying them
		// here would mislead agents into re-auth loops.
		//   - typed errors pass through unchanged
		//   - bare errors become InternalError(SubtypeSDKError) with Cause
		//     preserved so callers (errors.Is) can still see the underlying
		//     transport/parse failure.
		// Genuine permission failures are surfaced from appInfo *content*,
		// not from this transport-level error path.
		if errs.IsTyped(err) {
			return err
		}
		return errs.NewInternalError(errs.SubtypeSDKError,
			"failed to get app scope info: %v", err).WithCause(err)
	}
	data := map[string]interface{}{
		"appId":      config.AppID,
		"brand":      config.Brand,
		"tokenType":  "user",
		"userScopes": appInfo.UserScopes,
		"count":      len(appInfo.UserScopes),
	}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         f.IOStreams.Out,
		ErrOut:      f.IOStreams.ErrOut,
		CommandPath: "lark-cli auth scopes",
	})
	if opts.Format == "pretty" {
		return emitter.Value(data, output.StreamOptions{
			Format: output.FormatPretty,
			Pretty: func(w io.Writer, _ bool) error {
				if _, err := fmt.Fprintf(w, "App ID: %s\n", config.AppID); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(w, "Enabled scopes (%d):\n\n", len(appInfo.UserScopes)); err != nil {
					return err
				}
				for _, scope := range appInfo.UserScopes {
					if _, err := fmt.Fprintf(w, "  • %s\n", scope); err != nil {
						return err
					}
				}
				return nil
			},
		})
	}
	return emitter.Value(data, output.StreamOptions{Format: output.FormatJSON})
}
