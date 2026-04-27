// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"sync/atomic"

	"github.com/spf13/cobra"
)

// Cobra keeps completion callbacks in a package-global map keyed by
// *pflag.Flag with no removal path, so registrations made for a *cobra.Command
// outlive the command itself. Skip registration when the current invocation
// will not serve a completion request.
var flagCompletionsDisabled atomic.Bool

// SetFlagCompletionsDisabled switches RegisterFlagCompletion between
// registering and no-op. Typically set once at process start.
func SetFlagCompletionsDisabled(disabled bool) {
	flagCompletionsDisabled.Store(disabled)
}

// FlagCompletionsDisabled reports the current switch state.
func FlagCompletionsDisabled() bool {
	return flagCompletionsDisabled.Load()
}

// RegisterFlagCompletion wraps (*cobra.Command).RegisterFlagCompletionFunc
// and honors the package switch. The underlying error is swallowed to match
// the `_ = cmd.RegisterFlagCompletionFunc(...)` style already used here.
func RegisterFlagCompletion(cmd *cobra.Command, flagName string, fn cobra.CompletionFunc) {
	if flagCompletionsDisabled.Load() {
		return
	}
	_ = cmd.RegisterFlagCompletionFunc(flagName, fn)
}
