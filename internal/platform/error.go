// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import "fmt"

// PluginInstallError is the typed install-time failure. ReasonCode comes
// from the closed enum in the design doc (section 5.3 reason_code
// table). Cause carries the underlying error, if any, so consumers can
// errors.As to inspect it.
type PluginInstallError struct {
	PluginName string
	ReasonCode string
	Reason     string
	Cause      error
}

func (e *PluginInstallError) Error() string {
	prefix := fmt.Sprintf("plugin %q (%s)", e.PluginName, e.ReasonCode)
	if e.Reason != "" {
		prefix += ": " + e.Reason
	}
	if e.Cause != nil {
		prefix += ": " + e.Cause.Error()
	}
	return prefix
}

func (e *PluginInstallError) Unwrap() error { return e.Cause }

// ReasonCodes for PluginInstallError. The closed enum is referenced by
// the design doc's hard-constraint #15 (reason_code enum closure) and
// drives the JSON envelope's error.detail.reason_code field.
const (
	ReasonInvalidPluginName   = "invalid_plugin_name"
	ReasonPluginNamePanic     = "plugin_name_panic"
	ReasonInvalidHookName     = "invalid_hook_name"
	ReasonDuplicateHookName   = "duplicate_hook_name"
	ReasonInvalidHookRegister = "invalid_hook_registration"
	ReasonInvalidRule         = "invalid_rule"
	ReasonRestrictsMismatch   = "restricts_mismatch"
	ReasonCapabilityUnmet     = "capability_unmet"
	ReasonCapabilitiesPanic   = "capabilities_panic"
	// ReasonInvalidCapability flags a plugin authoring error in
	// Capabilities() output -- e.g. a syntactically malformed
	// RequiredCLIVersion string. This is distinct from
	// ReasonCapabilityUnmet (legitimate version mismatch): an authoring
	// bug must NOT be hidden by FailurePolicy=FailOpen, so this code is
	// classified as untrusted-config and aborts unconditionally.
	ReasonInvalidCapability   = "invalid_capability"
	ReasonInstallFailed       = "install_failed"
	ReasonInstallPanic        = "install_panic"
	ReasonDuplicatePluginName = "duplicate_plugin_name"
	ReasonMultipleRestricts   = "multiple_restrict_plugins"
)
