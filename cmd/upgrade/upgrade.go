// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package upgrade

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/update"
)

// Overridable function vars for testing.
var (
	fetchLatest    = func() (string, error) { return update.FetchLatest() }
	currentVersion = func() string { return build.Version }
)

// UpgradeOptions holds inputs for the upgrade command.
type UpgradeOptions struct {
	Factory *cmdutil.Factory
	JSON    bool
	Force   bool
}

// NewCmdUpgrade creates the upgrade command.
func NewCmdUpgrade(f *cmdutil.Factory) *cobra.Command {
	opts := &UpgradeOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade lark-cli to the latest version",
		Long: `Upgrade lark-cli to the latest version.

Detects the installation method automatically:
  - npm install: runs npm install -g @larksuite/cli@<version>
  - manual/other: shows GitHub Releases download URL

Use --json for structured output (for AI agents and scripts).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return upgradeRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force reinstall even if already up to date")

	return cmd
}

func upgradeRun(opts *UpgradeOptions) error {
	io := opts.Factory.IOStreams
	cur := currentVersion()

	// 1. Fetch latest version
	latest, err := fetchLatest()
	if err != nil {
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok": false,
				"error": map[string]interface{}{
					"type":    "network",
					"message": fmt.Sprintf("failed to check latest version: %s", err),
				},
			})
			return output.ErrBare(1)
		}
		return output.ErrNetwork("failed to check latest version: %s", err)
	}

	// 2. Validate version format (guard against tampered registry responses)
	if update.ParseVersion(latest) == nil {
		msg := fmt.Sprintf("invalid version from registry: %s", latest)
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "upgrade_error", "message": msg},
			})
			return output.ErrBare(1)
		}
		return output.Errorf(output.ExitInternal, "upgrade_error", "%s", msg)
	}

	// 3. Compare versions
	if !opts.Force && !update.IsNewer(latest, cur) {
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":               true,
				"previous_version": cur,
				"current_version":  cur,
				"latest_version":   latest,
				"action":           "already_up_to_date",
				"message":          fmt.Sprintf("lark-cli %s is already up to date", cur),
			})
			return nil
		}
		fmt.Fprintf(io.ErrOut, "✓ lark-cli %s is already up to date\n", cur)
		return nil
	}

	// 4. Detect installation method and upgrade (placeholder for Task 2)
	return doUpgrade(opts, cur, latest)
}

// doUpgrade performs the actual upgrade. Placeholder until Task 2.
func doUpgrade(opts *UpgradeOptions, cur, latest string) error {
	return output.Errorf(output.ExitInternal, "upgrade_error", "upgrade not implemented yet")
}
