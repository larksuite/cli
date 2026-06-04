// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

type LogoutOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthLogout creates the auth logout subcommand: wipes the whole
// profile. For per-user logout, use `auth users logout <id>`. Shares the
// login flock so a concurrent login cannot interleave.
func NewCmdAuthLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Wipe all users' tokens, sidecar profiles, and index rows in the active profile",
		Long: `logout wipes every user from the active profile:
  - Removes their UAT entries from the OS keychain
  - Clears AppConfig.Users and AppConfig.CurrentUser
  - Deletes every sidecar UserProfile JSON
  - Removes every user index row

For per-user surgical logout, use ` + "`lark-cli auth users logout <open_id|user_name>`" + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authLogoutRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func authLogoutRun(opts *LogoutOptions) error {
	f := opts.Factory

	// Pre-lock peek: short-circuit no-config / not-logged-in before
	// grabbing the flock, so a stale lock can't block status reads.
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		// R2 / parse / permission errors must NOT be silently coerced into
		// "no config" (the legacy behaviour). Pass them through; only the
		// genuine missing-file case takes the idempotent-success branch.
		if !errors.Is(err, os.ErrNotExist) {
			return core.PassThroughOrNotConfigured(err)
		}
		multi = nil
	}
	if multi == nil || len(multi.Apps) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No configuration found.")
		return nil
	}
	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil || len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}
	profileName := app.ProfileName()

	// Shared login flock; SingleUser() is intentional — wipe-all is
	// config-file-scoped, so it locks out everyone for the duration.
	root := loginRoot()
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "auth logout: acquire flock: %v", err).WithCause(err)
	}
	defer lk.Release()

	// Reload under the flock for R-M-W safety. Same R2 transparency rule
	// as the pre-lock peek — surface the structured envelope, don't coerce.
	multi, err = core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
			return nil
		}
		return core.PassThroughOrNotConfigured(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		// Idempotent: lost the row to a concurrent wiper.
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}
	idx := multi.FindAppIndex(profileName)
	if idx < 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}
	app = &multi.Apps[idx]
	if len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}

	// Snapshot victims; we still need their open_ids for the sidecar+index sweep.
	victims := make([]core.AppUser, len(app.Users))
	copy(victims, app.Users)

	// Keychain: warn-not-fatal so a transient hiccup can't desync config +
	// keychain — operator can re-run to mop up. Delete is idempotent on "not found".
	for _, u := range victims {
		if err := larkauth.RemoveStoredToken(app.AppId, u.UserOpenId); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth logout: remove keychain entry %s: %v\n", u.UserOpenId, err)
		}
	}

	// Drop all users in one save so a crash mid-loop can't leave half-removed users on disk.
	app.Users = []core.AppUser{}
	app.CurrentUser = ""
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	// Sidecar profiles + index rows: best-effort, both rebuild on next login.
	// Swept AFTER config save so we never delete on-disk state for a row the
	// save might have failed to drop.
	for _, u := range victims {
		ctx := larkauth.ForUser(app.AppId, u.UserOpenId)
		if err := larkauth.DeleteUserProfileFor(root, ctx); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth logout: delete sidecar profile %s: %v\n", u.UserOpenId, err)
		}
		if err := larkauth.DeleteUser(root, ctx); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth logout: delete index row %s: %v\n", u.UserOpenId, err)
		}
	}

	if len(victims) == 1 {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Logged out %s (%s)", victims[0].UserName, victims[0].UserOpenId))
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Logged out %d users from profile %q", len(victims), profileName))
	}
	return nil
}
