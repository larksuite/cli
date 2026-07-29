// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

func TestReadLocalFileFlag_AcceptsAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	rctx := localFileTestRuntime(t, path)

	if err := rctx.ValidateLocalFileFlag("file", 7); err != nil {
		t.Fatalf("ValidateLocalFileFlag() error = %v", err)
	}
	got, err := rctx.ReadLocalFileFlag("file", 7)
	if err != nil || string(got) != "content" {
		t.Fatalf("ReadLocalFileFlag() = %q, %v; want content", got, err)
	}
}

func TestValidateLocalFileFlag_ReturnsTypedInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(t *testing.T) string
		max  int64
	}{
		{name: "invalid characters", path: func(*testing.T) string { return "input\n.txt" }, max: 10},
		{name: "missing file", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, max: 10},
		{name: "directory", path: func(t *testing.T) string { return t.TempDir() }, max: 10},
		{name: "too large", path: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "large")
			if err := os.WriteFile(path, []byte("123456"), 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}, max: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := localFileTestRuntime(t, tc.path(t)).ValidateLocalFileFlag("file", tc.max)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--file" {
				t.Fatalf("error = %T %v, want invalid_argument for --file", err, err)
			}
		})
	}
}

func TestReadLocalFileFlag_ReturnsTypedInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(t *testing.T) string
		max  int64
	}{
		{name: "invalid characters", path: func(*testing.T) string { return "input\n.txt" }, max: 10},
		{name: "missing file", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, max: 10},
		{name: "directory", path: func(t *testing.T) string { return t.TempDir() }, max: 10},
		{name: "too large", path: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "large")
			if err := os.WriteFile(path, []byte("123456"), 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}, max: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := localFileTestRuntime(t, tc.path(t)).ReadLocalFileFlag("file", tc.max)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--file" {
				t.Fatalf("error = %T %v, want invalid_argument for --file", err, err)
			}
		})
	}
}

func localFileTestRuntime(t *testing.T, path string) *RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("file", "", "")
	if err := cmd.Flags().Set("file", path); err != nil {
		t.Fatal(err)
	}
	return &RuntimeContext{ctx: context.Background(), Cmd: cmd}
}
