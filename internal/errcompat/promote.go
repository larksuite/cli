// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package errcompat provides boundary helpers that bridge legacy error types
// to the typed errs/ taxonomy. These helpers run at the dispatcher boundary
// (cmd/root.go.handleRootError) before the typed envelope writer, converting
// pre-typed-taxonomy errors (*core.ConfigError, *internalauth.NeedAuthorizationError)
// into typed *errs.* errors while preserving the original error in the Cause
// chain so existing `errors.As` callers continue to match.
package errcompat

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// PromoteConfigError converts a legacy *core.ConfigError into the matching
// typed errs.*Error based on cfgErr.Type. Called from cmd/root.go.handleRootError
// before the typed envelope writer. The original *core.ConfigError is preserved
// in the Cause chain so external `errors.As(&core.ConfigError{})` callers
// (cmd/auth/list.go, cmd/doctor/doctor.go, etc.) still match.
func PromoteConfigError(cfgErr *core.ConfigError) error {
	if cfgErr == nil {
		return nil
	}
	switch cfgErr.Type {
	case "auth":
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "%s", cfgErr.Message).
			WithHint("%s", cfgErr.Hint).
			WithCause(cfgErr)
	case "config":
		subtype := errs.SubtypeNotConfigured
		lower := strings.ToLower(cfgErr.Message)
		if strings.Contains(lower, "parse") || strings.Contains(lower, "invalid") {
			subtype = errs.SubtypeInvalidConfig
		}
		return errs.NewConfigError(subtype, "%s", cfgErr.Message).
			WithHint("%s", cfgErr.Hint).
			WithCause(cfgErr)
	default:
		// dynamic Type (e.g. workspace name like "bind"/"hermes"/"openclaw") → NotConfigured
		return errs.NewConfigError(errs.SubtypeNotConfigured, "%s", cfgErr.Message).
			WithHint("%s", cfgErr.Hint).
			WithCause(cfgErr)
	}
}
