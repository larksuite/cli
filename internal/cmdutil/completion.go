// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"sync/atomic"

	"github.com/spf13/cobra"
)

// Cobra keeps completion callbacks in a package-global map keyed by
// *pflag.Flag with no removal path, so registrations made for a *cobra.Command
// outlive the command itself. Default to disabled (zero value = false) and let
// callers that actually serve a completion request opt in via
// SetFlagCompletionsEnabled(true).
var flagCompletionsEnabled atomic.Bool

// SetFlagCompletionsEnabled toggles whether RegisterFlagCompletion actually
// registers callbacks with cobra. Typically set once at process start.
func SetFlagCompletionsEnabled(enabled bool) {
	flagCompletionsEnabled.Store(enabled)
}

// FlagCompletionsEnabled reports the current switch state.
func FlagCompletionsEnabled() bool {
	return flagCompletionsEnabled.Load()
}

// RegisterFlagCompletion wraps (*cobra.Command).RegisterFlagCompletionFunc
// and honors the package switch. The underlying error is swallowed to match
// the `_ = cmd.RegisterFlagCompletionFunc(...)` style already used here.
func RegisterFlagCompletion(cmd *cobra.Command, flagName string, fn cobra.CompletionFunc) {
	if !flagCompletionsEnabled.Load() {
		return
	}
	_ = cmd.RegisterFlagCompletionFunc(flagName, fn)
}
