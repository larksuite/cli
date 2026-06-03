// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestUserOpenID_ValidatePass(t *testing.T) {
	if err := UserOpenID("ou_abc").ValidateValue(nil, "user-id"); err != nil {
		t.Errorf("ou_ prefix should pass, got: %v", err)
	}
}

func TestUserOpenID_ValidateReject(t *testing.T) {
	for _, v := range []string{"", "oc_abc", "abc"} {
		err := UserOpenID(v).ValidateValue(nil, "user-id")
		if err == nil {
			t.Errorf("expected error for %q", v)
			continue
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "user-id" {
			t.Errorf("wrong wrap for %q: %v", v, err)
		}
	}
}
