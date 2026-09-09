// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// PackEntry is one file of the upload payload. ZipPath is the protocol layout
// inside the zip; Content carries CLI-generated files such as routes.json.
type PackEntry struct {
	ZipPath string
	AbsPath string
	Content []byte
	Size    int64
}

// BuildManifest turns collected candidates into the zip manifest: the entry's
// ZipPath becomes output/index.html (only inside the zip — the file on disk is
// untouched), everything else keeps its relative path. The returned html paths
// are post-rename and feed route generation.
func BuildManifest(candidates []Candidate, entryRel string) ([]PackEntry, []string, error) {
	entries := make([]PackEntry, 0, len(candidates)+1)
	htmlRels := make([]string, 0, len(candidates))
	seenEntry := false
	for _, c := range candidates {
		rel := c.RelPath
		if rel == entryRel {
			rel = IndexName
			seenEntry = true
		}
		entries = append(entries, PackEntry{
			ZipPath: "output/" + rel,
			AbsPath: c.AbsPath,
			Size:    c.Size,
		})
		if strings.EqualFold(filepath.Ext(rel), ".html") {
			htmlRels = append(htmlRels, rel)
		}
	}
	if !seenEntry {
		return nil, nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"entry file %q is missing from the collected payload", entryRel)
	}
	return entries, htmlRels, nil
}
