// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func noActiveProfileError(f *cmdutil.Factory, multi *core.MultiAppConfig) error {
	return cmdutil.ActiveProfileError(f.Invocation, multi)
}
