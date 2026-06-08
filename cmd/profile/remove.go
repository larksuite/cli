// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdProfileRemove creates the profile remove subcommand.
func NewCmdProfileRemove(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileRemoveRun(f, args[0])
		},
	}
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT remove profiles unless the user explicitly asks. This is destructive and clears all associated credentials.",
	})
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func profileRemoveRun(f *cmdutil.Factory, name string) error {
	root := larkauth.NewLocalRoot(core.GetConfigDir())
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "profile remove: acquire flock: %v", err)
	}
	defer lk.Release()

	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}

	idx := multi.FindAppIndex(name)
	if idx < 0 {
		return output.ErrValidation("profile %q not found, available profiles: %s", name, strings.Join(multi.ProfileNames(), ", "))
	}

	if len(multi.Apps) == 1 {
		return output.ErrValidation("cannot remove the only profile")
	}

	app := &multi.Apps[idx]
	removedName := app.ProfileName()
	appId := app.AppId
	appSecret := app.AppSecret
	users := app.Users

	// Remove from slice
	multi.Apps = append(multi.Apps[:idx], multi.Apps[idx+1:]...)

	// Fix currentApp / previousApp references
	if multi.CurrentApp == removedName {
		multi.CurrentApp = multi.Apps[0].ProfileName()
	}
	if multi.PreviousApp == removedName {
		multi.PreviousApp = ""
	}
	// Self-toggle guard: if removing the active profile promoted Apps[0]
	// to CurrentApp and Apps[0] happens to equal PreviousApp, the invariant
	// CurrentApp != PreviousApp breaks. `profile use -` would short-circuit
	// "Already on profile X" — toggling back to where you already are.
	// Three-profile repro: CurrentApp=alpha, PreviousApp=beta, remove alpha
	// → Apps[0]=beta → CurrentApp:=beta (== PreviousApp). Clear PreviousApp
	// to restore the invariant; the next `profile use` round-trip
	// re-establishes a real previous.
	if multi.PreviousApp != "" && multi.PreviousApp == multi.CurrentApp {
		multi.PreviousApp = ""
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	// Best-effort credential cleanup after config commit
	core.RemoveSecretStore(appSecret, f.Keychain)
	for _, user := range users {
		// Triple sweep: keychain UAT + sidecar profile + index row.
		// Profile remove is destructive by user intent; leaving on-disk
		// artifacts in place would let a removed user resurface in
		// `auth users list` and mis-attribute the slot on re-login.
		_ = larkauth.PurgeUserArtifacts(root, appId, user.UserOpenId)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Profile %q removed", removedName))
	return nil
}
