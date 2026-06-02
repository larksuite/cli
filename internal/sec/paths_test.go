// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPaths_OverrideViaEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInstallDirOverride, dir)
	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if p.InstallDir() != dir {
		t.Errorf("InstallDir = %q, want %q", p.InstallDir(), dir)
	}
	if p.DataDir() != filepath.Join(dir, "data") {
		t.Errorf("DataDir = %q, want %s/data", p.DataDir(), dir)
	}
	if !strings.HasPrefix(p.StateFile(), dir) {
		t.Errorf("StateFile not under override root: %q", p.StateFile())
	}
}

func TestPaths_Ensure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInstallDirOverride, dir)
	p, err := DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for _, d := range []string{p.InstallDir(), p.DataDir(), p.VersionsDir()} {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("missing %s: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}
