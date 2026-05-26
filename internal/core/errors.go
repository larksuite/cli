// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "fmt"

// ConfigError is a structured error from config resolution.
// It carries enough information for main.go to convert it into an output.ExitError.
type ConfigError struct {
	Code    int    // exit code: 3 (config errors share the auth exit code)
	Type    string // "config" or "auth"
	Message string
	Hint    string
}

func (e *ConfigError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s\n  %s", e.Message, e.Hint)
	}
	return e.Message
}
