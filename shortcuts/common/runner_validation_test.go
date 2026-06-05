// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "testing"

func TestValidateEnumFlags_ReturnsTypedValidation(t *testing.T) {
	rctx := newTestRuntime(map[string]string{"mode": "delete"})
	err := validateEnumFlags(rctx, []Flag{
		{Name: "mode", Enum: []string{"append", "overwrite"}},
	})
	assertValidationParam(t, err, "--mode")
}

func TestHandleShortcutDryRunUnsupported_ReturnsTypedValidation(t *testing.T) {
	err := handleShortcutDryRun(nil, nil, &Shortcut{
		Service: "doc",
		Command: "fetch",
	})
	assertValidationParam(t, err, "--dry-run")
}
