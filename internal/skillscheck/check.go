// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import "github.com/larksuite/cli/internal/versioncheck"

// Init runs the synchronous skills version check. Stores a StaleNotice when
// the local skills state records a version that does not match currentVersion,
// or the last sync could not determine the complete official Skill set.
// Safe to call from cmd/root.go before rootCmd.Execute(); zero network, zero
// subprocess — only a local state file read.
//
// Skip rules: see shouldSkip (CI envs, DEV builds, non-release semver,
// LARKSUITE_CLI_NO_SKILLS_NOTIFIER opt-out).
func Init(currentVersion string) {
	InitForSource(currentVersion, OfficialSourceIdentity, false)
}

// InitForSource also considers which distribution owns the installed Skills.
// exactTarget is true for manifest distributions, whose versions are opaque
// strings rather than SemVer releases.
func InitForSource(currentVersion, sourceIdentity string, exactTarget bool) {
	SetPending(nil)
	if shouldSkip(currentVersion, exactTarget) {
		return
	}
	state, ok, err := ReadState()
	if err != nil || !ok || state.Version == "" {
		return
	}
	versionMatches := versioncheck.Equal(state.Version, currentVersion)
	if exactTarget {
		versionMatches = state.Version == currentVersion
	}
	if versionMatches && !state.OfficialSkillsUnknown && MatchesSource(state, sourceIdentity) {
		return
	}
	SetPending(&StaleNotice{
		Current:         state.Version,
		Target:          currentVersion,
		OfficialUnknown: state.OfficialSkillsUnknown,
	})
}
