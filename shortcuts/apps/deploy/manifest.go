// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
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
	seenPath := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		rel := c.RelPath
		if rel == entryRel {
			rel = IndexName
			seenEntry = true
		}
		// The entry rename can collide: a payload whose entry is page.html but
		// which also carries its own index.html would produce two files at the
		// same zip path, and which one survives unpacking is undefined.
		if seenPath[rel] {
			if rel == IndexName && entryRel != IndexName {
				// Name who dragged the other index.html in: under --file-path
				// the caller never wrote it down, so "the payload already
				// contains one" is not something they can act on by itself.
				origin := "it is in the payload"
				if c.Via != "" {
					origin = fmt.Sprintf("%s references it", c.Via)
				}
				return nil, nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
					"entry conflict: %q is published as %s, but the payload has its own %s (%s)",
					entryRel, IndexName, IndexName, origin).
					WithHint("rename one of the two files, or publish the directory with --dir and pick the entry with --entry-file")
			}
			return nil, nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the payload maps two files onto the same published path %q", rel)
		}
		seenPath[rel] = true
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
