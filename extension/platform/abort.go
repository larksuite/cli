// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "fmt"

// AbortError is returned by a Wrapper that wants to short-circuit the
// command chain (instead of calling next). The framework converts it
// to an *output.ExitError with type "hook" so the JSON envelope carries
// the structured fields agents expect.
//
// HookName is the framework-namespaced name ("secaudit.approval"); the
// Registrar adds the plugin-name prefix automatically.
//
// Cause and Detail are optional. Cause lets the consumer use
// errors.Is/As to find the underlying cause; Detail is serialized into
// envelope.detail under the "detail" key for agent consumption.
type AbortError struct {
	HookName string
	Reason   string
	Cause    error
	Detail   any
}

// Error renders a human-readable message; HookName + Reason + Cause are
// included when present.
func (e *AbortError) Error() string {
	msg := fmt.Sprintf("hook %q aborted: %s", e.HookName, e.Reason)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

// Unwrap enables errors.Is / errors.As to traverse to Cause.
func (e *AbortError) Unwrap() error { return e.Cause }
