// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

// Init runs the synchronous skills version check. Stores a StaleNotice
// when the local stamp records a version that does not match
// currentVersion. Safe to call from cmd/root.go before rootCmd.Execute();
// zero network, zero subprocess — only a local stamp file read.
//
// Skip rules: see shouldSkip (CI envs, DEV builds, non-release semver,
// LARKSUITE_CLI_NO_SKILLS_NOTIFIER opt-out).
//
// Failure modes (all → no notice, no nag):
//   - shouldSkip rule met
//   - ReadStamp returns an I/O error other than ENOENT
//   - Stamp matches currentVersion (in-sync)
//   - Stamp is missing (cold start) — only users who ran `lark-cli update`
//     opt into drift tracking; npx-only installs are intentionally silent.
func Init(currentVersion string) {
	// Clear any stale notice from a prior call so early returns below
	// (skip rules / read errors / cold start / in-sync) leave pending == nil
	// instead of preserving a stale value from a previous Init invocation.
	SetPending(nil)
	if shouldSkip(currentVersion) {
		return
	}
	stamp, err := ReadStamp()
	if err != nil {
		// Fail closed — don't nag for a transient FS problem.
		return
	}
	if stamp == "" {
		// Cold start: the stamp is written exclusively by `lark-cli update`
		// (runSkillsAndStamp). Users who installed skills via
		// `npx skills add larksuite/cli -g` have no stamp yet — they must
		// not be nagged with "skills not installed", since the on-disk
		// skills directory may already be fully populated.
		return
	}
	if stamp == currentVersion {
		return
	}
	SetPending(&StaleNotice{
		Current: stamp, // guaranteed non-empty under the new contract
		Target:  currentVersion,
	})
}
