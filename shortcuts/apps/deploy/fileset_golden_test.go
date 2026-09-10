// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// rootedFIO serves one fixture directory without the cwd sandbox, so the
// fixtures can live under testdata instead of forcing the test to chdir.
type rootedFIO struct{}

func (rootedFIO) Open(name string) (fileio.File, error)     { return os.Open(name) }
func (rootedFIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }
func (rootedFIO) ResolvePath(p string) (string, error)      { return p, nil }
func (rootedFIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	panic("Save not used in deploy unit tests")
}

// goldenErrorMarkers ties each refusal the web client can raise to the wording
// this implementation uses for the same rule, so a fixture cannot pass by
// failing for an unrelated reason.
var goldenErrorMarkers = map[string]string{
	"invalid_reference":   "invalid reference",
	"entry_path_conflict": "entry conflict",
	"symbolic_link":       "symbolic link",
	"max_depth_exceeded":  "levels deep",
	"file_count_exceeded": "file limit",
}

// goldenFileSet is what the web client produced for one fixture directory.
type goldenFileSet struct {
	Entry string   `json:"entry"`
	Files []string `json:"files"`
	Error string   `json:"error"`
}

// TestFileSetMatchesWebClientGolden pins the dependency closure against the set
// the web client's own collector builds from the same directory.
//
// The fingerprint sent with a publish describes this set, and the GUI compares
// it against the set it would have collected itself. A file one side includes
// and the other does not produces no error anywhere -- it produces a page the
// GUI reports as permanently out of sync. Re-deriving the rules in Go and
// testing them against Go would not catch that, so the expectations here come
// from running the other implementation.
func TestFileSetMatchesWebClientGolden(t *testing.T) {
	raw, err := os.ReadFile("testdata/fileset_golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var cases map[string]goldenFileSet
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("golden file has no cases")
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			entry := want.Entry
			if entry == "" {
				entry = IndexName
			}
			cands, _, _, err := CollectFile(rootedFIO{}, filepath.Join("testdata/fixtures", name, entry))
			// The golden lists published paths, where the entry is index.html,
			// so the rename has to run before comparing -- and it is where an
			// entry collision surfaces.
			var entries []PackEntry
			if err == nil {
				entries, _, err = BuildManifest(cands, entry)
			}

			if want.Error != "" {
				if err == nil {
					t.Fatalf("expected the publish to stop (%s), got file set %v", want.Error, relsOf(cands))
				}
				// Any error would satisfy "it failed", including one from an
				// unrelated guard, which would leave the case green while the
				// rule it exists for stopped working. Tie the failure to the
				// one the other implementation raised.
				ve := requireValidation(t, err, errs.SubtypeFailedPrecondition)
				if marker, ok := goldenErrorMarkers[want.Error]; !ok {
					t.Fatalf("golden names an error %q with no expected wording; add it", want.Error)
				} else if !strings.Contains(ve.Message, marker) {
					t.Errorf("stopped for the wrong reason\n got %q\nwant it to mention %q (%s)", ve.Message, marker, want.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected a file set, got error: %v", err)
			}
			got := make([]string, 0, len(entries))
			for _, e := range entries {
				got = append(got, strings.TrimPrefix(e.ZipPath, "output/"))
			}
			sort.Strings(got)
			expected := append([]string(nil), want.Files...)
			sort.Strings(expected)
			if strings.Join(got, "\n") != strings.Join(expected, "\n") {
				t.Errorf("file set differs from the web client\n got %v\nwant %v", got, expected)
			}
		})
	}
}

func relsOf(cands []Candidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.RelPath)
	}
	return out
}
