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

type UsersLogoutOptions struct {
	Factory *cmdutil.Factory
	Target  string // open_id or user_name
}

// NewCmdAuthUsersLogout wipes one user's tokens, AppUser row, sidecar
// profile, and index entry. If the user was active, CurrentUser is
// cleared rather than auto-switched.
func NewCmdAuthUsersLogout(f *cmdutil.Factory, runF func(*UsersLogoutOptions) error) *cobra.Command {
	opts := &UsersLogoutOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "logout <open_id|user_name>",
		Short: "Wipe one user's tokens, sidecar profile, and index row",
		Long: `logout removes a single user from the active profile:
  - Removes their UAT entry from the OS keychain
  - Removes their AppUser row from config.json
  - Deletes their sidecar UserProfile JSON
  - Removes their user index entry

If the user was the active CurrentUser for the profile, CurrentUser
is cleared. The next `+"`auth login`"+` or `+"`auth users use`"+` re-stamps it.

For "log out the entire profile" semantics see `+"`lark-cli auth logout`"+`.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			if runF != nil {
				return runF(opts)
			}
			return authUsersLogoutRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func authUsersLogoutRun(opts *UsersLogoutOptions) error {
	f := opts.Factory
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"<open_id|user_name> required").
			WithParam("<open_id|user_name>")
	}

	root := loginRoot()
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "users logout: acquire flock: %v", err).WithCause(err)
	}
	defer lk.Release()

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return core.PassThroughOrNotConfigured(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		return core.NotConfiguredError()
	}
	// Use CurrentAppConfig so --profile=ghost reports cleanly instead of
	// silently falling back to Apps[0] and deleting a user from the wrong
	// profile. Matches logout.go / users_list.go resolution policy.
	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		if f.Invocation.Profile != "" {
			// Operator typed a non-existent profile: this is an argument
			// problem, not a "config doesn't exist" problem. SubtypeNotConfigured
			// would route AI agents to `config init`, which would clobber the
			// working profiles. Use SubtypeInvalidArgument to match the
			// users_use.go contract.
			return errs.NewConfigError(errs.SubtypeInvalidArgument,
				"profile %q not found", f.Invocation.Profile).
				WithHint("available profiles: %s", strings.Join(multi.ProfileNames(), ", "))
		}
		// Config has Apps but no resolvable active — the file is in a
		// half-set state, not "no config". SubtypeInvalidConfig points the
		// operator at `profile use <name>` rather than re-init.
		return errs.NewConfigError(errs.SubtypeInvalidConfig, "no active profile to log out within")
	}

	uIdx := app.FindUserIndex(target)
	if uIdx < 0 {
		hint := "available users: "
		if names := app.UserNames(); len(names) > 0 {
			hint += strings.Join(names, ", ")
		} else {
			hint += "(none)"
		}
		return errs.NewConfigError(errs.SubtypeInvalidArgument,
			"user %q not found in profile %q", target, app.ProfileName()).
			WithHint(hint)
	}
	victim := app.Users[uIdx]
	// Capture the active-user signal BEFORE we mutate Users[] / clear
	// CurrentUser. This drives the post-save warning: when the victim was
	// the active user AND other users remain, the *next* command silently
	// dispatches as the new Users[0] via the empty-CurrentUser fallback.
	// Without the warning, the operator cannot tell their effective
	// identity changed — a stealth foot-gun.
	victimWasActive := app.CurrentUser == victim.UserOpenId

	// warn-not-fatal: a keychain hiccup must not leave config + keychain desynced.
	if err := larkauth.RemoveStoredToken(app.AppId, victim.UserOpenId); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] users logout: remove keychain entry: %v\n", err)
	}

	// Clear CurrentUser rather than auto-switching: silently changing the active
	// user during a removal would surprise the operator. (The empty-CurrentUser
	// → Users[0] fallback in ResolveConfigFromMulti still applies on the next
	// command — see the post-save warning below for the operator-facing nudge.)
	app.Users = append(app.Users[:uIdx], app.Users[uIdx+1:]...)
	if app.CurrentUser == victim.UserOpenId {
		app.CurrentUser = ""
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "save config: %v", err).WithCause(err)
	}

	// Best-effort: sidecar and index rebuild on next login.
	ctx := larkauth.ForUser(app.AppId, victim.UserOpenId)
	if err := larkauth.DeleteUserProfileFor(root, ctx); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] users logout: delete sidecar profile: %v\n", err)
	}
	if err := larkauth.DeleteUser(root, ctx); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] users logout: delete index row: %v\n", err)
	}

	// Active-user safety nudge. Cleared CurrentUser + non-empty Users[] means
	// the next resolve picks the new Users[0]. Tell the operator BEFORE they
	// run a command and discover the silent identity shift after the fact.
	// We deliberately don't pre-pick CurrentUser ourselves: an explicit
	// `auth users use` keeps the choice in the operator's hands.
	if victimWasActive && len(app.Users) > 0 {
		next := app.Users[0]
		fmt.Fprintf(f.IOStreams.ErrOut,
			"[lark-cli] [WARN] users logout: %s (%s) was the active user; the next command will dispatch as %s (%s) via the Users[0] fallback. Run `lark-cli auth users use <open_id|user_name>` to choose explicitly.\n",
			victim.UserName, victim.UserOpenId, next.UserName, next.UserOpenId)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Logged out: %s (%s)", victim.UserName, victim.UserOpenId))
	return nil
}
