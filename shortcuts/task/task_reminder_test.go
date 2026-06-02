// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// TestReminderTask_RequiresSetOrRemove covers the first Validate guard: neither
// --set nor --remove yields a typed validation error (exit 2) before any API
// call.
func TestReminderTask_RequiresSetOrRemove(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	s := ReminderTask
	args := []string{"+reminder", "--task-id", "task-1", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T, want *errs.ValidationError; err = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
	}
}

// TestReminderTask_CannotSpecifyBoth covers the second Validate guard: passing
// both --set and --remove yields a typed validation error (exit 2).
func TestReminderTask_CannotSpecifyBoth(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	s := ReminderTask
	args := []string{"+reminder", "--task-id", "task-1", "--set", "15m", "--remove", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T, want *errs.ValidationError; err = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", got, output.ExitValidation)
	}
}
