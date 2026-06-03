// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestSafePath_AcceptRelative(t *testing.T) {
	for _, p := range []string{"local.txt", "./sub/dir/x", "a/b/c"} {
		if err := SafePath(p).ValidateValue(nil, "file"); err != nil {
			t.Errorf("%q should pass, got: %v", p, err)
		}
	}
}

func TestSafePath_RejectAbsolute(t *testing.T) {
	err := SafePath("/etc/passwd").ValidateValue(nil, "file")
	if err == nil {
		t.Fatal("absolute path must be rejected")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "file" {
		t.Errorf("wrong wrap: %v", err)
	}
}

func TestSafePath_RejectDotDot(t *testing.T) {
	err := SafePath("../leak").ValidateValue(nil, "file")
	if err == nil {
		t.Fatal("'..' segment must be rejected")
	}
}

func TestSafePath_RejectMidPathDotDot(t *testing.T) {
	// filepath.Clean collapses "a/../b" to "b"; the raw-segment scan must
	// still reject it because the user literally typed a parent segment.
	for _, p := range []string{"a/../b", "sub/../../etc", `win\..\x`} {
		if err := SafePath(p).ValidateValue(nil, "file"); err == nil {
			t.Errorf("%q should be rejected (raw .. segment)", p)
		}
	}
}
