// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateImportSurface = flag.Bool("update-import-surface", false, "rewrite the recorded import surface files")

// importSurfacePlatforms are the release targets. GOARCH is pinned because the
// recorded surface is architecture-independent; only GOOS changes it.
var importSurfacePlatforms = []string{"darwin", "linux", "windows"}

// TestExternalImportSurface pins every non-stdlib package linked into the CLI
// binary, per platform.
//
// Adding a module is visible: go.mod changes and the diff invites a look.
// Adding a *subpackage* of a module that is already required is not -- the
// diff is one import line, go.mod is untouched, and the binary silently grows
// a new package graph. golang.org/x/net/idna entered that way, via a single
// httpguts import added for a header check, and brought three x/text packages
// with it; nobody noticed until an advisory landed on idna.
//
// Recording the surface turns that invisible change into a reviewable diff.
// A legitimate new dependency just needs the files regenerated:
//
//	go test ./internal/qualitygate/deptest/ -run TestExternalImportSurface -update-import-surface
func TestExternalImportSurface(t *testing.T) {
	root := repoRoot(t)
	for _, goos := range importSurfacePlatforms {
		t.Run(goos, func(t *testing.T) {
			got := externalPackages(t, root, goos)
			golden := filepath.Join(root, "internal", "qualitygate", "deptest", "testdata", "import-surface-"+goos+".txt")
			content := strings.Join(got, "\n") + "\n"
			if *updateImportSurface {
				if err := os.WriteFile(golden, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v\nregenerate with: go test ./internal/qualitygate/deptest/ -run TestExternalImportSurface -update-import-surface", err)
			}
			added, removed := diffLines(strings.Split(strings.TrimSpace(string(want)), "\n"), got)
			if len(added) == 0 && len(removed) == 0 {
				return
			}
			t.Errorf("the %s binary's external package surface changed.\n"+
				"  added:   %v\n"+
				"  removed: %v\n"+
				"Confirm every added package is intended -- a subpackage of an already-required module\n"+
				"enters here without any go.mod change -- then regenerate:\n"+
				"  go test ./internal/qualitygate/deptest/ -run TestExternalImportSurface -update-import-surface",
				goos, added, removed)
		})
	}
}

// externalPackages lists the packages linked into the CLI binary that come
// from neither the standard library nor this repository. Standard-library
// packages carry no dot in their first path element; the toolchain's own
// vendored copies are namespaced under "vendor/" and are maintained by the Go
// release, not by this module.
func externalPackages(t *testing.T, root, goos string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64")
	// Read stdout only. go list reports module downloads ("go: downloading
	// ...") on stderr, and a cold module cache -- CI, or a platform whose
	// dependencies this host has never fetched -- would otherwise mix those
	// lines into the package list.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . (GOOS=%s) failed: %v\n%s", goos, err, stderr.String())
	}
	var pkgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg := strings.TrimSpace(line)
		switch {
		case pkg == "", strings.HasPrefix(pkg, "vendor/"):
			continue
		case strings.HasPrefix(pkg, "github.com/larksuite/cli"):
			continue
		case strings.ContainsAny(pkg, " \t"):
			// An import path never contains whitespace; anything that does is
			// diagnostic output, not a package.
			t.Fatalf("unexpected non-package line from go list (GOOS=%s): %q", goos, pkg)
		}
		if first, _, _ := strings.Cut(pkg, "/"); strings.Contains(first, ".") {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

func diffLines(want, got []string) (added, removed []string) {
	inWant := make(map[string]bool, len(want))
	for _, w := range want {
		inWant[w] = true
	}
	inGot := make(map[string]bool, len(got))
	for _, g := range got {
		inGot[g] = true
		if !inWant[g] {
			added = append(added, g)
		}
	}
	for _, w := range want {
		if !inGot[w] {
			removed = append(removed, w)
		}
	}
	return added, removed
}
