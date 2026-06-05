// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

func TestAbortError_messageFormats(t *testing.T) {
	bare := &platform.AbortError{HookName: "secaudit.approval", Reason: "needs approval"}
	if got := bare.Error(); got != `hook "secaudit.approval" aborted: needs approval` {
		t.Errorf("Error() = %q", got)
	}

	withCause := &platform.AbortError{
		HookName: "audit.upload",
		Reason:   "upstream unreachable",
		Cause:    fs.ErrNotExist,
	}
	if got := withCause.Error(); got == bare.Error() {
		t.Errorf("Cause should be appended to message, got %q", got)
	}
}

// errors.As must traverse Unwrap so consumers can inspect the cause
// directly. This is the contract the host's wrapAbortError relies on.
func TestAbortError_unwrapErrorsAs(t *testing.T) {
	root := fs.ErrPermission
	ab := &platform.AbortError{
		HookName: "x",
		Reason:   "y",
		Cause:    root,
	}
	if !errors.Is(ab, fs.ErrPermission) {
		t.Errorf("errors.Is should find fs.ErrPermission via Unwrap")
	}
}
