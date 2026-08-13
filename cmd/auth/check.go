// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
)

// CheckOptions holds all inputs for auth check.
type CheckOptions struct {
	Factory *cmdutil.Factory
	Scope   string
	JSON    bool
}

// NewCmdAuthCheck creates the auth check subcommand.
func NewCmdAuthCheck(f *cmdutil.Factory, runF func(*CheckOptions) error) *cobra.Command {
	return newCmdAuthCheck(f, runF, nil)
}

func newCmdAuthCheck(
	f *cmdutil.Factory,
	runF func(*CheckOptions) error,
	projector *recovery.Projector,
) *cobra.Command {
	opts := &CheckOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check if current token has specified scopes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authCheckRunContext(cmd.Context(), opts, projector)
		},
	}

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to check (space-separated)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.MarkFlagRequired("scope")
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func authCheckRun(opts *CheckOptions) error {
	return authCheckRunContext(context.Background(), opts, nil)
}

func authCheckRunContext(ctx context.Context, opts *CheckOptions, projector *recovery.Projector) error {
	f := opts.Factory

	required := strings.Fields(opts.Scope)
	if len(required) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--scope cannot be empty").WithParam("--scope")
	}

	config, err := f.Config()
	if err != nil {
		return err
	}
	if f.Credential == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "credential inspection is unavailable")
	}

	inspection, err := f.Credential.InspectToken(ctx, credential.TokenInspectionRequest{
		TokenSpec: credential.TokenSpec{
			Type:  credential.TokenTypeUAT,
			AppID: config.AppID,
		},
		IncludeScopes: true,
	})
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return err
		}
		return errs.NewInternalError(errs.SubtypeUnknown,
			"failed to inspect user authorization: %v", err).
			WithCause(err)
	}
	if inspection == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"credential source returned no authorization inspection")
	}

	if inspection.Status == credential.TokenInspectionNotLoggedIn && !inspection.Source.Managed {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"ok": false, "error": "not_logged_in", "missing": required})
		return output.ErrBare(1)
	}
	if !inspection.Present {
		if inspection.Source.Managed {
			return errs.NewAuthenticationError(errs.SubtypeTokenMissing,
				"credential source %q did not provide a user access token", inspection.Source.Name).
				WithHint("authorize the user through the selected credential source")
		}
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"ok": false, "error": "no_token", "missing": required})
		return output.ErrBare(1)
	}

	switch inspection.ScopeState {
	case credential.ScopeUnsupported:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"auth check is unsupported by credential source %q because granted scopes are unavailable", inspection.Source.Name).
			WithHint("the credential source must expose trusted scope metadata before `auth check` can evaluate --scope")
	case credential.ScopeUnknown:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"auth check result is unknown because credential source %q returned no scope metadata", inspection.Source.Name).
			WithHint("configure the credential source to return trusted scopes for user access tokens")
	case credential.ScopeKnown:
		// Continue below.
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"credential source %q returned invalid scope inspection state %q", inspection.Source.Name, inspection.ScopeState)
	}

	suggestion := ""
	missing := larkauth.MissingScopes(inspection.Scopes, required)
	switch {
	case len(missing) == 0:
		// Nothing to recover from.
	case inspection.Source.Managed:
		// A managed source owns authorization, so local `auth login` cannot fix it.
		suggestion = fmt.Sprintf("grant these scopes through credential source %s: %s", inspection.Source.Name, strings.Join(missing, " "))
	case projector.CanReference(recovery.TargetAuthLogin):
		suggestion = projector.RenderHint(recovery.UserAuthorization(missing...))
	}
	return writeAuthCheckResult(f, required, inspection.Scopes, suggestion)
}

func writeAuthCheckResult(f *cmdutil.Factory, required []string, availableScopes, suggestion string) error {
	missing := larkauth.MissingScopes(availableScopes, required)
	missingSet := make(map[string]bool, len(missing))
	for _, s := range missing {
		missingSet[s] = true
	}
	var granted []string
	for _, s := range required {
		if !missingSet[s] {
			granted = append(granted, s)
		}
	}

	ok := len(missing) == 0
	result := map[string]interface{}{"ok": ok, "granted": granted, "missing": missing}
	if len(missing) > 0 && suggestion != "" {
		result["suggestion"] = suggestion
	}
	output.PrintJson(f.IOStreams.Out, result)
	if !ok {
		return output.ErrBare(1)
	}
	return nil
}
