// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"

	"github.com/larksuite/cli/errs"
)

// CodeMeta is the classification metadata attached to a Lark numeric code.
// It does NOT carry Message or Hint — those are derived at the dispatcher
// (see BuildAPIError).
//
// Risk + Action are populated only for codes that route to CategoryConfirmation;
// the dispatcher falls back to RiskUnknown + ctx.LarkCmd when either is empty
// so the envelope is never wire-invalid.
type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
	Risk      string // CategoryConfirmation arm only; empty otherwise
	Action    string // CategoryConfirmation arm only; empty otherwise
}

// codeMeta is the central registry. Top-level entries (auth/authorization/api/
// policy/config codes shared across services) live here; service-specific
// sub-tables (e.g. task) live in dedicated files like codemeta_task.go and
// merge into this map via init().
//
// Go language guarantees package-level vars initialize before init() functions,
// so sub-tables registering via init() can always assume codeMeta is non-nil.
var codeMeta = map[int]CodeMeta{
	// CategoryAuthentication
	99991661: {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenMissing},                        // Authorization header missing
	99991671: {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenInvalid},                        // token format error (must start with t- / u-)
	99991668: {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenInvalid},                        // UAT invalid/expired (server does not distinguish)
	99991663: {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenInvalid},                        // access_token invalid
	99991677: {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenExpired},                        // UAT expired
	20024:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshTokenInvalid},                 // authorization code or refresh_token does not match client_id
	20026:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshTokenInvalid},                 // refresh_token is invalid or v1 legacy format
	20037:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshTokenExpired},                 // refresh_token expired
	20050:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshServerError, Retryable: true}, // refresh endpoint transient error
	20064:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshTokenRevoked},                 // refresh_token revoked
	20072:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshServerError},                  // refresh endpoint temporarily unavailable
	20073:    {Category: errs.CategoryAuthentication, Subtype: errs.SubtypeRefreshTokenReused},                  // refresh_token already used

	// CategoryAuthorization
	99991672: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppScopeNotApplied},
	99991676: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeTokenScopeInsufficient},
	99991679: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeMissingScope},     // user authorized app but did not grant this scope
	230027:   {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeUserUnauthorized}, // user never authorized the app
	99991673: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppUnavailable},   // app status unavailable
	99991662: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppDisabled},      // app currently disabled in tenant
	20008:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeUserUnauthorized}, // user does not exist
	20009:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppUnavailable},   // app specified is not installed
	20010:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeUserUnauthorized}, // user does not have permission to use this app
	20048:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppUnavailable},   // app specified is not exist
	20066:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeUserUnauthorized}, // user staus is not normal
	20069:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppDisabled},      // app specified is disabled
	20074:    {Category: errs.CategoryAuthorization, Subtype: errs.SubtypeAppUnavailable},   // app specified not allows for refresh token

	// CategoryAPI
	99991400: {Category: errs.CategoryAPI, Subtype: errs.SubtypeRateLimit, Retryable: true},
	1061045:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict, Retryable: true},
	131009:   {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict, Retryable: true}, // wiki write-path lock contention; retryable with backoff
	1064510:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeCrossTenant},
	1064511:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeCrossBrand},
	1310246:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},
	1063006:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeRateLimit}, // drive perm-apply quota; 5/day, not short-term retryable
	1063007:  {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},
	231205:   {Category: errs.CategoryAPI, Subtype: errs.SubtypeOwnershipMismatch},
	20001:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // request missing required parameter
	20036:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // grant_type not supported
	20063:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // request format error
	20067:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // scope list contains duplicated items
	20068:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // scope list contains forbidden permissions
	20070:    {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // request provide multiple authorization methods

	// CategoryConfig
	99991543: {Category: errs.CategoryConfig, Subtype: errs.SubtypeInvalidClient}, // RFC 6749 §5.2 — app_id / app_secret incorrect (Open API)
	10014:    {Category: errs.CategoryConfig, Subtype: errs.SubtypeInvalidClient}, // legacy TAT endpoint — "app secret invalid" (pre-v3 variant of 99991543; CLI now reports invalid_client)
	20002:    {Category: errs.CategoryConfig, Subtype: errs.SubtypeInvalidClient}, // client secret invalid

	// CategoryPolicy
	21000: {Category: errs.CategoryPolicy, Subtype: errs.SubtypeChallengeRequired},
	21001: {Category: errs.CategoryPolicy, Subtype: errs.SubtypeAccessDenied},
}

// LookupCodeMeta is the single lookup entry. Returns ok=false for unknown codes —
// the caller (BuildAPIError) is responsible for falling back to
// CategoryAPI/SubtypeUnknown.
func LookupCodeMeta(code int) (CodeMeta, bool) {
	m, ok := codeMeta[code]
	return m, ok
}

// mergeCodeMeta is invoked by sub-table init() functions to merge service-specific
// codes into the central registry. Panics on duplicate code so a misregistration
// fails fast at startup rather than producing silently-inconsistent classification.
func mergeCodeMeta(src map[int]CodeMeta, owner string) {
	for code, meta := range src {
		if existing, dup := codeMeta[code]; dup {
			panic(fmt.Sprintf("codeMeta dup: code %d already mapped %+v; %s wants %+v",
				code, existing, owner, meta))
		}
		codeMeta[code] = meta
	}
}
