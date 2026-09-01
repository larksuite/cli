// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdupdate

import (
	"fmt"
	stdio "io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
	Factory      *cmdutil.Factory
	JSON         bool
	Force        bool
	Check        bool
	SkillsLayout string
}

// NewCmdUpdate creates the update command.
func NewCmdUpdate(f *cmdutil.Factory) *cobra.Command {
	opts := &UpdateOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update lark-cli to the latest version",
		Long: `Update lark-cli to the latest version.

Detects the installation method automatically:
  - npm install:  runs npm install -g @larksuite/cli@<version>
  - pnpm install: runs pnpm add -g @larksuite/cli@<version>
  - manual/other: shows GitHub Releases download URL

Use --json for structured output (for AI agents and scripts).
Use --check to only check for updates without installing.

The skill name "lark-suite" is reserved for CLI-managed suite layout.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateRun(opts)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force reinstall even if already up to date")
	cmd.Flags().BoolVar(&opts.Check, "check", false, "only check for updates, do not install")
	cmd.Flags().StringVar(&opts.SkillsLayout, "skills-layout", "", "skills layout: separate or suite")
	cmdutil.SetRisk(cmd, "high-risk-write")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	io := opts.Factory.IOStreams
	if _, err := skillscheck.ParseLayout(opts.SkillsLayout); err != nil {
		return reportError(opts, io, "validation",
			errs.NewValidationError(errs.SubtypeInvalidArgument, "--skills-layout must be one of separate or suite").WithParam("--skills-layout"))
	}
	if opts.Check && strings.TrimSpace(opts.SkillsLayout) != "" {
		return reportError(opts, io, "validation",
			errs.NewValidationError(errs.SubtypeInvalidArgument, "--skills-layout cannot be used with --check").
				WithParam("--skills-layout").
				WithHint("Remove --skills-layout when using --check."))
	}
	cur := currentVersion()
	updater := newUpdater()
	// Brand only steers skills sync. updateRun skips that resolution in --check,
	// where the Updater's zero-value brand retains the Feishu default.
	if !opts.Check {
		updater.Brand = resolveSkillsBrand(opts.Factory, io.ErrOut)
		updater.CleanupStaleFiles()
	}
	output.PendingNotice = nil

	// 1. Fetch latest version.
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
			skillsResult = runSkillsAndState(updater, io, cur, opts.Force, opts.SkillsLayout)
			if err := reportSkillsFailure(opts, io, skillsResult); err != nil {
				return err
			}
		}
		return reportAlreadyUpToDate(opts, io, cur, latest, skillsResult, opts.Check)
	}

	// 4. Detect installation method.
	detect := updater.DetectInstallMethod()

	// 5. --check
	if opts.Check {
		return reportCheckResult(opts, io, cur, latest, detect.CanAutoUpdate())
	}

	// 6. Execute update
	if !detect.CanAutoUpdate() {
		return doManualUpdate(opts, io, cur, latest, detect, updater)
	}
	return doAutoUpdate(opts, io, cur, latest, detect, updater)
}

// resolveSkillsBrand returns the skills-source brand: resolved config first,
// then the active profile's raw config entry (the brand is not a secret; a
// locked keychain must not flip the source), then the default with a notice.
func resolveSkillsBrand(f *cmdutil.Factory, errOut stdio.Writer) core.LarkBrand {
	if cfg, err := f.Config(); err == nil && cfg != nil {
		return core.ParseBrand(string(cfg.Brand))
	}
	if raw, err := core.LoadMultiAppConfig(); err == nil {
		if app := raw.CurrentAppConfig(f.Invocation.Profile); app != nil {
			return core.ParseBrand(string(app.Brand))
		}
	}
	fmt.Fprintf(errOut, "note: could not resolve the configured brand; syncing skills from the default source\n")
	return core.BrandFeishu
}

// --- Output helpers ---

// reportError emits the failure on the requested surface: JSON mode prints the
// {ok:false, error:{type, message}} envelope to stdout and signals the typed
// error's exit code bare; human mode returns the typed error for the
// dispatcher to render.
func reportError(opts *UpdateOptions, io *cmdutil.IOStreams, errType string, typedErr errs.TypedError) error {
	return reportErrorWithFields(opts, io, errType, typedErr, nil)
}

func reportErrorWithFields(opts *UpdateOptions, io *cmdutil.IOStreams, errType string, typedErr errs.TypedError, fields map[string]interface{}) error {
	if opts.JSON {
		out := make(map[string]interface{}, len(fields)+2)
		for key, value := range fields {
			out[key] = value
		}
		out["ok"] = false
		out["error"] = map[string]interface{}{"type": errType, "message": typedErr.ProblemDetail().Message}
		output.PrintJson(io.Out, out)
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
	skillsResult := runSkillsAndState(updater, io, cur, opts.Force, opts.SkillsLayout)
	reason := detect.ManualReason()
	if opts.JSON {
		out := map[string]interface{}{
			"ok": true, "previous_version": cur, "latest_version": latest,
			"action":  "manual_required",
			"message": fmt.Sprintf("Automatic update unavailable: %s (path: %s)", reason, detect.ResolvedPath),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsResult(out, skillsResult)
		if err := reportSkillsFailureWithFields(opts, io, skillsResult, out); err != nil {
			return err
		}
		output.PrintJson(io.Out, out)
		return nil
	}
	fmt.Fprintf(io.ErrOut, "Automatic update unavailable: %s (path: %s).\n\n", reason, detect.ResolvedPath)
	fmt.Fprintf(io.ErrOut, "To update manually, download the latest release:\n")
	fmt.Fprintf(io.ErrOut, "  Release:   %s\n", releaseURL(latest))
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	if detect.Method == selfupdate.InstallPnpm {
		fmt.Fprintf(io.ErrOut, "\nOr install via pnpm (note: skills will not be synced):\n  pnpm add -g %s@%s\n  pnpm dlx skills add larksuite/cli -y -g   # sync skills separately\n", selfupdate.NpmPackage, latest)
	} else {
		fmt.Fprintf(io.ErrOut, "\nOr install via npm (note: skills will not be synced):\n  npm install -g %s@%s\n  npx skills add larksuite/cli -y -g   # sync skills separately\n", selfupdate.NpmPackage, latest)
	}
	if err := reportSkillsFailure(opts, io, skillsResult); err != nil {
		return err
	}
	emitSkillsTextHints(io, skillsResult)
	return nil
}

func doAutoUpdate(opts *UpdateOptions, io *cmdutil.IOStreams, cur, latest string, detect selfupdate.DetectResult, updater *selfupdate.Updater) error {
	pm := "npm"
	install := updater.RunNpmInstall
	if detect.Method == selfupdate.InstallPnpm {
		pm = "pnpm"
		install = updater.RunPnpmInstall
	}

	restore, err := updater.PrepareSelfReplace()
	if err != nil {
		return reportError(opts, io, "update_error",
			errs.NewAPIError(errs.SubtypeUnknown, "failed to prepare update: %s", err).WithCause(err))
	}

	if !opts.JSON {
		fmt.Fprintf(io.ErrOut, "Updating lark-cli %s %s %s via %s ...\n", cur, symArrow(), latest, pm)
	}

	npmResult := install(latest)
	if npmResult.Err != nil {
		restore()
		combined := npmResult.CombinedOutput()
		if opts.JSON {
			output.PrintJson(io.Out, map[string]interface{}{
				"ok": false, "error": map[string]interface{}{
					"type": "update_error", "message": fmt.Sprintf("%s install failed: %s", pm, npmResult.Err),
					"detail": selfupdate.Truncate(combined, maxNpmOutput),
					"hint":   permissionHint(combined, pm),
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
		if hint := permissionHint(combined, pm); hint != "" {
			fmt.Fprintf(io.ErrOut, "  %s\n", hint)
		}
		return output.ErrBare(output.ExitAPI)
	}

	// Verify the new binary is functional before proceeding.
	// If corrupt, restore the previous version from .old.
	if err := updater.VerifyBinary(latest); err != nil {
		restore()
		msg := fmt.Sprintf("new binary verification failed: %s", err)
		hint := verificationFailureHint(updater, latest, pm)
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

	skillsResult := runSkillsAndState(updater, io, latest, opts.Force, opts.SkillsLayout)
	if skillsResult != nil && skillsResult.Err != nil {
		fields := map[string]interface{}{
			"previous_version": cur, "current_version": latest,
			"latest_version": latest, "action": "updated",
			"message": fmt.Sprintf("lark-cli updated from %s to %s, but skills update failed", cur, latest),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsResult(fields, skillsResult)
		if !opts.JSON {
			fmt.Fprintf(io.ErrOut, "\n%s lark-cli binary updated from %s to %s\n", symOK(), cur, latest)
			fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
		}
		return reportSkillsFailureWithFields(opts, io, skillsResult, fields)
	}

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
		skillsPM := "npx"
		if detect.Method == selfupdate.InstallPnpm && detect.PnpmAvailable {
			skillsPM = "pnpm dlx"
		}
		fmt.Fprintf(io.ErrOut, "\nUpdating skills via %s ...\n", skillsPM)
	}
	emitSkillsTextHints(io, skillsResult)
	return nil
}

func permissionHint(pmOutput, pm string) string {
	if !strings.Contains(pmOutput, "EACCES") || isWindows() {
		return ""
	}
	if pm == "pnpm" {
		return "Permission denied. Ensure your pnpm global directory is writable — re-run `pnpm setup`, or see https://pnpm.io/pnpm-cli"
	}
	return "Permission denied. Try: sudo lark-cli update, or adjust your npm global prefix: https://docs.npmjs.com/resolving-eacces-permissions-errors"
}

func verificationFailureHint(updater *selfupdate.Updater, latest, pm string) string {
	if updater.CanRestorePreviousVersion() {
		return "the previous version has been restored"
	}
	if pm == "pnpm" {
		return fmt.Sprintf("automatic rollback is unavailable on this platform; reinstall manually (skills will not be synced): pnpm add -g %s@%s && pnpm dlx skills add larksuite/cli -y -g, or download %s", selfupdate.NpmPackage, latest, releaseURL(latest))
	}
	return fmt.Sprintf("automatic rollback is unavailable on this platform; reinstall manually (skills will not be synced): npm install -g %s@%s && npx skills add larksuite/cli -y -g, or download %s", selfupdate.NpmPackage, latest, releaseURL(latest))
}

func runSkillsAndState(updater *selfupdate.Updater, io *cmdutil.IOStreams, stateVersion string, force bool, requestedLayout string) *skillscheck.SyncResult {
	layout, _ := skillscheck.ParseLayout(requestedLayout)
	if !force {
		if state, ok, err := skillscheck.ReadState(); err == nil && ok && normalizeVersion(state.Version) == normalizeVersion(stateVersion) {
			if !state.OfficialSkillsUnknown && (layout == "" || skillscheck.EffectiveLayout(state) == layout) {
				return nil
			}
		}
	}
	result := syncSkills(skillscheck.SyncOptions{
		Version: stateVersion,
		Layout:  layout,
		Force:   force,
		Runner:  updater,
	})
	if result.Err != nil && strings.Contains(result.Err.Error(), "state not written") {
		fmt.Fprintf(io.ErrOut, "warning: %v\n", result.Err)
	}
	return result
}

func reportSkillsFailure(opts *UpdateOptions, io *cmdutil.IOStreams, result *skillscheck.SyncResult) error {
	return reportSkillsFailureWithFields(opts, io, result, nil)
}

func reportSkillsFailureWithFields(opts *UpdateOptions, io *cmdutil.IOStreams, result *skillscheck.SyncResult, fields map[string]interface{}) error {
	if result == nil || result.Err == nil {
		return nil
	}
	typedErr := errs.NewInternalError(errs.SubtypeUnknown, "skills update failed: %s", result.Err).
		WithHint("retry with `lark-cli update --force`").
		WithCause(result.Err)
	return reportErrorWithFields(opts, io, "skills_update_error", typedErr, fields)
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
		"in_sync": normalizeVersion(state.Version) == normalizeVersion(target) && !state.OfficialSkillsUnknown,
	}
	if state.OfficialSkillsUnknown {
		status["official_unknown"] = true
	} else if len(state.OfficialSkills) > 0 {
		status["official"] = len(state.OfficialSkills)
	}
	if len(state.UpdatedSkills) > 0 {
		status["updated"] = len(state.UpdatedSkills)
	}
	if len(state.SkippedDeletedSkills) > 0 {
		status["skipped_deleted"] = state.SkippedDeletedSkills
	}
	status["layout"] = skillscheck.EffectiveLayout(state)
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
		if r.Warning != "" {
			env["skills_warning"] = r.Warning
		}
	}
}

func skillsSummary(r *skillscheck.SyncResult) map[string]interface{} {
	summary := map[string]interface{}{
		"updated":         len(r.Updated),
		"added":           len(r.Added),
		"skipped_deleted": len(r.SkippedDeleted),
		"layout":          r.Layout,
	}
	if r.OfficialUnknown {
		summary["official_unknown"] = true
	} else {
		summary["official"] = len(r.Official)
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
	case r.Warning != "":
		fmt.Fprintf(io.ErrOut, "%s Skills updated using %s layout\n", symOK(), r.Layout)
		fmt.Fprintf(io.ErrOut, "%s %s\n", symWarn(), r.Warning)
	case r.Force:
		fmt.Fprintf(io.ErrOut, "%s Skills updated using %s layout: restored all %d official skills\n", symOK(), r.Layout, len(r.Official))
	default:
		fmt.Fprintf(io.ErrOut, "%s Skills updated using %s layout: %d official, %d updated, %d added, %d skipped because deleted locally\n", symOK(), r.Layout, len(r.Official), len(r.Updated), len(r.Added), len(r.SkippedDeleted))
		if len(r.SkippedDeleted) > 0 {
			fmt.Fprintf(io.ErrOut, "  To restore all official skills: lark-cli update --force\n")
		}
	}
}
