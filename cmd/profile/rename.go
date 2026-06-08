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

// NewCmdProfileRename creates the profile rename subcommand.
func NewCmdProfileRename(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileRenameRun(f, args[0], args[1])
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func profileRenameRun(f *cmdutil.Factory, oldName, newName string) error {
	if err := core.ValidateProfileName(newName); err != nil {
		return output.ErrValidation("%v", err)
	}

	root := larkauth.NewLocalRoot(core.GetConfigDir())
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "profile rename: acquire flock: %v", err)
	}
	defer lk.Release()

	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}

	idx := multi.FindAppIndex(oldName)
	if idx < 0 {
		return output.ErrValidation("profile %q not found, available profiles: %s", oldName, strings.Join(multi.ProfileNames(), ", "))
	}

	// Check new name uniqueness across other profiles, allowing renames to this
	// profile's own appId or current name.
	for i := range multi.Apps {
		if i == idx {
			continue
		}
		if multi.Apps[i].Name == newName || multi.Apps[i].AppId == newName {
			return output.ErrValidation("profile %q already exists", newName)
		}
	}

	oldProfileName := multi.Apps[idx].ProfileName()
	multi.Apps[idx].Name = newName

	// Update currentApp / previousApp references
	if multi.CurrentApp == oldProfileName {
		multi.CurrentApp = newName
	}
	if multi.PreviousApp == oldProfileName {
		multi.PreviousApp = newName
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Profile renamed: %q -> %q", oldProfileName, newName))
	return nil
}
