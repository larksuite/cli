// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package info

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// InfoOptions holds inputs for the info command.
type InfoOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdInfo creates the info command.
func NewCmdInfo(f *cmdutil.Factory) *cobra.Command {
	opts := &InfoOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show CLI version and environment info",
		RunE: func(cmd *cobra.Command, args []string) error {
			return infoRun(opts)
		},
	}

	cmdutil.DisableAuthCheck(cmd)
	return cmd
}

func infoRun(opts *InfoOptions) error {
	output.PrintJson(opts.Factory.IOStreams.Out, map[string]interface{}{
		"ok":      true,
		"version": build.Version,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"go":      runtime.Version(),
	})
	return nil
}
