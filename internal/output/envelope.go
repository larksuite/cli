// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Envelope is the standard success response wrapper.
type Envelope struct {
	OK       bool        `json:"ok"`
	Identity string      `json:"identity,omitempty"`
	Data     interface{} `json:"data,omitempty"`
	Meta     *Meta       `json:"meta,omitempty"`
}

// ErrorEnvelope is the standard error response wrapper.
type ErrorEnvelope struct {
	OK       bool       `json:"ok"`
	Identity string     `json:"identity,omitempty"`
	Error    *ErrDetail `json:"error"`
	Meta     *Meta      `json:"meta,omitempty"`
}

// ErrDetail describes a structured error.
type ErrDetail struct {
	RetryAfterMS int64       `json:"retry_after_ms,omitempty"`
	HTTPStatus   int         `json:"http_status,omitempty"`
	LogID        string      `json:"log_id,omitempty"`
	Retryable    *bool       `json:"retryable,omitempty"`
	NextAction   string      `json:"next_action,omitempty"`
	Type         string      `json:"type"`
	Code         int         `json:"code,omitempty"`
	Message      string      `json:"message"`
	Hint         string      `json:"hint,omitempty"`
	ConsoleURL   string      `json:"console_url,omitempty"`
	Detail       interface{} `json:"detail,omitempty"`
}

// Meta carries optional metadata in envelope responses.
type Meta struct {
	Count    int    `json:"count,omitempty"`
	Rollback string `json:"rollback,omitempty"`
}
