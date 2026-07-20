// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// wikiCodeMeta holds wiki-service Lark code -> CodeMeta mappings observed from
// wiki shortcut failure telemetry. Keep these to wiki-wide meanings only; add
// command-specific recovery guidance at the shortcut layer.
var wikiCodeMeta = map[int]CodeMeta{
	131002: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},          // param err: space_id is not int / invalid page_token
	131005: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                   // wiki node / space not found
	131006: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // wiki space/node read permission denied
}

func init() { mergeCodeMeta(wikiCodeMeta, "wiki") }
