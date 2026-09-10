// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"io"

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
				note(SkipMissing, ref, c.RelPath, "the published directory has no "+rel)
			}
		}
	}
	return out
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
