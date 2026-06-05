// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import "strings"

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
	if strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V") == strings.TrimPrefix(strings.TrimPrefix(currentVersion, "v"), "V") {
		return
	}
	SetPending(&StaleNotice{
		Current: version,
		Target:  currentVersion,
	})
}
