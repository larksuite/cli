// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRejectPositionalArgs_WithArgs(t *testing.T) {
	t.Parallel()

	validator := rejectPositionalArgs()

	err := validator(&cobra.Command{}, []string{"hello"})
	if err == nil {
		t.Fatal("expected error for positional arg, got nil")
	}
	if !strings.Contains(err.Error(), "positional arguments are not supported") {
		t.Errorf("expected positional args rejection message, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"hello"`) {
		t.Errorf("expected the positional arg value in error, got: %v", err)
	}
}

func TestRejectPositionalArgs_MultipleArgs(t *testing.T) {
	t.Parallel()

	validator := rejectPositionalArgs()

	err := validator(&cobra.Command{}, []string{"hello", "world"})
	if err == nil {
		t.Fatal("expected error for multiple positional args, got nil")
	}
	if !strings.Contains(err.Error(), "positional arguments are not supported") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "hello") || !strings.Contains(err.Error(), "world") {
		t.Errorf("expected all positional args in error, got: %v", err)
	}
}

func TestRejectPositionalArgs_NoArgs(t *testing.T) {
	t.Parallel()

	validator := rejectPositionalArgs()

	if err := validator(&cobra.Command{}, nil); err != nil {
		t.Fatalf("expected no error for nil args, got: %v", err)
	}
	if err := validator(&cobra.Command{}, []string{}); err != nil {
		t.Fatalf("expected no error for empty args, got: %v", err)
	}
}
