// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

// Lark API generic error code constants.
// ref: https://open.feishu.cn/document/server-docs/api-call-guide/generic-error-code
//
// Kept as exported identifiers because external shortcut packages reference
// them by name (e.g. LarkErrOwnershipMismatch). The canonical Category /
// Subtype / Retryable metadata for each code lives in internal/errclass and
// must remain the single source of truth — ClassifyLarkError below resolves
// classification through errclass.LookupCodeMeta.
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

	// Drive shortcut / cross-space constraints.
	LarkErrDriveResourceContention = 1061045 // resource contention occurred, please retry
	LarkErrDriveCrossTenantUnit    = 1064510 // cross tenant and unit not support
	LarkErrDriveCrossBrand         = 1064511 // cross brand not support

	// Wiki write-path lock contention (e.g. concurrent wiki +node-create under the
	// same parent). Server-side write lock; transient, safe to retry with backoff.
	LarkErrWikiLockContention = 131009

	// Sheets float image: width/height/offset out of range or invalid.
	LarkErrSheetsFloatImageInvalidDims = 1310246

	// Drive permission apply: per-user-per-document submission limit (5/day) reached.
	LarkErrDrivePermApplyRateLimit = 1063006
	// Drive permission apply: request is not applicable for this document
	// (e.g. the document is configured to disallow access requests, or the
	// caller already holds the requested permission, or the target type does
	// not accept apply operations).
	LarkErrDrivePermApplyNotApplicable = 1063007

	// IM resource ownership mismatch.
	LarkErrOwnershipMismatch = 231205

	// Mail send: account / mailbox-level failures returned by
	// POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/drafts/:draft_id/send.
	// Mail v1 uses service-scoped 123xxxx codes; keep the full upstream code
	// because ErrAPI preserves Detail.Code exactly as returned by the server.
	// These codes indicate the entire batch will keep failing identically and
	// are consumed by shortcuts/mail.isFatalSendErr to abort early.
	LarkErrMailboxNotFound        = 1234013 // mailbox not found or not active
	LarkErrMailSendQuotaUser      = 1236007 // user daily send count exceeded
	LarkErrMailSendQuotaUserExt   = 1236008 // user daily external recipient count exceeded
	LarkErrMailSendQuotaTenantExt = 1236009 // tenant daily external recipient count exceeded
	LarkErrMailQuota              = 1236010 // mail quota limit
	LarkErrTenantStorageLimit     = 1236013 // tenant storage limit exceeded
)

// legacyHints supplies the per-code actionable hint string for the legacy
// (exitCode, errType, hint) tuple returned by ClassifyLarkError. Hint
// composition is not yet centralized in errclass (the canonical
// PermissionHint lives there but the long-form per-code hints below are
// still wire-stable strings), so this small lookup remains here. Codes
// absent from this map fall back to "".
var legacyHints = map[int]string{
	LarkErrTokenMissing: "run: lark-cli auth login to re-authorize",
	LarkErrTokenBadFmt:  "run: lark-cli auth login to re-authorize",
	LarkErrTokenInvalid: "run: lark-cli auth login to re-authorize",
	LarkErrATInvalid:    "run: lark-cli auth login to re-authorize",
	LarkErrTokenExpired: "run: lark-cli auth login to re-authorize",

	LarkErrAppScopeNotEnabled:    "check app permissions or re-authorize: lark-cli auth login",
	LarkErrTokenNoPermission:     "check app permissions or re-authorize: lark-cli auth login",
	LarkErrUserScopeInsufficient: "check app permissions or re-authorize: lark-cli auth login",
	LarkErrUserNotAuthorized:     "check app permissions or re-authorize: lark-cli auth login",

	LarkErrAppCredInvalid:  "check app_id / app_secret: lark-cli config set",
	LarkErrAppNotInUse:     "app is disabled or not installed — check developer console",
	LarkErrAppUnauthorized: "app is disabled or not installed — check developer console",

	LarkErrRateLimit:               "please try again later",
	LarkErrDriveResourceContention: "please retry later and avoid concurrent duplicate requests",
	LarkErrWikiLockContention:      "wiki write lock contention on this parent node; retry with exponential backoff or serialize sibling-node writes",
	LarkErrDriveCrossTenantUnit:    "operate on source and target within the same tenant and region/unit",
	LarkErrDriveCrossBrand:         "operate on source and target within the same brand environment",
	LarkErrSheetsFloatImageInvalidDims: "check --width / --height / --offset-x / --offset-y: " +
		"width/height must be >= 20 px; offsets must be >= 0 and less than the anchor cell's width/height",
	LarkErrDrivePermApplyRateLimit:     "permission-apply quota reached: each user may request access on the same document at most 5 times per day; wait or ask the owner directly",
	LarkErrDrivePermApplyNotApplicable: "this document does not accept a permission-apply request (common causes: the document is configured to disallow access requests, the caller already holds the permission, or the target type does not support apply); contact the owner directly",
}

