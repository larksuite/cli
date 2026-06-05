// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

func TestCommandDeniedError_messageFormats(t *testing.T) {
	withReason := &platform.CommandDeniedError{
		Path:       "docs/+update",
		Layer:      "policy",
		ReasonCode: "write_not_allowed",
		Reason:     "write disabled by policy",
	}
	if got := withReason.Error(); got != `command "docs/+update" denied: write disabled by policy` {
		t.Fatalf("Error() with Reason = %q", got)
	}

	noReason := &platform.CommandDeniedError{
		Path:       "docs/+update",
		Layer:      "strict_mode",
		ReasonCode: "identity_not_supported",
	}
	if got := noReason.Error(); got != `command "docs/+update" denied (strict_mode/identity_not_supported)` {
		t.Fatalf("Error() without Reason = %q", got)
	}
}

// errors.As must work so consumers can type-assert without unwrap gymnastics.
func TestCommandDeniedError_satisfiesErrorsAs(t *testing.T) {
	var err error = &platform.CommandDeniedError{Path: "x"}
	var target *platform.CommandDeniedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As should match CommandDeniedError")
	}
	if target.Path != "x" {
		t.Fatalf("target.Path = %q, want %q", target.Path, "x")
	}
}
