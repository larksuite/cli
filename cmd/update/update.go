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
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/update"
)

type installMethod int

const (
	installNpm installMethod = iota
	installManual
)

const (
	npmPackage   = "@larksuite/cli"
	repoURL      = "https://github.com/larksuite/cli"
	maxNpmOutput = 2000
	osWindows    = "windows"
)

// Overridable function vars for testing.
var (
	fetchLatest     = func() (string, error) { return update.FetchLatest() }
	currentVersion  = func() string { return build.Version }
	detectMethod    = detectInstallMethodAuto
	runNpmInstall   = runNpmInstallReal
	runSkillsUpdate = runSkillsUpdateReal
	lookPath        = exec.LookPath
	currentOS       = runtime.GOOS
)

func isWindows() bool { return currentOS == osWindows }

// releaseURL returns a version-pinned GitHub Releases URL.
func releaseURL(version string) string {
	return repoURL + "/releases/tag/v" + strings.TrimPrefix(version, "v")
}

// changelogURL returns the project CHANGELOG URL.
func changelogURL() string {
	return repoURL + "/blob/main/CHANGELOG.md"
}

// --- Terminal symbols ---
// Use ASCII fallbacks on Windows to avoid mojibake in legacy CMD/PowerShell 5.

func symOK() string {
	if isWindows() {
		return "[OK]"
	}
	return "✓"
}

func symFail() string {
	if isWindows() {
		return "[FAIL]"
	}
	return "✗"
}

func symWarn() string {
	if isWindows() {
		return "[WARN]"
	}
	return "⚠"
}

func symArrow() string {
	if isWindows() {
		return "->"
	}
	return "→"
}

// --- Command ---

// UpdateOptions holds inputs for the update command.
type UpdateOptions struct {
	Factory  *cmdutil.Factory
	JSON     bool
	Force    bool
	Check    bool
	Rollback bool
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

Use --json for structured output (for AI agents and scripts).
Use --check to only check for updates without installing.
Use --rollback to restore the previous version from backup.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force reinstall even if already up to date")
	cmd.Flags().BoolVar(&opts.Check, "check", false, "only check for updates, do not install")
	cmd.Flags().BoolVar(&opts.Rollback, "rollback", false, "restore the previous version from backup")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	io := opts.Factory.IOStreams
	cur := currentVersion()

	// Clean up stale .old files from previous Windows upgrades.
	selfupdate.CleanupStaleFiles()

	// Handle --rollback.
	if opts.Rollback {
		return runRollback(opts)
	}

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
			return output.ErrBare(output.ExitNetwork)
		}
		return output.ErrNetwork("failed to check latest version: %s", err)
	}

	// 2. Validate version format (guard against tampered registry responses)
	if update.ParseVersion(latest) == nil {
		msg := fmt.Sprintf("invalid version from registry: %s", latest)
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "update_error", "message": msg},
			})
			return output.ErrBare(output.ExitInternal)
		}
		return output.Errorf(output.ExitInternal, "update_error", "%s", msg)
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
		fmt.Fprintf(io.ErrOut, "%s lark-cli %s is already up to date\n", symOK(), cur)
		return nil
	}

	// 4. Detect installation method
	method, resolvedPath := detectMethod()

	npmAvailable := true
	if method == installNpm {
		if _, err := lookPath("npm"); err != nil {
			npmAvailable = false
		}
	}
	canAutoUpdate := method == installNpm && npmAvailable

	// 5. --check: report availability without installing
	if opts.Check {
		return reportCheckResult(opts, io, cur, latest, canAutoUpdate)
	}

	// 6. Execute update
	if !canAutoUpdate {
		return doManualUpdate(opts, cur, latest, resolvedPath)
	}
	if opts.JSON {
		return doNpmUpdateJSON(opts, cur, latest)
	}
	return doNpmUpdateHuman(opts, cur, latest)
}

// --- Check result ---