// ClassifyLarkError maps a Lark API error code + message to the legacy
// (exitCode, errType, hint) tuple consumed by the *ExitError path.
//
// Classification (Category / Subtype) is sourced from
// errclass.LookupCodeMeta — the single source of truth shipped for both
// this legacy adapter and the stage-2+ typed pipeline (errclass.BuildAPIError,
// not yet invoked in production). This function adapts that result back to
// the legacy tuple shape for callers that still go through *ExitError:
//
//   - exitCode: derived from (Category, Subtype) via legacyExitCode below.
//     Note this differs from the typed pipeline's ExitCodeForCategory in
//     two preserved-legacy-quirks: Authorization+permission subtypes return
//     ExitAPI (legacy treats "permission" as exit 1) and Config returns
//     ExitAuth (legacy bundles "check app_id/secret" under exit 3).
//   - errType: legacy short string per (Category, Subtype), mapped by
//     legacyErrType. Subtypes not present in the legacy taxonomy fall back
//     to "api_error".
//   - hint: per-code lookup in legacyHints; "" when absent.
//
// Unknown codes (LookupCodeMeta returns false) classify as
// (ExitAPI, "api_error", "") — matching the prior default.
//
// Deprecated: ClassifyLarkError belongs to the legacy *output.ExitError
// surface that predates the typed error contract introduced by errs/. New
// code MUST NOT use it — classify Lark API responses via
// internal/errclass.BuildAPIError, which emits a typed *errs.XxxError with
// Category, Subtype, and identity-aware extension fields populated at the
// source. This helper is retained only while existing call sites are
// migrated; it will be removed once they have moved to the typed surface.
func ClassifyLarkError(code int, msg string) (int, string, string) {
	meta, ok := errclass.LookupCodeMeta(code)
	if !ok {
		return ExitAPI, "api_error", ""
	}
	exitCode := legacyExitCode(meta.Category, meta.Subtype)
	errType := legacyErrType(meta.Category, meta.Subtype)
	hint := legacyHints[code]
	// IM ownership mismatch keeps its dynamic recovery hint.
	if code == LarkErrOwnershipMismatch {
		hint = buildOwnershipRecoveryHint()
	}
	return exitCode, errType, hint
}

// legacyExitCode maps (Category, Subtype) to the legacy *ExitError exit
// code. It diverges from ExitCodeForCategory in two places to preserve the
// historic wire:
//
//   - CategoryAuthorization with a "permission" subtype (missing_scope,
//     app_scope_not_enabled, token_no_permission) → ExitAPI (1), not
//     ExitAuth (3). Legacy considered permission failures a generic API
//     refusal.
//   - CategoryConfig → ExitAuth (3). Legacy bundled "check app_id/secret"
//     under the auth bucket.
func legacyExitCode(cat errs.Category, sub errs.Subtype) int {
	switch cat {
	case errs.CategoryAuthentication:
		return ExitAuth
	case errs.CategoryAuthorization:
		switch sub {
		case errs.SubtypeMissingScope,
			errs.SubtypeUserUnauthorized,
			errs.SubtypeAppScopeNotApplied,
			errs.SubtypeTokenScopeInsufficient:
			return ExitAPI
		case errs.SubtypeAppUnavailable,
			errs.SubtypeAppNotInstalled:
			return ExitAuth
		}
		return ExitAPI
	case errs.CategoryConfig:
		return ExitAuth
	}
	return ExitAPI
}

// legacyErrType maps (Category, Subtype) to the legacy *ExitError errType
// string (e.g. "permission", "rate_limit"). Subtypes outside the
// historically-classified set fall back to "api_error", matching the prior
// default-case behavior.
func legacyErrType(cat errs.Category, sub errs.Subtype) string {
	switch cat {
	case errs.CategoryAuthentication:
		return "auth"
	case errs.CategoryAuthorization:
		switch sub {
		case errs.SubtypeMissingScope,
			errs.SubtypeUserUnauthorized,
			errs.SubtypeAppScopeNotApplied,
			errs.SubtypeTokenScopeInsufficient:
			return "permission"
		case errs.SubtypeAppUnavailable,
			errs.SubtypeAppNotInstalled:
			return "app_status"
		}
		return "permission"
	case errs.CategoryConfig:
		switch sub {
		case errs.SubtypeInvalidClient,
			errs.SubtypeNotConfigured,
			errs.SubtypeInvalidConfig:
			return "config"
		}
		return "config"
	case errs.CategoryAPI:
		switch sub {
		case errs.SubtypeRateLimit:
			return "rate_limit"
		case errs.SubtypeConflict:
			return "conflict"
		case errs.SubtypeCrossTenant:
			return "cross_tenant"
		case errs.SubtypeCrossBrand:
			return "cross_brand"
		case errs.SubtypeInvalidParameters:
			return "invalid_parameters"
		case errs.SubtypeOwnershipMismatch:
			return "ownership_mismatch"
		}
		return "api_error"
	}
	return "api_error"
}
