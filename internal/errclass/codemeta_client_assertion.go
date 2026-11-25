// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

var clientAssertionPublicKeyCodeMeta = map[int]CodeMeta{
	1108026: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},
	1108027: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},
	1108107: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},
	1108108: {Category: errs.CategoryAPI, Subtype: errs.SubtypeAlreadyExists},
	1108109: {Category: errs.CategoryAPI, Subtype: errs.SubtypeQuotaExceeded},
	1108110: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied},
	1108111: {Category: errs.CategoryAPI, Subtype: errs.SubtypeConflict},
}

func init() { mergeCodeMeta(clientAssertionPublicKeyCodeMeta, "client_assertion_public_key") }

// IsClientAssertionPublicKeyError identifies deterministic key rejections that
// cannot be resolved by continuing registration polling.
func IsClientAssertionPublicKeyError(code int) bool {
	_, ok := clientAssertionPublicKeyCodeMeta[code]
	return ok
}

func clientAssertionPublicKeyHint(code int) string {
	switch code {
	case 1108026:
		return "check that the public JWK has valid key parameters and is intended for signing, then retry"
	case 1108027:
		return "use a signing algorithm supported by the platform and matching the registered public key"
	case 1108107:
		return "check the app's registered public keys and ensure the assertion KID matches an active key; re-register the matching public key if missing"
	case 1108108:
		return "use the existing public-key registration for this KID, or register a different public key"
	case 1108109:
		return "delete an unused public key in Developer Console, then retry"
	case 1108110:
		return "ask the app administrator to review its enterprise keyless authentication policy before registering a local public key"
	case 1108111:
		return "review the app's Client Assertion configuration in Developer Console and resolve the conflict with local public-key authentication before retrying"
	}
	return ""
}