func reportCheckResult(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, canAutoUpdate bool) error {
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok":               true,
			"previous_version": cur,
			"current_version":  cur,
			"latest_version":   latest,
			"action":           "update_available",
			"auto_update":      canAutoUpdate,
			"message":          fmt.Sprintf("lark-cli %s %s %s available", cur, symArrow(), latest),
			"url":              releaseURL(latest),
			"changelog":        changelogURL(),
		})
		return nil
	}
	fmt.Fprintf(io.ErrOut, "Update available: %s %s %s\n", cur, symArrow(), latest)
	fmt.Fprintf(io.ErrOut, "  Release:   %s\n", releaseURL(latest))
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	if canAutoUpdate {
		fmt.Fprintf(io.ErrOut, "\nRun `lark-cli update` to install.\n")
	} else {
		fmt.Fprintf(io.ErrOut, "\nDownload the release above to update manually.\n")
	}
	return nil
}

// --- Installation detection ---

func detectInstallMethod(resolvedPath string) installMethod {
	if strings.Contains(resolvedPath, "node_modules") {
		return installNpm
	}
	return installManual
}

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

// --- npm execution ---

func runNpmInstallReal(version string, stdout, stderr *bytes.Buffer) error {
	npmPath, err := lookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found in PATH: %w", err)
	}
	cmd := exec.Command(npmPath, "install", "-g", npmPackage+"@"+version)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func runSkillsUpdateReal(stdout, stderr *bytes.Buffer) error {
	npxPath, err := lookPath("npx")
	if err != nil {
		return fmt.Errorf("npx not found in PATH: %w", err)
	}
	cmd := exec.Command(npxPath, "skills", "add", "larksuite/cli", "-g", "-y")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// --- Update dispatch ---

func manualReason(method installMethod, npmAvailable bool) string {
	if method == installNpm && !npmAvailable {
		return "installed via npm, but npm is not available in PATH"
	}
	return "not installed via npm"
}

func doManualUpdate(opts *UpdateOptions, cur, latest string, resolvedPath string) error {
	io := opts.Factory.IOStreams
	reason := manualReason(installManual, true)
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok":               true,
			"previous_version": cur,
			"latest_version":   latest,
			"action":           "manual_required",
			"message":          fmt.Sprintf("Automatic update unavailable: %s (path: %s)", reason, resolvedPath),
			"url":              releaseURL(latest),
			"changelog":        changelogURL(),
		})
		return nil
	}
	fmt.Fprintf(io.ErrOut, "Automatic update unavailable: %s (path: %s).\n\n", reason, resolvedPath)
	fmt.Fprintf(io.ErrOut, "To update manually, download the latest release:\n")
	fmt.Fprintf(io.ErrOut, "  Release:   %s\n", releaseURL(latest))
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	fmt.Fprintf(io.ErrOut, "\nOr install via npm:\n  npm install -g %s@%s\n", npmPackage, latest)
	fmt.Fprintf(io.ErrOut, "\nAfter updating, also update skills:\n  npx skills add larksuite/cli -g -y\n")
	return nil
}

func doNpmUpdateJSON(opts *UpdateOptions, cur, latest string) error {
	io := opts.Factory.IOStreams

	// Backup current binary before upgrading.
	if _, err := selfupdate.Backup(cur); err != nil {
		// Non-fatal: warn but proceed.
		_ = err
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	if err := runNpmInstall(latest, &stdoutBuf, &stderrBuf); err != nil {
		combined := stdoutBuf.String() + stderrBuf.String()
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": false,
			"error": map[string]interface{}{
				"type":    "update_error",
				"message": fmt.Sprintf("npm install failed: %s", err),
				"detail":  truncate(combined, maxNpmOutput),
				"hint":    permissionHint(combined),
			},
		})
		return output.ErrBare(output.ExitAPI)
	}

	// Suppress stale update-available notice. Niling PendingNotice is safe:
	// it's only read from this goroutine (inside PrintJson -> injectNotice).
	output.PendingNotice = nil

	// Update skills (best-effort).
	var skillsStdout, skillsStderr bytes.Buffer
	skillsErr := runSkillsUpdate(&skillsStdout, &skillsStderr)

	result := map[string]interface{}{
		"ok":               true,
		"previous_version": cur,
		"current_version":  latest,
		"latest_version":   latest,
		"action":           "updated",
		"message":          fmt.Sprintf("lark-cli updated from %s to %s", cur, latest),
		"url":              releaseURL(latest),
		"changelog":        changelogURL(),
	}
	if skillsErr != nil {
		result["skills_warning"] = fmt.Sprintf("skills update failed: %s", skillsErr)
		if detail := strings.TrimSpace(skillsStderr.String()); detail != "" {
			result["skills_detail"] = truncate(detail, maxNpmOutput)
		}
	}
	output.PrintJson(io.Out, result)
	return nil
}

