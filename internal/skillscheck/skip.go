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
func shouldSkip(version string) bool {
	if os.Getenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER") != "" {
		return true
	}
	if versioncheck.IsCIEnv() {
		return true
	}
	if version == "DEV" || version == "dev" || version == "" {
		return true
	}
	return !versioncheck.IsRelease(version)
}
