// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/update"
)

const (
	repoURL      = "https://github.com/larksuite/cli"
	maxNpmOutput = 2000
	osWindows    = "windows"
)

// Overridable for testing.
var (
	fetchLatest    = func() (string, error) { return update.FetchLatest() }
	currentVersion = func() string { return build.Version }
	currentOS      = runtime.GOOS
	newUpdater     = func() *selfupdate.Updater { return selfupdate.New() }
)

func isWindows() bool { return currentOS == osWindows }

func releaseURL(version string) string {
	return repoURL + "/releases/tag/v" + strings.TrimPrefix(version, "v")
}

func changelogURL() string { return repoURL + "/blob/main/CHANGELOG.md" }

// --- Terminal symbols (ASCII fallback on Windows) ---

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
	Factory *cmdutil.Factory
	JSON    bool
	Force   bool
	Check   bool
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
Use --check to only check for updates without installing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force reinstall even if already up to date")
	cmd.Flags().BoolVar(&opts.Check, "check", false, "only check for updates, do not install")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	io := opts.Factory.IOStreams
	cur := currentVersion()
	updater := newUpdater()

	updater.CleanupStaleFiles()
	output.PendingNotice = nil

	// 1. Fetch latest version
	latest, err := fetchLatest()
	if err != nil {
		return reportError(opts, io, output.ExitNetwork, "network", "failed to check latest version: %s", err)
	}

	// 2. Validate version format
	if update.ParseVersion(latest) == nil {
		return reportError(opts, io, output.ExitInternal, "update_error", "invalid version from registry: %s", latest)
	}

	// 3. Compare versions
	if !opts.Force && !update.IsNewer(latest, cur) {
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok": true, "previous_version": cur, "current_version": cur,
				"latest_version": latest, "action": "already_up_to_date",
				"message": fmt.Sprintf("lark-cli %s is already up to date", cur),
			})
			return nil
		}
		fmt.Fprintf(io.ErrOut, "%s lark-cli %s is already up to date\n", symOK(), cur)
		return nil
	}

	// 4. Detect installation method
	detect := updater.DetectInstallMethod()

	// 5. --check
	if opts.Check {
		return reportCheckResult(opts, io, cur, latest, detect.CanAutoUpdate())
	}

	// 6. Execute update
	if !detect.CanAutoUpdate() {
		return doManualUpdate(opts, io, cur, latest, detect)
	}
	return doNpmUpdate(opts, io, cur, latest, updater)
}

// --- Output helpers ---

func reportError(opts *UpdateOptions, io *cmdutil.IOStreams, exitCode int, errType, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": false, "error": map[string]interface{}{"type": errType, "message": msg},
		})
		return output.ErrBare(exitCode)
	}
	return output.Errorf(exitCode, errType, "%s", msg)
}

func reportCheckResult(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, canAutoUpdate bool) error {
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": cur,
			"latest_version": latest, "action": "update_available",
			"auto_update": canAutoUpdate,
			"message":     fmt.Sprintf("lark-cli %s %s %s available", cur, symArrow(), latest),
			"url":         releaseURL(latest), "changelog": changelogURL(),
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

func doManualUpdate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, detect selfupdate.DetectResult) error {
	reason := detect.ManualReason()
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": true, "previous_version": cur, "latest_version": latest,
			"action":  "manual_required",
			"message": fmt.Sprintf("Automatic update unavailable: %s (path: %s)", reason, detect.ResolvedPath),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		})
		return nil
	}
	fmt.Fprintf(io.ErrOut, "Automatic update unavailable: %s (path: %s).\n\n", reason, detect.ResolvedPath)
	fmt.Fprintf(io.ErrOut, "To update manually, download the latest release:\n")
	fmt.Fprintf(io.ErrOut, "  Release:   %s\n", releaseURL(latest))
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	fmt.Fprintf(io.ErrOut, "\nOr install via npm:\n  npm install -g %s@%s\n", selfupdate.NpmPackage, latest)
	fmt.Fprintf(io.ErrOut, "\nAfter updating, also update skills:\n  npx -y skills add larksuite/cli -g -y\n")
	return nil
}