func doNpmUpdateHuman(opts *UpdateOptions, cur, latest string) error {
	ios := opts.Factory.IOStreams

	// Backup current binary before upgrading.
	if bk, err := selfupdate.Backup(cur); err == nil {
		fmt.Fprintf(ios.ErrOut, "Backed up current version to %s\n", bk.Path)
	}

	fmt.Fprintf(ios.ErrOut, "Updating lark-cli %s %s %s via npm ...\n", cur, symArrow(), latest)

	var stdoutBuf, stderrBuf bytes.Buffer

	if err := runNpmInstall(latest, &stdoutBuf, &stderrBuf); err != nil {
		combined := stdoutBuf.String() + stderrBuf.String()
		if stdoutBuf.Len() > 0 {
			fmt.Fprint(ios.ErrOut, stdoutBuf.String())
		}
		if stderrBuf.Len() > 0 {
			fmt.Fprint(ios.ErrOut, stderrBuf.String())
		}
		fmt.Fprintf(ios.ErrOut, "\n%s Update failed: %s\n", symFail(), err)
		if hint := permissionHint(combined); hint != "" {
			fmt.Fprintf(ios.ErrOut, "  %s\n", hint)
		}
		return output.ErrBare(1)
	}

	output.PendingNotice = nil
	fmt.Fprintf(ios.ErrOut, "\n%s Successfully updated lark-cli from %s to %s\n", symOK(), cur, latest)
	fmt.Fprintf(ios.ErrOut, "  Changelog: %s\n", changelogURL())

	// Update skills (best-effort).
	fmt.Fprintf(ios.ErrOut, "\nUpdating skills ...\n")
	var skillsStdout, skillsStderr bytes.Buffer
	if err := runSkillsUpdate(&skillsStdout, &skillsStderr); err != nil {
		fmt.Fprintf(ios.ErrOut, "%s Skills update failed: %s\n", symWarn(), err)
		if detail := strings.TrimSpace(skillsStderr.String()); detail != "" {
			fmt.Fprintf(ios.ErrOut, "  %s\n", truncate(detail, 500))
		}
		fmt.Fprintf(ios.ErrOut, "  Run manually: npx skills add larksuite/cli -g -y\n")
	} else {
		fmt.Fprintf(ios.ErrOut, "%s Skills updated\n", symOK())
	}
	return nil
}

// --- Rollback ---

func runRollback(opts *UpdateOptions) error {
	io := opts.Factory.IOStreams

	backups, err := selfupdate.ListBackups()
	if err != nil {
		msg := fmt.Sprintf("failed to list backups: %s", err)
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "rollback_error", "message": msg},
			})
			return output.ErrBare(output.ExitAPI)
		}
		return output.Errorf(output.ExitAPI, "rollback_error", "%s", msg)
	}

	if len(backups) == 0 {
		msg := "no backups available for rollback"
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "rollback_error", "message": msg},
			})
			return output.ErrBare(output.ExitAPI)
		}
		return output.Errorf(output.ExitAPI, "rollback_error", "%s", msg)
	}

	target := backups[0]
	info, err := selfupdate.RollbackTo(target)
	if err != nil {
		msg := fmt.Sprintf("rollback failed: %s", err)
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "rollback_error", "message": msg},
			})
			return output.ErrBare(output.ExitAPI)
		}
		return output.Errorf(output.ExitAPI, "rollback_error", "%s", msg)
	}

	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok":      true,
			"action":  "rolled_back",
			"version": info.Version,
			"message": fmt.Sprintf("rolled back to %s", info.Version),
		})
		return nil
	}
	fmt.Fprintf(io.ErrOut, "%s Rolled back to version %s\n", symOK(), info.Version)
	return nil
}

func permissionHint(npmOutput string) string {
	if strings.Contains(npmOutput, "EACCES") && !isWindows() {
		return "Permission denied. Try: sudo lark-cli update, or adjust your npm global prefix: https://docs.npmjs.com/resolving-eacces-permissions-errors"
	}
	return ""
}
