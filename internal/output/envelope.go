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
type ErrorEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    *ErrDetail             `json:"error"`
	Meta     *Meta                  `json:"meta,omitempty"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}

// ErrDetail describes a structured error.
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
