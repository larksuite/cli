// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/update"
)

const (
	repoURL         = "https://github.com/larksuite/cli"
	maxNpmOutput    = 2000
	maxStderrDetail = 500
	osWindows       = "windows"
)

// Overridable for testing.
var (
	fetchLatest    = func() (string, error) { return update.FetchLatest() }
	currentVersion = func() string { return build.Version }
	currentOS      = runtime.GOOS
	newUpdater     = func() *selfupdate.Updater { return selfupdate.New() }
	syncSkills     = func(opts skillscheck.SyncOptions) *skillscheck.SyncResult { return skillscheck.SyncSkills(opts) }
)

func isWindows() bool { return currentOS == osWindows }

// normalizeVersion canonicalizes a version string for state comparison.
// Strips a leading "v" so versions written from Makefile (git describe →
// "v1.0.0") and npm (no prefix → "1.0.0") compare equal.
func normalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	return strings.TrimPrefix(s, "V")
}

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
	cmdutil.SetRisk(cmd, "high-risk-write")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	io := opts.Factory.IOStreams
	cur := currentVersion()
	updater := newUpdater()

	if !opts.Check {
		updater.CleanupStaleFiles()
	}
	output.PendingNotice = nil

	// 1. Fetch latest version
	latest, err := fetchLatest()
	if err != nil {
		return reportError(opts, io, "network",
			errs.NewNetworkError(errs.SubtypeNetworkTransport, "failed to check latest version: %s", err).WithCause(err))
	}

	// 2. Validate version format
	if update.ParseVersion(latest) == nil {
		return reportError(opts, io, "update_error",
			errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid version from registry: %s", latest))
	}

	// 3. Compare versions
	if !opts.Force && !update.IsNewer(latest, cur) {
		var skillsResult *skillscheck.SyncResult
		if !opts.Check {
			skillsResult = runSkillsAndState(updater, io, cur, opts.Force)
		}
		return reportAlreadyUpToDate(opts, io, cur, latest, skillsResult, opts.Check)
	}

	// 4. Detect installation method
	detect := updater.DetectInstallMethod()

	// 5. --check
	if opts.Check {
		return reportCheckResult(opts, io, cur, latest, detect.CanAutoUpdate())
	}

	// 6. Execute update
	if !detect.CanAutoUpdate() {
		return doManualUpdate(opts, io, cur, latest, detect, updater)
	}
	return doNpmUpdate(opts, io, cur, latest, updater)
}

// --- Output helpers ---

// reportError emits the failure on the requested surface: JSON mode prints the
// {ok:false, error:{type, message}} envelope to stdout and signals the typed
// error's exit code bare; human mode returns the typed error for the
// dispatcher to render.
func reportError(opts *UpdateOptions, io *cmdutil.IOStreams, errType string, typedErr errs.TypedError) error {
	if opts.JSON {
		output.PrintJson(io.Out, map[string]interface{}{
			"ok": false, "error": map[string]interface{}{"type": errType, "message": typedErr.ProblemDetail().Message},
		})
		return output.ErrBare(output.ExitCodeOf(typedErr))
	}
	return typedErr
}

