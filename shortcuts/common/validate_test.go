// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/spf13/cobra"
)

// newTestRuntime creates a RuntimeContext with string flags for testing.
func newTestRuntime(flags map[string]string) *RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for name := range flags {
		cmd.Flags().String(name, "", "")
	}
	// Parse empty args so flags have defaults, then set values.
	cmd.ParseFlags(nil)
	for name, val := range flags {
		cmd.Flags().Set(name, val)
	}
	return &RuntimeContext{Cmd: cmd}
}

func assertValidationParam(t *testing.T, err error, param string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("Subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if param != "" && validationErr.Param != param {
		t.Fatalf("Param = %q, want %q", validationErr.Param, param)
	}
	return validationErr
}

func TestMutuallyExclusive(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		check   []string
		wantErr bool
	}{
		{
			name:    "none set",
			flags:   map[string]string{"a": "", "b": ""},
			check:   []string{"a", "b"},
			wantErr: false,
		},
		{
			name:    "one set",
			flags:   map[string]string{"a": "x", "b": ""},
			check:   []string{"a", "b"},
			wantErr: false,
		},
		{
			name:    "both set",
			flags:   map[string]string{"a": "x", "b": "y"},
			check:   []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "three flags two set",
			flags:   map[string]string{"a": "x", "b": "", "c": "z"},
			check:   []string{"a", "b", "c"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags)
			err := MutuallyExclusive(rt, tt.check...)
			if (err != nil) != tt.wantErr {
				t.Errorf("MutuallyExclusive() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationErrorf_ReturnsTypedInvalidArgument(t *testing.T) {
	err := ValidationErrorf("bad %s", "flag")
	validationErr := assertValidationParam(t, err, "")
	if validationErr.Message != "bad flag" {
		t.Fatalf("Message = %q, want %q", validationErr.Message, "bad flag")
	}
}

func TestTypedFlagGroupHelpers_ReturnValidationParams(t *testing.T) {
	t.Run("mutually exclusive", func(t *testing.T) {
		rt := newTestRuntime(map[string]string{"a": "x", "b": "y"})
		validationErr := assertValidationParam(t, MutuallyExclusiveTyped(rt, "a", "b"), "")
		if len(validationErr.Params) != 2 {
			t.Fatalf("Params len = %d, want 2: %+v", len(validationErr.Params), validationErr.Params)
		}
		if validationErr.Params[0].Name != "--a" || validationErr.Params[1].Name != "--b" {
			t.Fatalf("Params names = %+v, want --a/--b", validationErr.Params)
		}
	})

	t.Run("at least one", func(t *testing.T) {
		rt := newTestRuntime(map[string]string{"a": "", "b": ""})
		validationErr := assertValidationParam(t, AtLeastOneTyped(rt, "a", "b"), "")
		if len(validationErr.Params) != 2 {
			t.Fatalf("Params len = %d, want 2: %+v", len(validationErr.Params), validationErr.Params)
		}
		if !strings.Contains(validationErr.Message, "--a or --b") {
			t.Fatalf("Message = %q, want flag group", validationErr.Message)
		}
	})

	t.Run("exactly one", func(t *testing.T) {
		rt := newTestRuntime(map[string]string{"a": "x", "b": "y"})
		validationErr := assertValidationParam(t, ExactlyOneTyped(rt, "a", "b"), "")
		if len(validationErr.Params) != 2 {
			t.Fatalf("Params len = %d, want 2: %+v", len(validationErr.Params), validationErr.Params)
		}
	})
}

func TestValidatePageSizeTyped_ReturnsTypedValidation(t *testing.T) {
	rt := newTestRuntime(map[string]string{"page-size": "nope"})
	_, err := ValidatePageSizeTyped(rt, "page-size", 10, 1, 20)
	assertValidationParam(t, err, "--page-size")

	rt = newTestRuntime(map[string]string{"page-size": "30"})
	_, err = ValidatePageSizeTyped(rt, "page-size", 10, 1, 20)
	assertValidationParam(t, err, "--page-size")
}

func TestValidateIDTyped_ReturnsTypedValidation(t *testing.T) {
	chatID, err := ValidateChatIDTyped("--chat-ids", "https://example.feishu.cn/foo/oc_abc")
	if err != nil {
		t.Fatalf("ValidateChatIDTyped valid URL: %v", err)
	}
	if chatID != "oc_abc" {
		t.Fatalf("chatID = %q, want oc_abc", chatID)
	}
	assertValidationParam(t, func() error {
		_, err := ValidateChatIDTyped("--chat-ids", "bad")
		return err
	}(), "--chat-ids")
	assertValidationParam(t, func() error {
		_, err := ValidateUserIDTyped("--creator-ids", "bad")
		return err
	}(), "--creator-ids")
}

func TestRejectDangerousCharsTyped_ReturnsTypedValidation(t *testing.T) {
	err := RejectDangerousCharsTyped("--query", "bad\x01")
	validationErr := assertValidationParam(t, err, "--query")
	if !strings.Contains(validationErr.Message, "control character") {
		t.Fatalf("Message = %q, want control character", validationErr.Message)
	}
}

func TestWrapInputStatErrorTyped_ReturnsTypedValidation(t *testing.T) {
	cause := &fileio.PathValidationError{Err: errors.New("outside cwd")}
	err := WrapInputStatErrorTyped(cause)
	validationErr := assertValidationParam(t, err, "")
	if !strings.Contains(validationErr.Message, "unsafe file path") {
		t.Fatalf("Message = %q, want unsafe file path", validationErr.Message)
	}
	if !errors.Is(err, fileio.ErrPathValidation) {
		t.Fatalf("expected errors.Is(fileio.ErrPathValidation) to match")
	}
}

func TestWrapSaveErrorTyped_ClassifiesPathAndFileIO(t *testing.T) {
	pathErr := &fileio.PathValidationError{Err: errors.New("outside cwd")}
	assertValidationParam(t, WrapSaveErrorTyped(pathErr), "")

	mkdirErr := &fileio.MkdirError{Err: errors.New("permission denied")}
	err := WrapSaveErrorTyped(mkdirErr)
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeFileIO {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeFileIO)
	}
}

func TestWrapSaveErrorTyped_PreservesTypedWriteCause(t *testing.T) {
	typed := errs.NewNetworkError(errs.SubtypeNetworkServer, "HTTP 500: chunk failed").
		WithCode(500)
	err := WrapSaveErrorTyped(&fileio.WriteError{Err: typed})

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkServer || p.Code != 500 {
		t.Fatalf("problem = category %q subtype %q code %d, want network/%s/500",
			p.Category, p.Subtype, p.Code, errs.SubtypeNetworkServer)
	}
}

func TestAtLeastOne(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		check   []string
		wantErr bool
	}{
		{
			name:    "none set",
			flags:   map[string]string{"a": "", "b": ""},
			check:   []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "one set",
			flags:   map[string]string{"a": "x", "b": ""},
			check:   []string{"a", "b"},
			wantErr: false,
		},
		{
			name:    "both set",
			flags:   map[string]string{"a": "x", "b": "y"},
			check:   []string{"a", "b"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags)
			err := AtLeastOne(rt, tt.check...)
			if (err != nil) != tt.wantErr {
				t.Errorf("AtLeastOne() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExactlyOne(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		check   []string
		wantErr bool
	}{
		{
			name:    "none set",
			flags:   map[string]string{"a": "", "b": ""},
			check:   []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "one set",
			flags:   map[string]string{"a": "x", "b": ""},
			check:   []string{"a", "b"},
			wantErr: false,
		},
		{
			name:    "both set",
			flags:   map[string]string{"a": "x", "b": "y"},
			check:   []string{"a", "b"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(tt.flags)
			err := ExactlyOne(rt, tt.check...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExactlyOne() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseIntBounded(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		min, max int
		want     int
	}{
		{"within range", "10", 1, 50, 10},
		{"below min", "0", 1, 50, 1},
		{"above max", "100", 1, 50, 50},
		{"at min", "1", 1, 50, 1},
		{"at max", "50", 1, 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Int("page-size", 0, "")
			cmd.ParseFlags(nil)
			cmd.Flags().Set("page-size", tt.val)
			rt := &RuntimeContext{Cmd: cmd}
			got := ParseIntBounded(rt, "page-size", tt.min, tt.max)
			if got != tt.want {
				t.Errorf("ParseIntBounded() = %d, want %d", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateSafePath — symlink escape prevention
// ---------------------------------------------------------------------------

// chdirForTest changes CWD to dir and restores the original CWD on cleanup.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// TestValidateSafePath_RejectsSymlinkEscape verifies that a relative path
// that resolves to a symlink pointing outside CWD is rejected.
func TestValidateSafePath_RejectsSymlinkEscape(t *testing.T) {
	outside := t.TempDir() // target outside CWD
	workDir := t.TempDir()
	chdirForTest(t, workDir)

	// Create a symlink inside CWD pointing to outside.
	if err := os.Symlink(outside, filepath.Join(workDir, "evil_out")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if err := ValidateSafePath(&localfileio.LocalFileIO{}, "evil_out"); err == nil {
		t.Fatal("expected error for symlink pointing outside CWD, got nil")
	}
}

// TestValidateSafePath_RejectsDanglingSymlink verifies that a dangling
// symlink (target does not exist) is rejected to prevent future escapes.
func TestValidateSafePath_RejectsDanglingSymlink(t *testing.T) {
	workDir := t.TempDir()
	chdirForTest(t, workDir)

	if err := os.Symlink("/nonexistent/outside/target", filepath.Join(workDir, "dangling")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if err := ValidateSafePath(&localfileio.LocalFileIO{}, "dangling"); err == nil {
		t.Fatal("expected error for dangling symlink, got nil")
	}
}

// TestValidateSafePath_AllowsNormalSubdir verifies that an existing real
// subdirectory within CWD is accepted.
func TestValidateSafePath_AllowsNormalSubdir(t *testing.T) {
	workDir := t.TempDir()
	chdirForTest(t, workDir)

	subDir := filepath.Join(workDir, "output")
	if err := os.Mkdir(subDir, 0700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := ValidateSafePath(&localfileio.LocalFileIO{}, "output"); err != nil {
		t.Fatalf("expected no error for real subdir, got: %v", err)
	}
}

// TestValidateSafePath_AllowsNonExistentPath verifies that a path that
// does not yet exist (new output directory) is accepted.
func TestValidateSafePath_AllowsNonExistentPath(t *testing.T) {
	workDir := t.TempDir()
	chdirForTest(t, workDir)

	if err := ValidateSafePath(&localfileio.LocalFileIO{}, "new_output_dir"); err != nil {
		t.Fatalf("expected no error for non-existent path, got: %v", err)
	}
}

// TestValidateSafePathTyped_ReturnsTypedValidation verifies that an escaping
// path is rejected with a typed validation error and a safe path passes.
func TestValidateSafePathTyped_ReturnsTypedValidation(t *testing.T) {
	outside := t.TempDir()
	workDir := t.TempDir()
	chdirForTest(t, workDir)

	if err := os.Symlink(outside, filepath.Join(workDir, "evil_out")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertValidationParam(t, ValidateSafePathTyped(&localfileio.LocalFileIO{}, "evil_out"), "")

	if err := ValidateSafePathTyped(&localfileio.LocalFileIO{}, "new_output_dir"); err != nil {
		t.Fatalf("expected no error for safe path, got: %v", err)
	}
}
