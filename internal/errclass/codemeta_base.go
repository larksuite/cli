// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// baseCodeMeta holds Base/Bitable Lark code -> CodeMeta mappings.
// Only codes whose meaning is verified from Base responses are registered here.
var baseCodeMeta = map[int]CodeMeta{
	91403: {Category: errs.CategoryAuthorization, Subtype: errs.SubtypePermissionDenied}, // Base Forbidden/resource permission denied
}

func init() { mergeCodeMeta(baseCodeMeta, "base") }