func reportCheckResult(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, canAutoUpdate bool) error {
	if opts.JSON {
		out := map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": cur,
			"latest_version": latest, "action": "update_available",
			"auto_update": canAutoUpdate,
			"message":     fmt.Sprintf("lark-cli %s %s %s available", cur, symArrow(), latest),
			"url":         releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsStatus(out, cur)
		output.PrintJson(io.Out, out)
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

func doManualUpdate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, detect selfupdate.DetectResult, updater *selfupdate.Updater) error {
	skillsResult := runSkillsAndState(updater, io, cur, opts.Force)

	reason := detect.ManualReason()
	if opts.JSON {
		out := map[string]interface{}{
			"ok": true, "previous_version": cur, "latest_version": latest,
			"action":  "manual_required",
			"message": fmt.Sprintf("Automatic update unavailable: %s (path: %s)", reason, detect.ResolvedPath),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsResult(out, skillsResult)
		output.PrintJson(io.Out, out)
		return nil
	}
	fmt.Fprintf(io.ErrOut, "Automatic update unavailable: %s (path: %s).\n\n", reason, detect.ResolvedPath)
	fmt.Fprintf(io.ErrOut, "To update manually, download the latest release:\n")
	fmt.Fprintf(io.ErrOut, "  Release:   %s\n", releaseURL(latest))
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	fmt.Fprintf(io.ErrOut, "\nOr install via npm (note: skills will not be synced):\n  npm install -g %s@%s\n  npx skills add larksuite/cli -y -g   # sync skills separately\n", selfupdate.NpmPackage, latest)
	emitSkillsTextHints(io, skillsResult)
	return nil
}

func doNpmUpdate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, updater *selfupdate.Updater) error {
	restore, err := updater.PrepareSelfReplace()
	if err != nil {
		return reportError(opts, io, "update_error",
			errs.NewAPIError(errs.SubtypeUnknown, "failed to prepare update: %s", err).WithCause(err))
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

	skillsResult := runSkillsAndState(updater, io, latest, opts.Force)

	if opts.JSON {
		result := map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": latest,
			"latest_version": latest, "action": "updated",
			"message": fmt.Sprintf("lark-cli updated from %s to %s", cur, latest),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsResult(result, skillsResult)
		output.PrintJson(io.Out, result)
		return nil
	}

	fmt.Fprintf(io.ErrOut, "\n%s Successfully updated lark-cli from %s to %s\n", symOK(), cur, latest)
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	if skillsResult != nil {
		fmt.Fprintf(io.ErrOut, "\nUpdating skills ...\n")
	}
	emitSkillsTextHints(io, skillsResult)
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
	return fmt.Sprintf("automatic rollback is unavailable on this platform; reinstall manually (skills will not be synced): npm install -g %s@%s && npx skills add larksuite/cli -y -g, or download %s", selfupdate.NpmPackage, latest, releaseURL(latest))
}

func runSkillsAndState(updater *selfupdate.Updater, io *cmdutil.IOStreams, stateVersion string, force bool) *skillscheck.SyncResult {
	if !force {
		if existing, ok := skillscheck.ReadSyncedVersion(); ok && normalizeVersion(existing) == normalizeVersion(stateVersion) {
			return nil
		}
	}
	result := syncSkills(skillscheck.SyncOptions{
		Version: stateVersion,
		Force:   force,
		Runner:  updater,
	})
	if result.Err != nil && strings.Contains(result.Err.Error(), "state not written") {
		fmt.Fprintf(io.ErrOut, "warning: %v\n", result.Err)
	}
	return result
}

// reportAlreadyUpToDate emits the JSON / pretty output for the
// already-up-to-date branch, including any skills_action / skills_warning
// fields derived from skillsResult. When check is true, this is the pure
// report path (spec §3.6): no side-effects, JSON envelope uses
// skills_status (spec §4.2) instead of skills_action.
func reportAlreadyUpToDate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, skillsResult *skillscheck.SyncResult, check bool) error {
	if opts.JSON {
		out := map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": cur,
			"latest_version": latest, "action": "already_up_to_date",
			"message": fmt.Sprintf("lark-cli %s is already up to date", cur),
		}
		if check {
			applySkillsStatus(out, cur)
		} else {
			applySkillsResult(out, skillsResult)
		}
		output.PrintJson(io.Out, out)
		return nil
	}
	fmt.Fprintf(io.ErrOut, "%s lark-cli %s is already up to date\n", symOK(), cur)
	if !check {
		emitSkillsTextHints(io, skillsResult)
	}
	return nil
}

func applySkillsStatus(env map[string]interface{}, target string) {
	state, readable, err := skillscheck.ReadState()
	if err != nil || !readable || state.Version == "" {
		return
	}
	status := map[string]interface{}{
		"current": state.Version,
		"target":  target,
		"in_sync": normalizeVersion(state.Version) == normalizeVersion(target),
	}
	if len(state.OfficialSkills) > 0 {
		status["official"] = len(state.OfficialSkills)
	}
	if len(state.UpdatedSkills) > 0 {
		status["updated"] = len(state.UpdatedSkills)
	}
	if len(state.SkippedDeletedSkills) > 0 {
		status["skipped_deleted"] = state.SkippedDeletedSkills
	}
	env["skills_status"] = status
}

func applySkillsResult(env map[string]interface{}, r *skillscheck.SyncResult) {
	switch {
	case r == nil:
		env["skills_action"] = "in_sync"
	case r.Err != nil:
		env["skills_action"] = "failed"
		env["skills_warning"] = fmt.Sprintf("skills update failed: %s", r.Err)
		env["skills_summary"] = skillsSummary(r)
	default:
		env["skills_action"] = "synced"
		env["skills_summary"] = skillsSummary(r)
	}
}

func skillsSummary(r *skillscheck.SyncResult) map[string]interface{} {
	summary := map[string]interface{}{
		"official":        len(r.Official),
		"updated":         len(r.Updated),
		"added":           len(r.Added),
		"skipped_deleted": len(r.SkippedDeleted),
	}
	if len(r.Failed) > 0 {
		summary["failed"] = r.Failed
	}
	return summary
}

func emitSkillsTextHints(io *cmdutil.IOStreams, r *skillscheck.SyncResult) {
	switch {
	case r == nil:
	case r.Err != nil:
		fmt.Fprintf(io.ErrOut, "%s Skills update failed: %v\n", symWarn(), r.Err)
		if len(r.Failed) > 0 {
			fmt.Fprintf(io.ErrOut, "  Failed skills: %s\n", strings.Join(r.Failed, ", "))
		}
		fmt.Fprintf(io.ErrOut, "  To retry all official skills: lark-cli update --force\n")
	case r.Force:
		fmt.Fprintf(io.ErrOut, "%s Skills updated: restored all %d official skills\n", symOK(), len(r.Official))
	default:
		fmt.Fprintf(io.ErrOut, "%s Skills updated: %d official, %d updated, %d added, %d skipped because deleted locally\n", symOK(), len(r.Official), len(r.Updated), len(r.Added), len(r.SkippedDeleted))
		if len(r.SkippedDeleted) > 0 {
			fmt.Fprintf(io.ErrOut, "  To restore all official skills: lark-cli update --force\n")
		}
	}
}
