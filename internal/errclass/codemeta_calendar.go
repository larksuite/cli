// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// calendarCodeMeta holds calendar-service Lark code → CodeMeta mappings.
// Only codes whose meaning is verifiable from repo evidence are registered;
// ambiguous codes fall back to CategoryAPI via BuildAPIError.
// BuildAPIError consumes this map via mergeCodeMeta + LookupCodeMeta.
var calendarCodeMeta = map[int]CodeMeta{
	190014: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters}, // invalid params (carries a field-level detail lifted into Hint)
}

func init() { mergeCodeMeta(calendarCodeMeta, "calendar") }
