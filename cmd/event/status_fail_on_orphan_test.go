// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestExitForOrphan_Orphan(t *testing.T) {
	statuses := []appStatus{
		{AppID: "cli_a", State: stateRunning},
		{AppID: "cli_b", State: stateOrphan, PID: 70926},
	}
	err := exitForOrphan(statuses, true)
	if err == nil {
		t.Fatal("expected error when failOnOrphan=true and orphan present")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("Code = %d, want %d", exitErr.Code, output.ExitValidation)
	}
}

func TestExitForOrphan_NoOrphan(t *testing.T) {
	statuses := []appStatus{
		{AppID: "cli_a", State: stateRunning},
		{AppID: "cli_b", State: stateNotRunning},
	}
	if err := exitForOrphan(statuses, true); err != nil {
		t.Errorf("expected nil error when no orphan; got %v", err)
	}
}

func TestExitForOrphan_FlagDisabled(t *testing.T) {
	statuses := []appStatus{
		{AppID: "cli_b", State: stateOrphan, PID: 70926},
	}
	if err := exitForOrphan(statuses, false); err != nil {
		t.Errorf("flag off should never return error; got %v", err)
	}
}
