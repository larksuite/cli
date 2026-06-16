// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Subtype is the second-level taxonomy axis. Wire JSON: "subtype".
type Subtype string

const (
	SubtypeUnknown Subtype = "unknown" // catch-all fallback; producers must prefer a specific subtype
)

// CategoryValidation subtypes
const (
	SubtypeInvalidArgument    Subtype = "invalid_argument"    // user-supplied flag / arg failed validation (gRPC INVALID_ARGUMENT alignment)
	SubtypeFailedPrecondition Subtype = "failed_precondition" // request is valid but the system/resource state is not in the state required to execute; caller must change state (not retry) — e.g. ambiguous remote mapping (gRPC FAILED_PRECONDITION alignment)
)

// CategoryAuthentication subtypes
const (
	SubtypeTokenMissing        Subtype = "token_missing"         // no token in request (Authorization header absent / no local token cache)
	SubtypeTokenInvalid        Subtype = "token_invalid"         // token present but content/format wrong
	SubtypeTokenExpired        Subtype = "token_expired"         // token explicitly expired
	SubtypeRefreshTokenInvalid Subtype = "refresh_token_invalid" // refresh_token is v1 legacy format, unusable
	SubtypeRefreshTokenExpired Subtype = "refresh_token_expired" // refresh_token expired
	SubtypeRefreshTokenRevoked Subtype = "refresh_token_revoked" // refresh_token revoked (user logout / admin action)
	SubtypeRefreshTokenReused  Subtype = "refresh_token_reused"  // refresh_token already used (single-use rotation triggered)
	SubtypeRefreshServerError  Subtype = "refresh_server_error"  // refresh endpoint transient error (retryable)
)

// CategoryAuthorization subtypes
const (
	SubtypeMissingScope           Subtype = "missing_scope"            // user authorized app but did not grant this scope
	SubtypeUserUnauthorized       Subtype = "user_unauthorized"        // user never authorized the app
	SubtypeAppScopeNotApplied     Subtype = "app_scope_not_applied"    // app did not apply for this scope on the open platform
	SubtypeTokenScopeInsufficient Subtype = "token_scope_insufficient" // token was issued without this scope (RFC 6750 alignment)
	SubtypeAppUnavailable         Subtype = "app_unavailable"          // app status unavailable
	SubtypeAppDisabled            Subtype = "app_disabled"             // app currently disabled in this tenant (was installed/enabled before)
	SubtypePermissionDenied       Subtype = "permission_denied"        // resource-level permission denial (authenticated but lacks rights for this resource, HTTP 403 / gRPC PERMISSION_DENIED alignment)
)

// CategoryConfig subtypes
const (
	SubtypeInvalidClient Subtype = "invalid_client" // app_id / app_secret incorrect (RFC 6749 §5.2 alignment)
	SubtypeNotConfigured Subtype = "not_configured" // local config file absent (user has not run `config init`)
	SubtypeInvalidConfig Subtype = "invalid_config" // local config file present but malformed
)

// CategoryNetwork subtypes
const (
	SubtypeNetworkTransport Subtype = "transport"    // fallback when no more-specific network subtype matches
	SubtypeNetworkTimeout   Subtype = "timeout"      // dial / read timeout
	SubtypeNetworkTLS       Subtype = "tls"          // TLS handshake / cert failure
	SubtypeNetworkDNS       Subtype = "dns"          // DNS resolution failure
	SubtypeNetworkServer    Subtype = "server_error" // upstream HTTP 5xx
)

// CategoryAPI subtypes
const (
	SubtypeRateLimit         Subtype = "rate_limit"         // request rate limit exceeded
	SubtypeConflict          Subtype = "conflict"           // resource state conflict (e.g. concurrent modification)
	SubtypeCrossTenant       Subtype = "cross_tenant"       // operation crosses tenant boundary (not supported)
	SubtypeCrossBrand        Subtype = "cross_brand"        // operation crosses brand boundary (feishu vs lark, not supported)
	SubtypeInvalidParameters Subtype = "invalid_parameters" // API-side parameter validation rejected the request
	SubtypeOwnershipMismatch Subtype = "ownership_mismatch" // caller is not the resource owner
	SubtypeNotFound          Subtype = "not_found"          // referenced resource does not exist (HTTP 404 alignment)
	SubtypeServerError       Subtype = "server_error"       // upstream server-side transient error (HTTP 5xx alignment, retryable)
	SubtypeQuotaExceeded     Subtype = "quota_exceeded"     // resource quota / collection size limit reached (assignees, followers, members, etc.)
	SubtypeAlreadyExists     Subtype = "already_exists"     // idempotency violation: resource already exists in target state
)

// CategoryPolicy subtypes (security-policy envelope shape)
const (
	SubtypeChallengeRequired Subtype = "challenge_required" // user must complete browser challenge / MFA
	SubtypeAccessDenied      Subtype = "access_denied"      // policy denies access outright
	SubtypeContentSafety     Subtype = "content_safety"     // content-safety scanner blocked output in block mode
)

// CategoryInternal subtypes
const (
	SubtypeSDKError        Subtype = "sdk_error"        // lark SDK Do() returned an unexpected error
	SubtypeInvalidResponse Subtype = "invalid_response" // SDK response body not parsable as JSON
	SubtypeFileIO          Subtype = "file_io"          // local file I/O failure (mkdir / write / read)
	SubtypeExternalTool    Subtype = "external_tool"    // an external tool the CLI shells out to (git, npx) failed at runtime; the tool output is in the message
	SubtypeStorage         Subtype = "storage"          // local persistence failure (e.g. config file save)
	// Generic untyped error lifted to InternalError uses SubtypeUnknown.
)

// CategoryConfirmation subtypes
const (
	SubtypeConfirmationRequired Subtype = "confirmation_required" // high-risk operation needs explicit --yes
)
