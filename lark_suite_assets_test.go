// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestInstallWizardUsesRegularSkillsRoute(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	contents, err := vfs.ReadFile(filepath.Join(filepath.Dir(currentFile), "scripts", "install-wizard.js"))
	if err != nil {
		t.Fatalf("read install wizard: %v", err)
	}
	const route = `const SKILLS_REPO = "https://open.feishu.cn/lark-cli/skills/regular";`
	if !strings.Contains(string(contents), route) {
		t.Fatalf("install wizard must use the production regular Agent Skills route")
	}
}

func TestLarkSuiteArchivePathsFitSkillsInstallerTarLimit(t *testing.T) {
	// npx skills v1.5.21 did not honor PAX long-path metadata while extracting
	// Agent Skills v0.2 archives and instead used tar's 100-byte name field. A
	// 101-byte suite entry ending in .md was consequently installed as .m. Keep
	// generated suite paths within this compatibility limit until the upstream
	// extractor reliably supports long archive paths.
	const maxTarPathBytes = 100

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	skillsRoot := filepath.Join(filepath.Dir(currentFile), "skills")

	var walk func(string, string)
	walk = func(dir, relativeDir string) {
		t.Helper()
		entries, err := vfs.ReadDir(dir)
		if err != nil {
			t.Fatalf("read skills directory %s: %v", dir, err)
		}
		for _, entry := range entries {
			relativePath := filepath.Join(relativeDir, entry.Name())
			archivePath := filepath.ToSlash(filepath.Join("references", relativePath))
			if entry.IsDir() {
				archivePath += "/"
			}
			if pathBytes := len([]byte(archivePath)); pathBytes > maxTarPathBytes {
				t.Errorf("suite archive path %q is %d bytes; npx skills compatibility limit is %d bytes", archivePath, pathBytes, maxTarPathBytes)
			}
			if entry.IsDir() {
				walk(filepath.Join(dir, entry.Name()), relativePath)
			}
		}
	}

	walk(skillsRoot, "")
}
