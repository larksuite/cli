// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestRejectPositionalArgs_WithArgs(t *testing.T) {
	t.Parallel()

	validator := rejectPositionalArgs()

	err := validator(&cobra.Command{}, []string{"hello"})
	if err == nil {
		t.Fatal("expected error for positional arg, got nil")
	}
	// The message keeps every stray word so the user can see all of them at once.
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
	// All stray words belong in the message: naming only the first would hide
	// the rest from a user who passed several.
	if !strings.Contains(err.Error(), "positional arguments are not supported") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "hello") || !strings.Contains(err.Error(), "world") {
		t.Errorf("expected all positional args in error, got: %v", err)
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	// param identifies the parameter the caller must correct, and agents read it
	// literally. A shortcut declares no positional parameter, so echoing what the
	// user typed there would name a parameter that does not exist.
	if verr.Param != "" {
		t.Errorf("param = %q, want it unset: a shortcut has no positional parameter to name", verr.Param)
	}
	if len(verr.Params) != 0 {
		t.Errorf("params = %v, want none", verr.Params)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error %T carries no Problem", err)
	}
	if problem.Category != errs.CategoryValidation ||
		problem.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("classified as %s/%s, want %s/%s",
			problem.Category, problem.Subtype,
			errs.CategoryValidation, errs.SubtypeInvalidArgument)
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

func TestShortcutFlagIntArray(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	var got []int
	shortcut := Shortcut{
		Service:     "slides",
		Command:     "+screenshot",
		Description: "capture screenshots",
		Flags: []Flag{
			{Name: "slide-number", Type: "int_array"},
		},
		Execute: func(ctx context.Context, runtime *RuntimeContext) error {
			got = runtime.IntArray("slide-number")
			return nil
		},
	}
	shortcut.Mount(parent, f)
	parent.SetArgs([]string{"+screenshot", "--as", "user", "--slide-number", "1", "--slide-number", "2,3"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("slide-number = %#v, want %#v", got, want)
	}
}
