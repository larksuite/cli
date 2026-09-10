// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"io"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
)

// DiagnoseDir reports references inside a directory payload that will not
// resolve once the payload is online.
//
// --dir publishes what is in the directory and follows nothing, which is the
// point: the caller chose the file set. But it also means a page referencing a
// stylesheet one level up publishes cleanly and then renders unstyled, with
// nothing said at any point. That silence is what makes the failure expensive
// -- and it is where every "use --dir instead" suggestion sends people, so the
// suggestion has to stop being a way to launder a broken payload into a
// successful publish.
//
// This only reports. The file set is untouched: a reference that does not
// resolve is the caller's to fix, and guessing which file they meant would be
// worse than saying what is missing.
func DiagnoseDir(fio fileio.FileIO, root string, candidates []Candidate) []Skip {
	published := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		published[c.RelPath] = true
	}

	var out []Skip
	seen := map[string]bool{}
	note := func(kind SkipKind, ref, from, why string) {
		key := from + "\x00" + ref
		if seen[key] || len(out) >= maxSkipNotes {
			return
		}
		seen[key] = true
		out = append(out, Skip{Ref: ref, From: from, Why: why, Kind: kind})
	}

	for _, c := range candidates {
		if !parseableExts[refExtension(c.RelPath)] {
			continue
		}
		raw, ok := readAll(fio, c.AbsPath)
		if !ok {
			continue
		}
		refs, _, err := collectReferences(c.RelPath, raw)
		if err != nil {
			// A file that cannot be parsed is reported by the publish itself;
			// there is nothing further to say about references it may hold.
			continue
		}
		for _, ref := range refs {
			rel, skip, rerr := resolveReference(c.RelPath, ref)
			switch {
			case skip:
				continue
			case rerr != nil:
				note(SkipOutsideDir, ref, c.RelPath, "it does not point at a file inside the published directory")
			case !published[rel]:
				note(SkipMissing, ref, c.RelPath, whyNotPublished(root, rel))
			}
		}
	}
	return out
}

// whyNotPublished separates "there is no such file" from "the file is there
// but the walker did not take it". Telling someone a file is missing when they
// can see it in the directory reads as a bug in the tool.
func whyNotPublished(root, rel string) string {
	//nolint:forbidigo // fileio exposes no Lstat, and the distinction being drawn here is exactly the one Stat erases by following the link.
	if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return rel + " is a symbolic link, which is never published; replace it with a copy of the file it points at"
		}
		if !info.Mode().IsRegular() {
			return rel + " is not a regular file, so it is not published"
		}
		return rel + " was not published; check that it is inside the directory being published"
	}
	return "the published directory has no " + rel
}

func readAll(fio fileio.FileIO, path string) ([]byte, bool) {
	f, err := fio.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	return raw, true
}
