// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// minutesCodeMeta holds minutes-service Lark code → CodeMeta mappings.
// Only codes whose meaning is stable across minutes endpoints are registered;
// endpoint-specific codes fall back to CategoryAPI via BuildAPIError.
// Command-specific messages, hints, and subtypes are layered on top via
// per-command enrichment.
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var minutesCodeMeta = map[int]CodeMeta{
	2091005: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // caller lacks edit/read permission for the minute
}

func init() { mergeCodeMeta(minutesCodeMeta, "minutes") }
