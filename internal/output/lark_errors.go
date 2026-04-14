// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Lark API generic error code constants.
// ref: https://open.feishu.cn/document/server-docs/api-call-guide/generic-error-code
const (
	// Auth: token missing / invalid / expired.
	LarkErrTokenMissing = 99991661 // Authorization header missing or empty
	LarkErrTokenBadFmt  = 99991671 // token format error (must start with "t-" or "u-")
	LarkErrTokenInvalid = 99991668 // user_access_token invalid or expired
	LarkErrATInvalid    = 99991663 // access_token invalid (generic)
	LarkErrTokenExpired = 99991677 // user_access_token expired, refresh to obtain a new one

	// Permission: scope not granted.
	LarkErrAppScopeNotEnabled    = 99991672 // app has not applied for the required API scope
	LarkErrTokenNoPermission     = 99991676 // token lacks the required scope
	LarkErrUserScopeInsufficient = 99991679 // user has not granted the required scope
	LarkErrUserNotAuthorized     = 230027   // user not authorized

	// App credential / status.
	LarkErrAppCredInvalid  = 99991543 // app_id or app_secret is incorrect
	LarkErrAppNotInUse     = 99991662 // app is disabled or not installed in this tenant
	LarkErrAppUnauthorized = 99991673 // app status unavailable; check installation

	// Rate limit.
	LarkErrRateLimit = 99991400 // request frequency limit exceeded

	// Refresh token errors (authn service).
	LarkErrRefreshInvalid     = 20026 // refresh_token invalid or v1 format
	LarkErrRefreshExpired     = 20037 // refresh_token expired
	LarkErrRefreshRevoked     = 20064 // refresh_token revoked
	LarkErrRefreshAlreadyUsed = 20073 // refresh_token already consumed (single-use rotation)
	LarkErrRefreshServerError = 20050 // refresh endpoint server-side error, retryable

	// Drive shortcut / cross-space constraints.
	LarkErrDriveResourceContention = 1061045 // resource contention occurred, please retry
	LarkErrDriveCrossTenantUnit    = 1064510 // cross tenant and unit not support
	LarkErrDriveCrossBrand         = 1064511 // cross brand not support
)

// ClassifyLarkError maps a Lark API error code + message to (exitCode, errType, hint).
// errType provides fine-grained classification in the JSON envelope;
// exitCode is kept coarse (ExitAuth or ExitAPI).
func ClassifyLarkError(code int, msg string) (int, string, string) {
	switch code {
	// auth: token missing / invalid / expired
	case LarkErrTokenMissing, LarkErrTokenBadFmt:
		return ExitAuth, "auth", "run: lark-cli auth login to re-authorize"
	case LarkErrTokenInvalid, LarkErrATInvalid, LarkErrTokenExpired:
		return ExitAuth, "auth", "run: lark-cli auth login to re-authorize"

	// permission: scope not granted
	case LarkErrAppScopeNotEnabled, LarkErrTokenNoPermission,
		LarkErrUserScopeInsufficient, LarkErrUserNotAuthorized:
		return ExitAPI, "permission", "check app permissions or re-authorize: lark-cli auth login"

	// app credential / status
	case LarkErrAppCredInvalid:
		return ExitAuth, "config", "check app_id / app_secret: lark-cli config set"
	case LarkErrAppNotInUse, LarkErrAppUnauthorized:
		return ExitAuth, "app_status", "app is disabled or not installed — check developer console"

	// rate limit
	case LarkErrRateLimit:
		return ExitAPI, "rate_limit", "please try again later"

	// drive-specific constraints that benefit from actionable hints
	case LarkErrDriveResourceContention:
		return ExitAPI, "conflict", "please retry later and avoid concurrent duplicate requests"
	case LarkErrDriveCrossTenantUnit:
		return ExitAPI, "cross_tenant_unit", "operate on source and target within the same tenant and region/unit"
	case LarkErrDriveCrossBrand:
		return ExitAPI, "cross_brand", "operate on source and target within the same brand environment"
	}

	return ExitAPI, "api_error", ""
}
