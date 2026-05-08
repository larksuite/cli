// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestIsNeedUserAuthorizationError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if IsNeedUserAuthorizationError(nil) {
			t.Fatal("expected nil error not to match")
		}
	})

	t.Run("direct auth error", func(t *testing.T) {
		if !IsNeedUserAuthorizationError(&NeedAuthorizationError{UserOpenId: "u_1"}) {
			t.Fatal("expected direct NeedAuthorizationError to match")
		}
	})

	t.Run("wrapped exit error", func(t *testing.T) {
		err := output.ErrNetwork("API call failed: %s", &NeedAuthorizationError{})
		if !IsNeedUserAuthorizationError(err) {
			t.Fatal("expected wrapped ExitError to match")
		}
	})

	t.Run("other error", func(t *testing.T) {
		err := output.ErrNetwork("API call failed: timeout")
		if IsNeedUserAuthorizationError(err) {
			t.Fatal("expected unrelated error not to match")
		}
	})
}
