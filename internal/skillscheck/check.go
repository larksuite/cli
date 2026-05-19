// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

// Init runs the synchronous skills version check. Stores a StaleNotice when
// the local skills state records a version that does not match currentVersion.
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
	version, ok := ReadSyncedVersion()
	if !ok {
		return
	}
	if version == currentVersion {
		return
	}
	SetPending(&StaleNotice{
		Current: version,
		Target:  currentVersion,
	})
}
