// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"go/build"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestEditionSourceIsolation(t *testing.T) {
	t.Parallel()

	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve edition source contract path")
	}
	packageDir := filepath.Dir(sourcePath)
	entries, err := vfs.ReadDir(packageDir)
	if err != nil {
		t.Fatal(err)
	}

	var productionFiles []string
	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		productionFiles = append(productionFiles, entry.Name())
	}

	gooses := []string{"linux", "darwin", "windows"}
	extendedFiles := 0
	standardFiles := 0
	for _, name := range productionFiles {
		extendedName := strings.HasSuffix(name, "_extended.go")
		standardName := strings.HasSuffix(name, "_standard.go")
		if extendedName {
			extendedFiles++
		}
		if standardName {
			standardFiles++
		}

		var (
			editionConditional bool
			inStandard         bool
			inExtended         bool
		)
		for _, goos := range gooses {
			standard := editionFileMatches(t, packageDir, name, goos, false)
			extended := editionFileMatches(t, packageDir, name, goos, true)
			editionConditional = editionConditional || standard != extended
			inStandard = inStandard || standard
			inExtended = inExtended || extended

			if extendedName && standard {
				t.Errorf("%s enters the Standard %s/amd64 build", name, goos)
			}
			if standardName && extended {
				t.Errorf("%s enters the Extended %s/amd64 build", name, goos)
			}
		}

		if editionConditional && !extendedName && !standardName {
			t.Errorf("%s changes inclusion by edition but has no *_extended.go or *_standard.go suffix", name)
		}
		if extendedName && !inExtended {
			t.Errorf("%s is never included by a supported Extended build", name)
		}
		if standardName && !inStandard {
			t.Errorf("%s is never included by a supported Standard build", name)
		}
	}

	if extendedFiles == 0 || standardFiles == 0 {
		t.Fatalf("edition-specific source convention is not represented: extended=%d standard=%d",
			extendedFiles, standardFiles)
	}
}

func editionFileMatches(
	t *testing.T,
	packageDir string,
	name string,
	goos string,
	extended bool,
) bool {
	t.Helper()

	ctx := build.Default
	ctx.GOOS = goos
	ctx.GOARCH = "amd64"
	ctx.CgoEnabled = false
	ctx.BuildTags = nil
	if extended {
		ctx.BuildTags = []string{"extended"}
	}
	matches, err := ctx.MatchFile(packageDir, name)
	if err != nil {
		t.Fatalf("match %s for %s/amd64 extended=%v: %v", name, goos, extended, err)
	}
	return matches
}
