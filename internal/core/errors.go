// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "fmt"

// ConfigErrorRung tags the resolution rung that produced a ConfigError.
// Decorators (e.g. credential.decorateUserResolutionError) use this
// instead of substring-matching the Message — substring matches drift
// when copy is reworded and false-match neighboring messages that
// happen to contain the same word.
type ConfigErrorRung string

const (
	// RungUnspecified is the zero value — used by historical call sites
	// that build a ConfigError without thinking about the rung.
	RungUnspecified ConfigErrorRung = ""
	// RungProfile means the resolution failed before a single AppConfig
	// could be picked: profile not found, dangling CurrentApp, schema
	// errors, etc. User-rung decorators MUST skip this — adding a
	// "user" hint to a profile failure is wrong copy.
	RungProfile ConfigErrorRung = "profile"
	// RungUser means the resolution failed at the user fallback chain
	// (--user override miss, drifted CurrentUser, no users in the
	// profile). User-rung decorators decorate THIS rung.
	RungUser ConfigErrorRung = "user"
)

// ConfigError is a structured error from config resolution.
// It carries enough information for main.go to convert it into an output.ExitError.
type ConfigError struct {
	Code    int    // exit code: 3 (config errors share the auth exit code)
	Type    string // "config" or "auth"
	Message string
	Hint    string
	// Rung is the structural classification of which resolution layer
	// produced the error. Empty for legacy / historical errors.
	Rung ConfigErrorRung
}

func (e *ConfigError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s\n  %s", e.Message, e.Hint)
	}
	return e.Message
}
