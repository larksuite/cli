// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"fmt"
	"slices"
)

// formatMessage applies fmt.Sprintf only when args are present, so a
// caller passing a literal message with a stray "%" (e.g. "disk 100% full")
// is not rendered as "%!(NOVERB)". `go vet -printf` catches most accidental
// format misuse upstream; this guard makes the constructor safe even when
// the message string is dynamically composed.
func formatMessage(format string, args []any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// Typed error types and their builder APIs.
//
// Each typed error has:
//   - A struct embedding Problem, with type-specific extension fields
//   - A nil-safe Unwrap() method when the struct carries a Cause field
//   - A NewXxxError(subtype, format, args...) constructor — Category locked
//     by the function name, Subtype + Message positional and required
//   - Chainable WithX(...) setters that return the concrete *XxxError pointer
//     so type-specific setters remain reachable to the end of the chain
//
// Preferred shape for new code:
//
//	return errs.NewValidationError(errs.SubtypeInvalidArgument,
//	    "invalid --start: %v", err).
//	    WithHint("expected RFC3339, e.g. 2026-05-26T10:00:00Z").
//	    WithParam("--start")
//
// Category is locked by the constructor name — it can never be mis-specified
// at the call site. Subtype + Message are required positional arguments so the
// compiler refuses to build a typed error missing either identity field.
// Subtype well-formedness is enforced at PR time by the lint guard
// CheckDeclaredSubtype (`lint/errscontract`), not at runtime, to avoid
// coupling the typed package to a registry. ad_hoc_* subtypes are accepted
// at runtime; CheckAdHocSubtype emits a follow-up warning.

// TypedError is implemented by all typed errors in this package.
// It identifies a value as a typed envelope producer to the dispatcher,
// which uses it to short-circuit promotion when the outer error is
// already typed (avoiding overwrite of producer-set Subtype/Hint).
type TypedError interface {
	error
	ProblemDetail() *Problem
}

// ============================== ValidationError ==============================

// ValidationError is the typed error for CategoryValidation.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type ValidationError struct {
	Problem
	Param  string         `json:"param,omitempty"`
	Params []InvalidParam `json:"params,omitempty"`
	Cause  error          `json:"-"`
}

// InvalidParam is one structured validation diagnostic: the parameter that
// failed (Name) and why (Reason). It mirrors an RFC 7807 "invalid-params"
// item (RFC 7807 §3.1 extension members).
//
// The wire key on ValidationError is "params" rather than "invalid_params"
// because the enclosing envelope already carries type:"validation", so the
// "invalid" qualifier would be redundant on the wire. The Go type keeps the
// InvalidParam prefix because, at package level, the name must self-describe.
type InvalidParam struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
	// Suggestions holds machine-readable, ranked candidate corrections for this
	// parameter (e.g. did-you-mean flags or subcommands), so an agent can retry
	// without parsing the human-facing hint. Omitted when there are none.
	Suggestions []string `json:"suggestions,omitempty"`
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

// Error returns the typed error message. Nil-safe — falls back to "" when the
// receiver is a typed nil pointer, mirroring the embedded Problem.Error() guard
// that promote-through-value-embed would otherwise bypass.
func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

