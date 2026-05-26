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
type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
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
	99991661: {errs.CategoryAuthentication, errs.SubtypeTokenMissing, false},        // Authorization header missing
	99991671: {errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},        // token format error (must start with t- / u-)
	99991668: {errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},        // UAT invalid/expired (server does not distinguish)
	99991663: {errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},        // access_token invalid
	99991677: {errs.CategoryAuthentication, errs.SubtypeTokenExpired, false},        // UAT expired
	20026:    {errs.CategoryAuthentication, errs.SubtypeRefreshTokenInvalid, false}, // refresh_token v1 legacy format
	20037:    {errs.CategoryAuthentication, errs.SubtypeRefreshTokenExpired, false}, // refresh_token expired
	20064:    {errs.CategoryAuthentication, errs.SubtypeRefreshTokenRevoked, false}, // refresh_token revoked
	20073:    {errs.CategoryAuthentication, errs.SubtypeRefreshTokenReused, false},  // refresh_token already used
	20050:    {errs.CategoryAuthentication, errs.SubtypeRefreshServerError, true},   // refresh endpoint transient error

	// CategoryAuthorization
	99991672: {errs.CategoryAuthorization, errs.SubtypeAppScopeNotApplied, false},
	99991676: {errs.CategoryAuthorization, errs.SubtypeTokenScopeInsufficient, false},
	99991679: {errs.CategoryAuthorization, errs.SubtypeMissingScope, false},     // user authorized app but did not grant this scope
	230027:   {errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, false}, // user never authorized the app
	99991673: {errs.CategoryAuthorization, errs.SubtypeAppUnavailable, false},   // app status unavailable
	99991662: {errs.CategoryAuthorization, errs.SubtypeAppNotInstalled, false},  // app not enabled / not installed in tenant

	// CategoryAPI
	99991400: {errs.CategoryAPI, errs.SubtypeRateLimit, true},
	1061045:  {errs.CategoryAPI, errs.SubtypeConflict, true},
	131009:   {errs.CategoryAPI, errs.SubtypeConflict, true}, // wiki write-path lock contention; retryable with backoff
	1064510:  {errs.CategoryAPI, errs.SubtypeCrossTenant, false},
	1064511:  {errs.CategoryAPI, errs.SubtypeCrossBrand, false},
	1310246:  {errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
	1063006:  {errs.CategoryAPI, errs.SubtypeRateLimit, false}, // drive perm-apply quota; 5/day, not short-term retryable
	1063007:  {errs.CategoryAPI, errs.SubtypeInvalidParameters, false},
	231205:   {errs.CategoryAPI, errs.SubtypeOwnershipMismatch, false},

	// CategoryConfig
	99991543: {errs.CategoryConfig, errs.SubtypeInvalidClient, false}, // RFC 6749 §5.2 — app_id / app_secret incorrect

	// CategoryPolicy
	21000: {errs.CategoryPolicy, errs.SubtypeChallengeRequired, false},
	21001: {errs.CategoryPolicy, errs.SubtypeAccessDenied, false},
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
