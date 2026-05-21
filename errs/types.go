// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// ValidationError is the typed error for CategoryValidation.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type ValidationError struct {
	Problem
	Param string `json:"param,omitempty"`
	Cause error  `json:"-"`
}

// Unwrap exposes the wrapped cause so errors.Unwrap / errors.Is can traverse
// it. A nil typed-pointer held inside an error interface is treated as
// "no cause" so callers cannot panic on `errors.Unwrap(err)`.
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AuthenticationError is the typed error for CategoryAuthentication.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type AuthenticationError struct {
	Problem
	UserOpenID string `json:"user_open_id,omitempty"`
	Cause      error  `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *AuthenticationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// PermissionError is the typed error for CategoryAuthorization.
type PermissionError struct {
	Problem
	MissingScopes []string `json:"missing_scopes,omitempty"`
	Identity      string   `json:"identity,omitempty"`
	ConsoleURL    string   `json:"console_url,omitempty"`
}

// ConfigError is the typed error for CategoryConfig.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type ConfigError struct {
	Problem
	Field string `json:"field,omitempty"`
	Cause error  `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *ConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NetworkError is the typed error for CategoryNetwork.
// CauseKind (string) is one of: "timeout" | "tls" | "dns" | "5xx" — the
// canonical wire taxonomy (emitted as JSON key "cause"). Cause preserves an
// optional wrapped sentinel for errors.Is / errors.Unwrap; it is intentionally
// not serialized.
type NetworkError struct {
	Problem
	CauseKind string `json:"cause,omitempty"`
	Cause     error  `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *NetworkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// APIError is the typed error for CategoryAPI (catch-all for classified Lark API
// business errors). Detail preserves the raw Lark error map for diagnostics.
type APIError struct {
	Problem
	Detail map[string]any `json:"detail,omitempty"`
}

// SecurityPolicyError is the typed error for CategoryPolicy security-policy subtypes.
// Subtype is "challenge_required" or "access_denied"; Code is 21000 or 21001.
type SecurityPolicyError struct {
	Problem
	ChallengeURL string `json:"challenge_url,omitempty"`
	Cause        error  `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *SecurityPolicyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ContentSafetyError is the typed error for CategoryPolicy content-safety subtypes.
type ContentSafetyError struct {
	Problem
	Rules []string `json:"rules,omitempty"`
}

// InternalError is the typed error for CategoryInternal.
// Cause is preserved for logging but not emitted on the wire.
type InternalError struct {
	Problem
	Cause error `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *InternalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ConfirmationRequiredError is the typed error for CategoryConfirmation.
// Risk is one of: "read" | "write" | "high-risk-write".
type ConfirmationRequiredError struct {
	Problem
	Risk   string `json:"risk"`
	Action string `json:"action"`
}
