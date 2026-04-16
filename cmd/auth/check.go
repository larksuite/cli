// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// CheckOptions holds all inputs for auth check.
type CheckOptions struct {
	Factory *cmdutil.Factory
	Scope   string
}

// NewCmdAuthCheck creates the auth check subcommand.
func NewCmdAuthCheck(f *cmdutil.Factory, runF func(*CheckOptions) error) *cobra.Command {
	opts := &CheckOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check if current token has specified scopes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authCheckRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to check (space-separated)")
	cmd.MarkFlagRequired("scope")

	return cmd
}

func authCheckRun(opts *CheckOptions) error {
	f := opts.Factory

	required := strings.Fields(opts.Scope)
	if len(required) == 0 {
		return output.ErrValidation("--scope cannot be empty")
	}

	config, err := f.Config()
	if err != nil {
		return err
	}
	if config.UserOpenId == "" {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"ok": false, "error": "not_logged_in", "missing": required})
		return output.ErrBare(1)
	}

	stored := larkauth.GetStoredToken(config.AppID, config.UserOpenId)
	if stored == nil {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{"ok": false, "error": "no_token", "missing": required})
		return output.ErrBare(1)
	}

	missing := larkauth.MissingScopes(stored.Scope, required)
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
	if len(missing) > 0 {
		result["suggestion"] = fmt.Sprintf(`lark-cli auth login --scope "%s"`, strings.Join(missing, " "))
	}
	output.PrintJson(f.IOStreams.Out, result)
	if !ok {
		return output.ErrBare(1)
	}
	return nil
}
