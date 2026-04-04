// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestAuthCheckRun_EmptyScopeReturnsValidationError(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	err := authCheckRun(&CheckOptions{
		Factory: f,
		Scope:   "   ",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--scope cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
}
