// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import "github.com/larksuite/cli/errs"

// whiteboardCodeMeta holds whiteboard-service Lark code -> CodeMeta mappings.
var whiteboardCodeMeta = map[int]CodeMeta{
	2890002: {Category: errs.CategoryAPI, Subtype: errs.SubtypeInvalidParameters},
}

func init() { mergeCodeMeta(whiteboardCodeMeta, "whiteboard") }
