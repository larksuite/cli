// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package version

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

type options struct {
	factory *cmdutil.Factory
	json    bool
}

type versionReport struct {
	Version      string   `json:"version"`
	Edition      string   `json:"edition"`
	Capabilities []string `json:"capabilities"`
}

// NewCmdVersion reports the immutable edition identity compiled into the
// binary. Root --version remains unchanged for compatibility.
func NewCmdVersion(f *cmdutil.Factory) *cobra.Command {
	opts := &options{factory: f}
	cmd := &cobra.Command{
		Use:    "version",
		Short:  "Show version and edition information",
		Hidden: hideVersionCommand(),
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.json {
				output.PrintJson(opts.factory.IOStreams.Out, versionReport{
					Version:      build.Version,
					Edition:      build.Edition,
					Capabilities: build.Capabilities(),
				})
				return nil
			}
			_, err := fmt.Fprintf(opts.factory.IOStreams.Out, "lark-cli version %s (%s)\n", build.Version, build.Edition)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeSDKError, "failed to write version output: %v", err).WithCause(err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.json, "json", false, "structured JSON output")
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetRisk(cmd, "read")
	return cmd
}
