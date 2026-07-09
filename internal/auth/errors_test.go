// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"

	"github.com/larksuite/cli/errs"
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

	t.Run("typed missing-UAT error carries sentinel in cause", func(t *testing.T) {
		// The typed constructor preserves the legacy sentinel in the Cause
		// chain, so errors.As traverses into it.
		if !IsNeedUserAuthorizationError(NewNeedUserAuthorizationError("u_1")) {
			t.Fatal("expected typed missing-UAT error to match via its cause chain")
		}
	})

	t.Run("other error", func(t *testing.T) {
		err := errs.NewNetworkError(errs.SubtypeNetworkTransport, "API call failed: timeout")
		if IsNeedUserAuthorizationError(err) {
			t.Fatal("expected unrelated error not to match")
		}
	})
}
