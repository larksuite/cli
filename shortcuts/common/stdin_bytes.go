// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "io"

// ReadStdinBytesOnce claims the command's single stdin for bounded binary input.
// It shares the claim with text Input flags and never trims or decodes bytes.
func (ctx *RuntimeContext) ReadStdinBytesOnce(flag string, limit int64) ([]byte, error) {
	if ctx.stdinConsumed {
		return nil, ValidationErrorf("%s: stdin can only be used by one flag", flag).WithParam(flag)
	}
	if limit <= 0 {
		return nil, ValidationErrorf("%s: invalid stdin byte limit", flag).WithParam(flag)
	}
	ctx.stdinConsumed = true
	if ctx.IO() == nil || ctx.IO().In == nil {
		return nil, ValidationErrorf("%s: stdin is unavailable", flag).WithParam(flag)
	}
	data, err := io.ReadAll(io.LimitReader(ctx.IO().In, limit+1))
	if err != nil {
		return nil, ValidationErrorf("%s: failed to read stdin: %v", flag, err).WithParam(flag).WithCause(err)
	}
	if int64(len(data)) > limit {
		return nil, ValidationErrorf("%s: stdin exceeds %d bytes", flag, limit).WithParam(flag)
	}
	return data, nil
}