// NewValidationError constructs a *ValidationError with Category locked to
// CategoryValidation and Message formatted via fmt.Sprintf(format, args...).
func NewValidationError(subtype Subtype, format string, args ...any) *ValidationError {
	return &ValidationError{
		Problem: Problem{
			Category: CategoryValidation,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *ValidationError) WithHint(format string, args ...any) *ValidationError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *ValidationError) WithLogID(logID string) *ValidationError {
	e.LogID = logID
	return e
}

func (e *ValidationError) WithCode(code int) *ValidationError {
	e.Code = code
	return e
}

func (e *ValidationError) WithRetryable() *ValidationError {
	e.Retryable = true
	return e
}

func (e *ValidationError) WithParam(param string) *ValidationError {
	e.Param = param
	return e
}

func (e *ValidationError) WithParams(params ...InvalidParam) *ValidationError {
	e.Params = append(e.Params, params...)
	return e
}

func (e *ValidationError) WithCause(cause error) *ValidationError {
	e.Cause = cause
	return e
}

// =========================== AuthenticationError =============================

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

// Error is nil-receiver safe; see ValidationError.Error.
func (e *AuthenticationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewAuthenticationError(subtype Subtype, format string, args ...any) *AuthenticationError {
	return &AuthenticationError{
		Problem: Problem{
			Category: CategoryAuthentication,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *AuthenticationError) WithHint(format string, args ...any) *AuthenticationError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *AuthenticationError) WithLogID(logID string) *AuthenticationError {
	e.LogID = logID
	return e
}

func (e *AuthenticationError) WithCode(code int) *AuthenticationError {
	e.Code = code
	return e
}

func (e *AuthenticationError) WithRetryable() *AuthenticationError {
	e.Retryable = true
	return e
}

func (e *AuthenticationError) WithUserOpenID(id string) *AuthenticationError {
	e.UserOpenID = id
	return e
}

func (e *AuthenticationError) WithCause(cause error) *AuthenticationError {
	e.Cause = cause
	return e
}

// ============================= PermissionError ===============================

// PermissionError is the typed error for CategoryAuthorization.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type PermissionError struct {
	Problem
	MissingScopes   []string `json:"missing_scopes,omitempty"`
	RequestedScopes []string `json:"requested_scopes,omitempty"`
	GrantedScopes   []string `json:"granted_scopes,omitempty"`
	Identity        string   `json:"identity,omitempty"`
	ConsoleURL      string   `json:"console_url,omitempty"`
	Cause           error    `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *PermissionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Error is nil-receiver safe; see ValidationError.Error.
func (e *PermissionError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewPermissionError(subtype Subtype, format string, args ...any) *PermissionError {
	return &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *PermissionError) WithHint(format string, args ...any) *PermissionError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *PermissionError) WithLogID(logID string) *PermissionError {
	e.LogID = logID
	return e
}

func (e *PermissionError) WithCode(code int) *PermissionError {
	e.Code = code
	return e
}

func (e *PermissionError) WithRetryable() *PermissionError {
	e.Retryable = true
	return e
}

func (e *PermissionError) WithMissingScopes(scopes ...string) *PermissionError {
	e.MissingScopes = slices.Clone(scopes)
	return e
}

func (e *PermissionError) WithRequestedScopes(scopes ...string) *PermissionError {
	e.RequestedScopes = slices.Clone(scopes)
	return e
}

func (e *PermissionError) WithGrantedScopes(scopes ...string) *PermissionError {
	e.GrantedScopes = slices.Clone(scopes)
	return e
}

func (e *PermissionError) WithIdentity(identity string) *PermissionError {
	e.Identity = identity
	return e
}

func (e *PermissionError) WithConsoleURL(url string) *PermissionError {
	e.ConsoleURL = url
	return e
}

func (e *PermissionError) WithCause(cause error) *PermissionError {
	e.Cause = cause
	return e
}

// =============================== ConfigError =================================

// ConfigError is the typed error for CategoryConfig. Cause preserves an
// optional wrapped sentinel for errors.Is / errors.Unwrap; it is
// intentionally not serialized.
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

// Error is nil-receiver safe; see ValidationError.Error.
func (e *ConfigError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewConfigError(subtype Subtype, format string, args ...any) *ConfigError {
	return &ConfigError{
		Problem: Problem{
			Category: CategoryConfig,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *ConfigError) WithHint(format string, args ...any) *ConfigError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *ConfigError) WithLogID(logID string) *ConfigError {
	e.LogID = logID
	return e
}

func (e *ConfigError) WithCode(code int) *ConfigError {
	e.Code = code
	return e
}

func (e *ConfigError) WithRetryable() *ConfigError {
	e.Retryable = true
	return e
}

func (e *ConfigError) WithField(field string) *ConfigError {
	e.Field = field
	return e
}

func (e *ConfigError) WithCause(cause error) *ConfigError {
	e.Cause = cause
	return e
}

// =============================== NetworkError ================================

// NetworkError is the typed error for CategoryNetwork. The Subtype carries
// the failure taxonomy: timeout / tls / dns / server_error, with transport
// as the fallback. Cause preserves an optional wrapped sentinel for
// errors.Is / errors.Unwrap; it is intentionally not serialized.
type NetworkError struct {
	Problem
	Cause error `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *NetworkError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Error is nil-receiver safe; see ValidationError.Error.
func (e *NetworkError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewNetworkError(subtype Subtype, format string, args ...any) *NetworkError {
	return &NetworkError{
		Problem: Problem{
			Category: CategoryNetwork,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *NetworkError) WithHint(format string, args ...any) *NetworkError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *NetworkError) WithLogID(logID string) *NetworkError {
	e.LogID = logID
	return e
}

func (e *NetworkError) WithCode(code int) *NetworkError {
	e.Code = code
	return e
}

func (e *NetworkError) WithRetryable() *NetworkError {
	e.Retryable = true
	return e
}

func (e *NetworkError) WithCause(cause error) *NetworkError {
	e.Cause = cause
	return e
}

// ================================ APIError ===================================

// APIError is the typed error for CategoryAPI (catch-all for classified Lark
// API business errors). Cause preserves an optional wrapped sentinel for
// errors.Is / errors.Unwrap; it is intentionally not serialized.
type APIError struct {
	Problem
	Cause error `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Error is nil-receiver safe; see ValidationError.Error.
func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewAPIError(subtype Subtype, format string, args ...any) *APIError {
	return &APIError{
		Problem: Problem{
			Category: CategoryAPI,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *APIError) WithHint(format string, args ...any) *APIError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *APIError) WithLogID(logID string) *APIError {
	e.LogID = logID
	return e
}

func (e *APIError) WithCode(code int) *APIError {
	e.Code = code
	return e
}

func (e *APIError) WithRetryable() *APIError {
	e.Retryable = true
	return e
}

func (e *APIError) WithCause(cause error) *APIError {
	e.Cause = cause
	return e
}

// =========================== SecurityPolicyError =============================

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

// Error is nil-receiver safe; see ValidationError.Error.
func (e *SecurityPolicyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewSecurityPolicyError(subtype Subtype, format string, args ...any) *SecurityPolicyError {
	return &SecurityPolicyError{
		Problem: Problem{
			Category: CategoryPolicy,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *SecurityPolicyError) WithHint(format string, args ...any) *SecurityPolicyError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *SecurityPolicyError) WithLogID(logID string) *SecurityPolicyError {
	e.LogID = logID
	return e
}

func (e *SecurityPolicyError) WithCode(code int) *SecurityPolicyError {
	e.Code = code
	return e
}

func (e *SecurityPolicyError) WithRetryable() *SecurityPolicyError {
	e.Retryable = true
	return e
}

func (e *SecurityPolicyError) WithChallengeURL(url string) *SecurityPolicyError {
	e.ChallengeURL = url
	return e
}

func (e *SecurityPolicyError) WithCause(cause error) *SecurityPolicyError {
	e.Cause = cause
	return e
}

// ============================ ContentSafetyError =============================

// ContentSafetyError is the typed error for CategoryPolicy content-safety subtypes.
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type ContentSafetyError struct {
	Problem
	Rules []string `json:"rules,omitempty"`
	Cause error    `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *ContentSafetyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Error is nil-receiver safe; see ValidationError.Error.
func (e *ContentSafetyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewContentSafetyError(subtype Subtype, format string, args ...any) *ContentSafetyError {
	return &ContentSafetyError{
		Problem: Problem{
			Category: CategoryPolicy,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *ContentSafetyError) WithHint(format string, args ...any) *ContentSafetyError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *ContentSafetyError) WithLogID(logID string) *ContentSafetyError {
	e.LogID = logID
	return e
}

func (e *ContentSafetyError) WithCode(code int) *ContentSafetyError {
	e.Code = code
	return e
}

func (e *ContentSafetyError) WithRetryable() *ContentSafetyError {
	e.Retryable = true
	return e
}

func (e *ContentSafetyError) WithRules(rules ...string) *ContentSafetyError {
	e.Rules = slices.Clone(rules)
	return e
}

func (e *ContentSafetyError) WithCause(cause error) *ContentSafetyError {
	e.Cause = cause
	return e
}

// =============================== InternalError ===============================

// InternalError is the typed error for CategoryInternal. Cause is preserved
// for logging but not emitted on the wire.
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

// Error is nil-receiver safe; see ValidationError.Error.
func (e *InternalError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

func NewInternalError(subtype Subtype, format string, args ...any) *InternalError {
	return &InternalError{
		Problem: Problem{
			Category: CategoryInternal,
			Subtype:  subtype,
			Message:  formatMessage(format, args),
		},
	}
}

func (e *InternalError) WithHint(format string, args ...any) *InternalError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *InternalError) WithLogID(logID string) *InternalError {
	e.LogID = logID
	return e
}

func (e *InternalError) WithCode(code int) *InternalError {
	e.Code = code
	return e
}

func (e *InternalError) WithRetryable() *InternalError {
	e.Retryable = true
	return e
}

func (e *InternalError) WithCause(cause error) *InternalError {
	e.Cause = cause
	return e
}

// ========================= ConfirmationRequiredError =========================

// Risk classifies the impact of a confirmation-required operation. Every
// ConfirmationRequiredError MUST populate Risk; callers without a known
// risk level use RiskUnknown so the envelope is never wire-invalid.
const (
	RiskRead          = "read"
	RiskWrite         = "write"
	RiskHighRiskWrite = "high-risk-write"
	RiskUnknown       = "unknown"
)

// ConfirmationRequiredError is the typed error for CategoryConfirmation.
// Risk is one of: "read" | "write" | "high-risk-write" | "unknown".
// Cause preserves an optional wrapped sentinel for errors.Is / errors.Unwrap;
// it is intentionally not serialized.
type ConfirmationRequiredError struct {
	Problem
	Risk   string `json:"risk"`
	Action string `json:"action"`
	Cause  error  `json:"-"`
}

// Unwrap is nil-receiver safe; see ValidationError.Unwrap.
func (e *ConfirmationRequiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Error is nil-receiver safe; see ValidationError.Error.
func (e *ConfirmationRequiredError) Error() string {
	if e == nil {
		return ""
	}
	return e.Problem.Error()
}

// NewConfirmationRequiredError constructs a *ConfirmationRequiredError.
// Risk + Action are wire-required (non-omitempty). Empty inputs are
// normalized at the constructor boundary so callers cannot build a
// wire-invalid envelope: risk falls back to RiskUnknown, action to
// "unknown". risk is one of: "read" | "write" | "high-risk-write".
func NewConfirmationRequiredError(risk, action, format string, args ...any) *ConfirmationRequiredError {
	if risk == "" {
		risk = RiskUnknown
	}
	if action == "" {
		action = "unknown"
	}
	return &ConfirmationRequiredError{
		Problem: Problem{
			Category: CategoryConfirmation,
			Subtype:  SubtypeConfirmationRequired,
			Message:  formatMessage(format, args),
		},
		Risk:   risk,
		Action: action,
	}
}

func (e *ConfirmationRequiredError) WithHint(format string, args ...any) *ConfirmationRequiredError {
	e.Hint = formatMessage(format, args)
	return e
}

func (e *ConfirmationRequiredError) WithLogID(logID string) *ConfirmationRequiredError {
	e.LogID = logID
	return e
}

func (e *ConfirmationRequiredError) WithCode(code int) *ConfirmationRequiredError {
	e.Code = code
	return e
}

func (e *ConfirmationRequiredError) WithCause(cause error) *ConfirmationRequiredError {
	e.Cause = cause
	return e
}
