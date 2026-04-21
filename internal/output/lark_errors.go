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

	// Sheets float image: width/height/offset out of range or invalid.
	LarkErrSheetsFloatImageInvalidDims = 1310246

	// Drive permission apply: per-user-per-document submission limit (5/day) reached.
	LarkErrDrivePermApplyRateLimit = 1063006
	// Drive permission apply: request is not applicable for this document
	// (e.g. the document is configured to disallow access requests, or the
	// caller already holds the requested permission, or the target type does
	// not accept apply operations).
	LarkErrDrivePermApplyNotApplicable = 1063007
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

	// sheets-specific constraints that benefit from actionable hints
	case LarkErrSheetsFloatImageInvalidDims:
		return ExitAPI, "invalid_params",
			"check --width / --height / --offset-x / --offset-y: " +
				"width/height must be >= 20 px; offsets must be >= 0 and less than the anchor cell's width/height"

	// drive permission-apply specific guidance
	case LarkErrDrivePermApplyRateLimit:
		return ExitAPI, "rate_limit",
			"permission-apply quota reached: each user may request access on the same document at most 5 times per day; wait or ask the owner directly"
	case LarkErrDrivePermApplyNotApplicable:
		return ExitAPI, "invalid_params",
			"this document does not accept a permission-apply request (common causes: the document is configured to disallow access requests, the caller already holds the permission, or the target type does not support apply); contact the owner directly"
	}

	return ExitAPI, "api_error", ""
}
