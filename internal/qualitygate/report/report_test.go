// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package report

import "testing"

func TestExitCodeRejectOnly(t *testing.T) {
	ds := []Diagnostic{
		{Action: ActionWarning},
		{Action: ActionLabel},
	}
	if got := ExitCode(ds); got != 0 {
		t.Fatalf("warnings and labels should not fail, got %d", got)
	}

	ds = append(ds, Diagnostic{Action: ActionReject})
	if got := ExitCode(ds); got != 1 {
		t.Fatalf("reject should fail, got %d", got)
	}
}
