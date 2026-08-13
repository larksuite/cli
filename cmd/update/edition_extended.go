// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmdupdate

import (
	"errors"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/extendedupdate"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/update"
)

var (
	fetchExtendedLatest = extendedupdate.FetchLatest
	installExtended     = extendedupdate.Install
)

func updateLongDescription() string {
	return `Update lark-cli Extended from the matching GitHub Release.

The command downloads the lark-cli-extended asset for the current platform,
verifies its SHA-256 checksum and compiled edition identity, then replaces the
current binary. It never installs the Standard npm/npx edition.

Use --json for structured output (for AI agents and scripts).
Use --check to only check for updates without installing.`
}

func runEditionUpdate(opts *UpdateOptions) (bool, error) {
	io := opts.Factory.IOStreams
	cur := currentVersion()
	updater := newUpdater()
	if !opts.Check {
		updater.Brand = resolveSkillsBrand(opts.Factory, io.ErrOut)
		updater.CleanupStaleFiles()
	}
	output.PendingNotice = nil

	latest, err := fetchExtendedLatest()
	if err != nil {
		var typed errs.TypedError
		if errors.As(err, &typed) {
			return true, reportError(opts, io, "network", typed)
		}
		return true, reportError(opts, io, "network",
			errs.NewNetworkError(errs.SubtypeNetworkTransport,
				"failed to check the latest Extended version: %v", err).WithCause(err))
	}
	if update.ParseVersion(latest) == nil {
		return true, reportError(opts, io, "update_error",
			errs.NewInternalError(errs.SubtypeInvalidResponse,
				"invalid Extended version from GitHub Releases: %s", latest))
	}
	if !opts.Force && !update.IsNewer(latest, cur) {
		var skillsResult *skillscheck.SyncResult
		if !opts.Check {
			skillsResult = runSkillsAndState(updater, io, cur, opts.Force)
		}
		return true, reportAlreadyUpToDate(opts, io, cur, latest, skillsResult, opts.Check)
	}
	if opts.Check {
		return true, reportCheckResult(opts, io, cur, latest, true)
	}
	if !opts.JSON {
		fmt.Fprintf(io.ErrOut, "Updating lark-cli Extended %s %s %s from GitHub Releases ...\n", cur, symArrow(), latest)
	}
	if err := installExtended(latest); err != nil {
		var typed errs.TypedError
		if errors.As(err, &typed) {
			return true, reportError(opts, io, "update_error", typed)
		}
		return true, reportError(opts, io, "update_error",
			errs.NewInternalError(errs.SubtypeUnknown,
				"failed to install lark-cli Extended: %v", err).WithCause(err))
	}

	skillsResult := runSkillsAndState(updater, io, latest, opts.Force)
	if opts.JSON {
		result := map[string]interface{}{
			"ok": true, "previous_version": cur, "current_version": latest,
			"latest_version": latest, "edition": build.Edition, "action": "updated",
			"message": fmt.Sprintf("lark-cli Extended updated from %s to %s", cur, latest),
			"url":     releaseURL(latest), "changelog": changelogURL(),
		}
		applySkillsResult(result, skillsResult)
		output.PrintJson(io.Out, result)
		return true, nil
	}
	fmt.Fprintf(io.ErrOut, "\n%s Successfully updated lark-cli Extended from %s to %s\n", symOK(), cur, latest)
	fmt.Fprintf(io.ErrOut, "  Changelog: %s\n", changelogURL())
	emitSkillsTextHints(io, skillsResult)
	return true, nil
}
