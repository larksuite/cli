// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs"
)

// NewCmdProfileBind creates the profile bind subcommand.
func NewCmdProfileBind(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bind <name>",
		Short: "Bind the current project to a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileBindRun(f, args[0])
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

// NewCmdProfileUnbind creates the profile unbind subcommand.
func NewCmdProfileUnbind(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unbind",
		Short: "Remove the current project's profile binding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileUnbindRun(f)
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func profileBindRun(f *cmdutil.Factory, name string) error {
	name = strings.TrimSpace(name)
	if err := core.ValidateProfileName(name); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("name").WithCause(err)
	}
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}
	app := multi.FindApp(name)
	if app == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "profile %q not found", name).
			WithHint("available profiles: %s", formatProfileNameList(multi.ProfileNames())).
			WithParam("name")
	}
	cwd, err := vfs.Getwd()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot determine working directory: %v", err).WithCause(err)
	}
	path, err := core.ProjectConfigWritePath(cwd)
	if err != nil {
		return err
	}
	if err := core.SaveProjectConfig(path, app.ProfileName()); err != nil {
		return err
	}
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Project profile bound to %q", app.ProfileName()))
	fmt.Fprintf(f.IOStreams.ErrOut, "Config: %s\n", path)
	return nil
}

func profileUnbindRun(f *cmdutil.Factory) error {
	cwd, err := vfs.Getwd()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot determine working directory: %v", err).WithCause(err)
	}
	path, ok, err := core.FindProjectConfigPath(cwd)
	if err != nil {
		return err
	}
	if !ok {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "no project profile binding found").
			WithHint("run: lark-cli profile bind <name>")
	}
	fileRemoved, profileRemoved, err := core.RemoveProjectProfile(path)
	if err != nil {
		return err
	}
	if !profileRemoved {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "no project profile binding found").
			WithHint("run: lark-cli profile bind <name>")
	}
	output.PrintSuccess(f.IOStreams.ErrOut, "Project profile binding removed")
	if fileRemoved {
		fmt.Fprintf(f.IOStreams.ErrOut, "Removed: %s\n", path)
	} else {
		fmt.Fprintf(f.IOStreams.ErrOut, "Updated: %s\n", path)
	}
	return nil
}

func formatProfileNameList(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}
