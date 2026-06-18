// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Lark API generic error code constants.
// ref: https://open.feishu.cn/document/server-docs/api-call-guide/generic-error-code
//
// Kept as exported identifiers because external shortcut packages reference
// them by name (e.g. LarkErrOwnershipMismatch). The canonical Category /
// Subtype / Retryable metadata for each code lives in internal/errclass and
// must remain the single source of truth.
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
	LarkErrAppCredInvalid  = 99991543 // app_id or app_secret is incorrect (Open API)
	LarkErrAppNotInUse     = 99991662 // app is disabled in this tenant
	LarkErrAppUnauthorized = 99991673 // app status unavailable; check installation

	// "Wrong app credentials" code from the LEGACY TAT endpoint
	// (/open-apis/auth/v3/tenant_access_token/internal returns 10014, "app secret
	// invalid", instead of 99991543). Since the OAuth v3 migration the CLI mints
	// TAT via accounts/oauth/v3/token and reports this as the OAuth invalid_client
	// error, so it no longer emits 10014 itself; the constant + codemeta mapping
	// are retained as a defensive fallback should 10014 still arrive.
	LarkErrTATInvalidSecret = 10014

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
	// because the typed envelope preserves Problem.Code exactly as returned by
	// the server.
	// These codes indicate the entire batch will keep failing identically and
	// are consumed by shortcuts/mail.isFatalSendErr to abort early.
	LarkErrMailboxNotFound        = 1234013 // mailbox not found or not active
	LarkErrMailSendQuotaUser      = 1236007 // user daily send count exceeded
	LarkErrMailSendQuotaUserExt   = 1236008 // user daily external recipient count exceeded
	LarkErrMailSendQuotaTenantExt = 1236009 // tenant daily external recipient count exceeded
	LarkErrMailQuota              = 1236010 // mail quota limit
	LarkErrTenantStorageLimit     = 1236013 // tenant storage limit exceeded
)
