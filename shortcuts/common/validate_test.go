// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"os"
	"path/filepath"
	"testing"

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
