// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import "github.com/larksuite/cli/internal/imcontent"

func pureConvertContext(ctx *ConvertContext) *imcontent.ConvertContext {
	if ctx == nil {
		return nil
	}
	return &imcontent.ConvertContext{
		RawContent: ctx.RawContent,
		MentionMap: ctx.MentionMap,
		Mentions:   ctx.Mentions,
	}
}

func unwrapPostLocale(parsed map[string]interface{}) map[string]interface{} {
	return imcontent.UnwrapPostLocale(parsed)
}
