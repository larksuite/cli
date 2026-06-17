// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// ProjectProfileError returns a project-specific error when the invocation
// selects a project profile that does not exist in the global profile list.
func ProjectProfileError(inv InvocationContext, multi *core.MultiAppConfig) error {
	if inv.Profile == "" || inv.ProfileSource != core.ProfileSourceProject {
		return nil
	}
	if multi != nil && multi.FindApp(inv.Profile) != nil {
		return nil
	}
	return core.ProjectProfileNotFoundError(inv.Profile, inv.ProfileConfigPath, profileNames(multi))
}

// ActiveProfileError classifies a missing effective profile by selector source.
func ActiveProfileError(inv InvocationContext, multi *core.MultiAppConfig) error {
	if err := ProjectProfileError(inv, multi); err != nil {
		return err
	}
	if inv.Profile != "" && inv.ProfileSource == core.ProfileSourceCLI {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "profile %q not found", inv.Profile).
			WithParam("--profile").
			WithHint("run: lark-cli profile list")
	}
	return errs.NewConfigError(errs.SubtypeNotConfigured, "no active profile").WithHint("run: lark-cli profile list")
}

func profileNames(multi *core.MultiAppConfig) []string {
	if multi == nil {
		return nil
	}
	return multi.ProfileNames()
}
