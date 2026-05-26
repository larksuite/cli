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
	SubtypeInvalidArgument Subtype = "invalid_argument" // user-supplied flag / arg failed validation (gRPC INVALID_ARGUMENT alignment)
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
	SubtypeAppNotInstalled        Subtype = "app_not_installed"        // app not enabled / not installed in this tenant
)

// CategoryConfig subtypes
const (
	SubtypeInvalidClient Subtype = "invalid_client" // app_id / app_secret incorrect (RFC 6749 §5.2 alignment)
	SubtypeNotConfigured Subtype = "not_configured" // local config file absent (user has not run `config init`)
	SubtypeInvalidConfig Subtype = "invalid_config" // local config file present but malformed
)

// CategoryNetwork subtypes
const (
	SubtypeNetworkTransport Subtype = "transport" // transport-layer failure (timeout / TLS / DNS / 5xx); see NetworkError.CauseKind
)

// CategoryAPI subtypes
const (
	SubtypeRateLimit         Subtype = "rate_limit"         // request rate limit exceeded
	SubtypeConflict          Subtype = "conflict"           // resource state conflict (e.g. concurrent modification)
	SubtypeCrossTenant       Subtype = "cross_tenant"       // operation crosses tenant boundary (not supported)
	SubtypeCrossBrand        Subtype = "cross_brand"        // operation crosses brand boundary (feishu vs lark, not supported)
	SubtypeInvalidParameters Subtype = "invalid_parameters" // API-side parameter validation rejected the request
	SubtypeOwnershipMismatch Subtype = "ownership_mismatch" // caller is not the resource owner
)

// CategoryPolicy subtypes (security-policy envelope shape)
const (
	SubtypeChallengeRequired Subtype = "challenge_required" // user must complete browser challenge / MFA
	SubtypeAccessDenied      Subtype = "access_denied"      // policy denies access outright
)

// CategoryInternal subtypes
const (
	SubtypeSDKError        Subtype = "sdk_error"        // lark SDK Do() returned an unexpected error
	SubtypeInvalidResponse Subtype = "invalid_response" // SDK response body not parsable as JSON
	// Generic untyped error lifted to InternalError uses SubtypeUnknown.
)

// CategoryConfirmation subtypes intentionally have no declarations yet.
