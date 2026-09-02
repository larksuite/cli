// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

// defaultRestrictedCommandUnavailableMessage is the distribution-neutral
// fallback for a concealed command. It lives in the presentation layer rather
// than extension/platform.Rule: the same enforcement rule may be rendered as a
// visible policy denial by one host and as an absent capability by another.
const defaultRestrictedCommandUnavailableMessage = "command not included in this build"

// restrictionPresentationConfig is a per-Build snapshot. It is deliberately
// private so adding a future presentation knob cannot break downstream
// unkeyed struct literals.
type restrictionPresentationConfig struct {
	enabled               bool
	unavailableMessage    string
	hidePolicyDiagnostics bool
}

func (c restrictionPresentationConfig) effectiveUnavailableMessage() string {
	if c.unavailableMessage != "" {
		return c.unavailableMessage
	}
	return defaultRestrictedCommandUnavailableMessage
}

// RestrictionPresentationOption configures the presentation of commands
// denied by an embedded distribution's Restrict plugin.
//
// Values are accepted only by ConcealRestrictedCommands. The pointed-to
// configuration type is private by design; callers use the constructors in
// this file instead of depending on a public struct layout.
type RestrictionPresentationOption func(*restrictionPresentationConfig)

// ConcealRestrictedCommands opts one command tree into presenting
// plugin-restricted commands as capabilities absent from the distribution.
//
// Restrict remains the enforcement boundary. Without this BuildOption,
// existing Restrict plugins keep their established failed_precondition
// envelope, explicit-help, and completion behavior.
//
// Pass the returned option to Build, or to ExecuteWithOptions when using the
// standard host entrypoint.
func ConcealRestrictedCommands(opts ...RestrictionPresentationOption) BuildOption {
	presentation := restrictionPresentationConfig{enabled: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&presentation)
		}
	}
	return func(cfg *buildConfig) {
		cfg.presentation = presentation
	}
}

// UnavailableMessage customizes the error message for a concealed command.
// An empty message selects the distribution-neutral default.
func UnavailableMessage(message string) RestrictionPresentationOption {
	return func(cfg *restrictionPresentationConfig) {
		cfg.unavailableMessage = message
	}
}

// HidePolicyDiagnostics removes the policy self-inspection commands from a
// concealed distribution. Without it, those commands remain the operator's
// recovery and inspection escape hatch.
func HidePolicyDiagnostics() RestrictionPresentationOption {
	return func(cfg *restrictionPresentationConfig) {
		cfg.hidePolicyDiagnostics = true
	}
}