func doNpmUpdate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, updater *selfupdate.Updater) error {
	restore, err := updater.PrepareSelfReplace()
	if err != nil {
		return reportError(opts, io, output.ExitAPI, "update_error", "failed to prepare update: %s", err)
	}

	if !opts.JSON {
		fmt.Fprintf(io.ErrOut, "Updating lark-cli %s %s %s via npm ...\n", cur, symArrow(), latest)
	}

	npmResult := updater.RunNpmInstall(latest)
	if npmResult.Err != nil {
		restore()
		combined := npmResult.CombinedOutput()
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok": false, "error": map[string]interface{}{
					"type": "update_error", "message": fmt.Sprintf("npm install failed: %s", npmResult.Err),
					"detail": selfupdate.Truncate(combined, maxNpmOutput),
					"hint":   permissionHint(combined),
				},
			})
			return output.ErrBare(output.ExitAPI)
		}
		if npmResult.Stdout.Len() > 0 {
			fmt.Fprint(io.ErrOut, npmResult.Stdout.String())
		}
		if npmResult.Stderr.Len() > 0 {
			fmt.Fprint(io.ErrOut, npmResult.Stderr.String())
		}
		fmt.Fprintf(io.ErrOut, "\n%s Update failed: %s\n", symFail(), npmResult.Err)
		if hint := permissionHint(combined); hint != "" {
			fmt.Fprintf(io.ErrOut, "  %s\n", hint)
		}
		return output.ErrBare(output.ExitAPI)
	}

	// Verify the new binary is functional before proceeding.
	// If corrupt, restore the previous version from .old.
	if err := updater.VerifyBinary(latest); err != nil {
		restore()
		msg := fmt.Sprintf("new binary verification failed: %s", err)
		hint := verificationFailureHint(updater, latest)
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok":    false,
				"error": map[string]interface{}{"type": "update_error", "message": msg, "hint": hint},
			})
			return output.ErrBare(output.ExitAPI)
		}
		fmt.Fprintf(io.ErrOut, "\n%s %s\n", symFail(), msg)
		fmt.Fprintf(io.ErrOut, "  %s\n", hint)
		return output.ErrBare(output.ExitAPI)
	}

	// Skills update (best-effort).
	skillsResult := updater.RunSkillsUpdate()

	if opts.JSON {
		result := map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": latest,
			"latest_version": latest, "action": "updated",
			"message": fmt.Sprintf("lark-cli updated from %s to %s", cur, latest),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		if skillsResult.Err != nil {
			result["skills_warning"] = fmt.Sprintf("skills update failed: %s", skillsResult.Err)
			if detail := strings.TrimSpace(skillsResult.Stderr.String()); detail != "" {
				result["skills_detail"] = selfupdate.Truncate(detail, maxNpmOutput)
			}
		}
		output.PrintJson(io.Out, result)
		return nil
	}

	fmt.Fprintf(io.ErrOut, "\n%s Successfully updated lark-cli from %s to %s\n", symOK(), cur, latest)
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	fmt.Fprintf(io.ErrOut, "\nUpdating skills ...\n")
	if skillsResult.Err != nil {
		fmt.Fprintf(io.ErrOut, "%s Skills update failed: %s\n", symWarn(), skillsResult.Err)
		if detail := strings.TrimSpace(skillsResult.Stderr.String()); detail != "" {
			fmt.Fprintf(io.ErrOut, "  %s\n", selfupdate.Truncate(detail, 500))
		}
		fmt.Fprintf(io.ErrOut, "  Run manually: npx -y skills add larksuite/cli -g -y\n")
	} else {
		fmt.Fprintf(io.ErrOut, "%s Skills updated\n", symOK())
	}
	return nil
}

func permissionHint(npmOutput string) string {
	if strings.Contains(npmOutput, "EACCES") && !isWindows() {
		return "Permission denied. Try: sudo lark-cli update, or adjust your npm global prefix: https://docs.npmjs.com/resolving-eacces-permissions-errors"
	}
	return ""
}

func verificationFailureHint(updater *selfupdate.Updater, latest string) string {
	if updater.CanRestorePreviousVersion() {
		return "the previous version has been restored"
	}
	return fmt.Sprintf("automatic rollback is unavailable on this platform; reinstall manually: npm install -g %s@%s, or download %s", selfupdate.NpmPackage, latest, releaseURL(latest))
}
