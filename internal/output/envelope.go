// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Envelope is the standard success response wrapper.
type Envelope struct {
	OK                 bool                   `json:"ok"`
	Identity           string                 `json:"identity,omitempty"`
	Data               interface{}            `json:"data,omitempty"`
	Meta               *Meta                  `json:"meta,omitempty"`
	ContentSafetyAlert interface{}            `json:"_content_safety_alert,omitempty"`
	Notice             map[string]interface{} `json:"_notice,omitempty"`
}

// ErrorEnvelope is the standard error response wrapper.
//
// Deprecated: ErrorEnvelope belongs to the legacy *output.ExitError surface
// that predates the typed error contract introduced by errs/. New code MUST
// NOT use it — the typed envelope shape is owned by
// internal/output.WriteTypedErrorEnvelope which marshals typed errs.* errors
// directly via JSON reflection (no wrapper struct needed). This struct is
// retained only while existing *ExitError call sites are migrated; it will
// be removed once they have moved to the typed surface.
type ErrorEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    *ErrDetail             `json:"error"`
	Meta     *Meta                  `json:"meta,omitempty"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}

// ErrDetail describes a structured error.
//
// Deprecated: ErrDetail belongs to the legacy *output.ExitError surface that
// predates the typed error contract introduced by errs/. New code MUST NOT
// use it — typed errs.* structs embed errs.Problem and own their wire shape
// via JSON tags (Category, Subtype, Hint, etc. promote to the top level).
// This struct is retained only while existing *ExitError call sites are
// migrated; it will be removed once they have moved to the typed surface.
type ErrDetail struct {
	Type       string      `json:"type"`
	Code       int         `json:"code,omitempty"`
	Message    string      `json:"message"`
	Hint       string      `json:"hint,omitempty"`
	ConsoleURL string      `json:"console_url,omitempty"`
	Risk       *RiskDetail `json:"risk,omitempty"`
	Detail     interface{} `json:"detail,omitempty"`
}

// RiskDetail carries agent-protocol risk information alongside
// confirmation_required errors. Level is one of "read" | "write" |
// "high-risk-write". Action identifies the command for the agent (e.g.
// "mail +send", "drive.files.delete").
//
// Deprecated: RiskDetail is reachable only via *output.ExitError.Detail.Risk,
// part of the legacy envelope surface that predates the typed error contract
// introduced by errs/. New code MUST NOT use it — confirmation-required
// signals belong on *errs.ConfirmationRequiredError (its own typed extension
// fields can carry agent-protocol metadata directly). This struct is
// retained only while existing *ExitError call sites are migrated; it will
// be removed once they have moved to the typed surface.
type RiskDetail struct {
	Level  string `json:"level"`
	Action string `json:"action"`
}

// Meta carries optional metadata in envelope responses.
type Meta struct {
	Count    int    `json:"count,omitempty"`
	Rollback string `json:"rollback,omitempty"`
}

// PendingNotice, if set, returns system-level notices to inject as the
// "_notice" field in JSON output envelopes. Set by cmd/root.go.
// Returns nil when there is nothing to report.
var PendingNotice func() map[string]interface{}

// GetNotice returns the current pending notice for struct-based callers.
// Returns nil when there is nothing to report.
func GetNotice() map[string]interface{} {
	if PendingNotice == nil {
		return nil
	}
	return PendingNotice()
}
