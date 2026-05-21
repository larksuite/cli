// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package errcompat bridges the legacy *core.ConfigError shape into the
// canonical typed errors taxonomy in errs/. It is a thin boundary helper —
// placed in its own package so it can import both core (for the legacy
// type) and errs (for the typed targets) without creating an import cycle
// with internal/errclass, which intentionally avoids depending on
// internal/core.
package errcompat

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// PromoteConfigError is the stage-2 boundary helper that will convert a
// *core.ConfigError into the matching typed errs.* error. In stage 1 it
// is a passthrough — the dispatcher continues to render *core.ConfigError
// via the legacy envelope path (cmd/root.go asExitError) so the wire
// shape stays identical to pre-PR. Per-domain typed migration in stage 2+
// will fill in the actual promotion logic alongside its corresponding
// wire-change announcement.
func PromoteConfigError(cfgErr *core.ConfigError) error {
	if cfgErr == nil {
		return nil
	}
	return cfgErr
}

// _ keeps the errs import live so stage-2 fill-in does not need to re-add it.
var _ = errs.CategoryConfig
