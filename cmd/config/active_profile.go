// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func noActiveProfileError(f *cmdutil.Factory, multi *core.MultiAppConfig) error {
	if f.Invocation.Profile != "" && f.Invocation.ProfileSource == core.ProfileSourceProject {
		return core.ProjectProfileNotFoundError(f.Invocation.Profile, f.Invocation.ProfileConfigPath, multi.ProfileNames())
	}
	return errs.NewConfigError(errs.SubtypeNotConfigured, "no active profile").WithHint("run: lark-cli profile list")
}
