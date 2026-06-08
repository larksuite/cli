// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// UsersUseOptions holds inputs for `auth users use`.
type UsersUseOptions struct {
	Factory *cmdutil.Factory
	Target  string // open_id or user_name
}

// NewCmdAuthUsersUse creates the `auth users use <id|name>` subcommand.
// Resolution matches open_id first, then user_name.
func NewCmdAuthUsersUse(f *cmdutil.Factory, runF func(*UsersUseOptions) error) *cobra.Command {
	opts := &UsersUseOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "use <open_id|user_name>",
		Short: "Switch the active user within the current profile",
		Long: `use sets the active user for the current profile. Subsequent commands that
do not pass --user resolve through AppConfig.CurrentUser.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			if runF != nil {
				return runF(opts)
			}
			return authUsersUseRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func authUsersUseRun(opts *UsersUseOptions) error {
	f := opts.Factory
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"<open_id|user_name> required").
			WithParam("<open_id|user_name>")
	}

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return core.PassThroughOrNotConfigured(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		return core.NotConfiguredError()
	}
	// Use CurrentAppConfig so --profile=ghost reports cleanly instead of
	// silently falling back to Apps[0] and writing CurrentUser to the wrong
	// profile. Matches users_logout.go / users_list.go resolution policy.
	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		if f.Invocation.Profile != "" {
			return errs.NewConfigError(errs.SubtypeInvalidArgument,
				"profile %q not found", f.Invocation.Profile).
				WithHint("available profiles: %s", strings.Join(multi.ProfileNames(), ", "))
		}
		return errs.NewConfigError(errs.SubtypeInvalidConfig, "no active profile to switch within")
	}

	user := app.FindUser(target)
	if user == nil {
		hint := "available users: "
		if names := app.UserNames(); len(names) > 0 {
			hint += strings.Join(names, ", ")
		} else {
			hint += "(none — run `lark-cli auth login` to add one)"
		}
		return errs.NewConfigError(errs.SubtypeInvalidArgument,
			"user %q not found in profile %q", target, app.ProfileName()).
			WithHint(hint)
	}

	// Share login's flock so concurrent `auth users use` and `auth login`
	// cannot interleave reads/writes of the config file.
	root := loginRoot()
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "users use: acquire flock: %v", err).WithCause(err)
	}
	defer lk.Release()

	// Reload under the flock — state may have changed since the pre-lock load.
	multi, err = core.LoadMultiAppConfig()
	if err != nil {
		return core.PassThroughOrNotConfigured(err)
	}
	idx := multi.FindAppIndex(app.ProfileName())
	if idx < 0 {
		return errs.NewConfigError(errs.SubtypeInvalidConfig,
			"profile %q vanished during users use", app.ProfileName())
	}
	app = &multi.Apps[idx]

	// Re-find under the flock — a concurrent logout could have removed it.
	user = app.FindUser(target)
	if user == nil {
		return errs.NewConfigError(errs.SubtypeInvalidArgument,
			"user %q vanished during users use", target)
	}

	if app.CurrentUser == user.UserOpenId {
		// still report so scripts can rely on stable JSON
		fmt.Fprintf(f.IOStreams.ErrOut, "Already active: %s (%s)\n", user.UserName, user.UserOpenId)
		return nil
	}
	app.CurrentUser = user.UserOpenId

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "save config: %v", err).WithCause(err)
	}

	// Best-effort: bump LastUsed so `users list` reflects the active pick.
	if err := larkauth.RecordUserActivity(root, larkauth.ForUser(app.AppId, user.UserOpenId), nil); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth users use: record activity: %v\n", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Active user: %s (%s)", user.UserName, user.UserOpenId))
	return nil
}
