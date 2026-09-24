// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import "strings"

// NormalizeVersion canonicalizes a version string for state comparison.
// Trims surrounding whitespace and a leading "v"/"V" so versions written
// from Makefile (git describe → "v1.0.0") and npm (no prefix) compare equal.
func NormalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	return strings.TrimPrefix(s, "V")
}

// Init runs the synchronous skills version check. Stores a StaleNotice when
// the local skills state records a version that does not match currentVersion,
// the last sync could not determine the complete official Skill set, or the
// state file exists but is unreadable.
// Safe to call from cmd/root.go before rootCmd.Execute(); zero network, zero
// subprocess — only a local state file read.
//
// Skip rules: see shouldSkip (CI envs, DEV builds, non-release semver,
// LARKSUITE_CLI_NO_SKILLS_NOTIFIER opt-out).
func Init(currentVersion string) {
	SetPending(nil)
	if shouldSkip(currentVersion) {
		return
	}
	state, ok, err := ReadState()
	if err != nil {
		// The state file exists but cannot be parsed: drift detection is
		// blind, so surface a notice instead of failing silently.
		SetPending(&StaleNotice{StateUnreadable: true})
		return
	}
	if !ok || state.Version == "" {
		return
	}
	if NormalizeVersion(state.Version) == NormalizeVersion(currentVersion) && !state.OfficialSkillsUnknown {
		return
	}
	SetPending(&StaleNotice{
		Current:         state.Version,
		Target:          currentVersion,
		OfficialUnknown: state.OfficialSkillsUnknown,
	})
}
