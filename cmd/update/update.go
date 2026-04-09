// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/update"
)

type installMethod int

const (
	installNpm installMethod = iota
	installManual
)

const (
	npmPackage   = "@larksuite/cli"
	releasesURL  = "https://github.com/larksuite/cli/releases/latest"
	maxNpmOutput = 2000
)

// Overridable function vars for testing.
var (
	fetchLatest    = func() (string, error) { return update.FetchLatest() }
	currentVersion = func() string { return build.Version }
	detectMethod   = detectInstallMethodAuto
	runNpmInstall  = runNpmInstallReal
)

// UpdateOptions holds inputs for the update command.
type UpdateOptions struct {
	Factory *cmdutil.Factory
	JSON    bool
	Force   bool
}

// NewCmdUpdate creates the update command.
func NewCmdUpdate(f *cmdutil.Factory) *cobra.Command {
	opts := &UpdateOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update lark-cli to the latest version",
		Long: `Update lark-cli to the latest version.

Detects the installation method automatically:
  - npm install: runs npm install -g @larksuite/cli@<version>
  - manual/other: shows GitHub Releases download URL

Use --json for structured output (for AI agents and scripts).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force reinstall even if already up to date")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
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

	// 4. Detect installation method and upgrade
	return doUpdate(opts, cur, latest)
}

// detectInstallMethod checks if the resolved executable path indicates npm installation.
func detectInstallMethod(resolvedPath string) installMethod {
	if strings.Contains(resolvedPath, "node_modules") {
		return installNpm
	}
	return installManual
}

// detectInstallMethodAuto detects the install method from the running executable.
func detectInstallMethodAuto() (installMethod, string) {
	exe, err := os.Executable()
	if err != nil {
		return installManual, ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return installManual, exe
	}
	return detectInstallMethod(resolved), resolved
}

// runNpmInstallReal executes npm install -g @larksuite/cli@<version>.
func runNpmInstallReal(version string, stdout, stderr *bytes.Buffer) error {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found in PATH: %w", err)
	}
	cmd := exec.Command(npmPath, "install", "-g", npmPackage+"@"+version)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// truncate returns the last maxLen bytes of s.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// doUpdate detects installation method and dispatches to the appropriate upgrade path.
func doUpdate(opts *UpdateOptions, cur, latest string) error {
	method, resolvedPath := detectMethod()

	if method == installManual {
		if opts.JSON {
			return doManualUpdateJSON(opts, cur, latest, resolvedPath)
		}
		return doManualUpdateHuman(opts, cur, latest, resolvedPath)
	}

	if opts.JSON {
		return doNpmUpdateJSON(opts, cur, latest)
	}
	return doNpmUpdateHuman(opts, cur, latest)
}

func doManualUpdateJSON(opts *UpdateOptions, cur, latest, resolvedPath string) error {
	io := opts.Factory.IOStreams
	output.PrintJson(io.Out, map[string]interface{}{
		"ok":               true,
		"previous_version": cur,
		"latest_version":   latest,
		"action":           "manual_required",
		"message":          fmt.Sprintf("lark-cli was not installed via npm (path: %s). Download the latest release from GitHub.", resolvedPath),
		"url":              releasesURL,
	})
	return nil
}

func doManualUpdateHuman(opts *UpdateOptions, cur, latest, resolvedPath string) error {
	io := opts.Factory.IOStreams
	fmt.Fprintf(io.ErrOut, "lark-cli was not installed via npm (path: %s).\n", resolvedPath)
	fmt.Fprintf(io.ErrOut, "Automatic update is only supported for npm installations.\n\n")
	fmt.Fprintf(io.ErrOut, "To upgrade manually, download the latest release:\n")
	fmt.Fprintf(io.ErrOut, "  %s\n", releasesURL)
	return nil
}

func doNpmUpdateJSON(opts *UpdateOptions, cur, latest string) error {
	io := opts.Factory.IOStreams
	var stdoutBuf, stderrBuf bytes.Buffer

	if err := runNpmInstall(latest, &stdoutBuf, &stderrBuf); err != nil {
		combined := stdoutBuf.String() + stderrBuf.String()
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": false,
			"error": map[string]interface{}{
				"type":    "upgrade_error",
				"message": fmt.Sprintf("npm install failed: %s", err),
				"detail":  truncate(combined, maxNpmOutput),
				"hint":    suggestSudo(combined),
			},
		})
		return output.ErrBare(1)
	}

	// Suppress the update-available notice entirely. Simply clearing
	// update.SetPending(nil) is racy — the background goroutine in
	// setupUpdateNotice() may re-set it between our clear and PrintJson's
	// read. Niling the function pointer is safe: PendingNotice is only read
	// from this goroutine (inside PrintJson → injectNotice).
	output.PendingNotice = nil

	output.PrintJson(io.Out, map[string]interface{}{
		"ok":               true,
		"previous_version": cur,
		"current_version":  latest,
		"latest_version":   latest,
		"action":           "updated",
		"message":          fmt.Sprintf("lark-cli updated from %s to %s", cur, latest),
	})
	return nil
}

func doNpmUpdateHuman(opts *UpdateOptions, cur, latest string) error {
	ios := opts.Factory.IOStreams
	fmt.Fprintf(ios.ErrOut, "Updating lark-cli %s → %s via npm ...\n", cur, latest)

	var stdoutBuf, stderrBuf bytes.Buffer

	if err := runNpmInstall(latest, &stdoutBuf, &stderrBuf); err != nil {
		combined := stdoutBuf.String() + stderrBuf.String()
		// Write captured output to terminal for diagnosis.
		if stdoutBuf.Len() > 0 {
			fmt.Fprint(ios.ErrOut, stdoutBuf.String())
		}
		if stderrBuf.Len() > 0 {
			fmt.Fprint(ios.ErrOut, stderrBuf.String())
		}
		fmt.Fprintf(ios.ErrOut, "\n✗ Update failed: %s\n", err)
		if hint := suggestSudo(combined); hint != "" {
			fmt.Fprintf(ios.ErrOut, "  %s\n", hint)
		}
		return output.ErrBare(1)
	}

	output.PendingNotice = nil
	fmt.Fprintf(ios.ErrOut, "\n✓ Successfully updated lark-cli from %s to %s\n", cur, latest)
	return nil
}

func suggestSudo(output string) string {
	if strings.Contains(output, "EACCES") && runtime.GOOS != "windows" {
		return "Hint: try running with sudo, e.g. sudo npm install -g " + npmPackage
	}
	return ""
}
