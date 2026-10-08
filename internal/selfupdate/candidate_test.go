// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestVerifyCandidateVersionIgnoresStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	path := filepath.Join(t.TempDir(), "lark-cli")
	script := "#!/bin/sh\nprintf 'Fetching API metadata...\\n' >&2\nprintf 'lark-cli version 1.2.3\\n'\n"
	if err := vfs.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCandidateVersion(path, "1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyCandidateVersionRejectsInvalidOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell script")
	}
	for _, tt := range []struct{ name, script, want string }{
		{"wrong version", "printf 'lark-cli version 0.0.1\\n'", "want version"},
		{"excess output", "head -c 65536 /dev/zero", "output exceeds"},
		{"inherited pipe", "sleep 3 &\nprintf 'lark-cli version 1.2.3\\n'", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lark-cli")
			if err := vfs.WriteFile(path, []byte("#!/bin/sh\n"+tt.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			err := VerifyCandidateVersion(path, "1.2.3")
			if err == nil || len(err.Error()) > 1200 {
				t.Fatalf("expected bounded verification error, got %v", err)
			}
			if tt.want == "" {
				if !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("expected pipe wait deadline, got %v", err)
				}
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestCandidateInstallPromotesStagedBinaryAndCleansBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "lark-cli")
	backup := target + ".old"
	staged := filepath.Join(root, "staged")
	if err := vfs.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	finalize, err := (&Candidate{path: staged, target: target}).Install()
	if err != nil {
		t.Fatal(err)
	}
	got, err := vfs.ReadFile(target)
	if err != nil || string(got) != "new" {
		t.Fatalf("target = %q, %v", got, err)
	}
	finalize()
	if _, err := vfs.Stat(backup); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("backup remains after finalize: %v", err)
	}
}
