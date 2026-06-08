// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// vcCodeMeta holds vc-service Lark code → CodeMeta mappings.
// Only codes whose meaning is verifiable from repo evidence are registered;
// ambiguous codes (e.g. 124002 "recording still generating", which has no
// precise taxonomy fit) fall back to CategoryAPI via BuildAPIError and rely on
// per-command enrichment for a retry hint.
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var vcCodeMeta = map[int]CodeMeta{
	121004: {Category: errs.CategoryAPI, Subtype: errs.SubtypeNotFound},                   // meeting has no minute file
	121005: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // caller is not a participant / lacks view permission
}

func init() { mergeCodeMeta(vcCodeMeta, "vc") }
