// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestLintcheckExitCode proves the guard gates CI end to end: a violating
// fixture must make the lintcheck binary exit 1, and a clean tree exit 0.
func TestLintcheckExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the lintcheck binary")
	}
	dirty := t.TempDir()
	writeFile(t, dirty, "internal/x/x.go", "package x\n\nvar h = \"https://open.feishu.cn\"\n")

	run := func(dir string) (string, error) {
		cmd := exec.Command("go", "run", "..", dir)
		cmd.Dir = "." // lint/domaincontract — `..` is the lintcheck main package
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := run(dirty)
	if err == nil || !strings.Contains(out, "no-hardcoded-endpoint") {
		t.Fatalf("violating fixture: err=%v out=%s (want exit 1 with a no-hardcoded-endpoint REJECT)", err, out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("violating fixture exit = %v, want 1", err)
	}

	clean := t.TempDir()
	writeFile(t, clean, "internal/x/x.go", "package x\n\nvar ok = 1\n")
	if out, err := run(clean); err != nil {
		t.Fatalf("clean fixture: err=%v out=%s (want exit 0)", err, out)
	}
}
