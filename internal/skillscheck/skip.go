// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"os"

	"github.com/larksuite/cli/internal/versioncheck"
)

// shouldSkip returns true when the skills check should be silently
// suppressed. Mirrors internal/update.shouldSkip semantics but uses
// a dedicated opt-out env var so users can disable the skills nag
// without also disabling the binary update nag.
//
// exactTarget marks a manifest distribution: its version is an opaque string
// chosen by the producer, so the SemVer/release gate (including the DEV
// marker) does not apply — only the opt-out, CI, and a missing version
// suppress the check.
func shouldSkip(version string, exactTarget bool) bool {
	if os.Getenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER") != "" {
		return true
	}
	if versioncheck.IsCIEnv() {
		return true
	}
	if version == "" {
		return true
	}
	if exactTarget {
		return false
	}
	if version == "DEV" || version == "dev" {
		return true
	}
	return !versioncheck.IsRelease(version)
}
