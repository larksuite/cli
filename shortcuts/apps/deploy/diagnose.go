// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
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
	note := func(kind SkipKind, ref, from, why, advice string) {
		key := from + "\x00" + ref
		if seen[key] || len(out) >= maxSkipNotes {
			return
		}
		seen[key] = true
		out = append(out, Skip{Ref: ref, From: from, Why: why, Advice: advice, Kind: kind})
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
				// Reuse what resolveReference already decided. Restating it
				// here is how --dir came to tell people to move /etc/passwd
				// into the directory they were about to publish.
				why, advice := referenceProblem(rerr, ref, c.RelPath)
				note(SkipOutsideDir, ref, c.RelPath, why, advice)
			case !published[rel]:
				why, advice := whyNotPublished(root, rel)
				note(SkipMissing, ref, c.RelPath, why, advice)
			}
		}
	}
	return out
}

// referenceProblem unpacks the reason and the way out that resolveReference
// attached to a rejected reference, so both input modes say the same thing
// about the same reference.
func referenceProblem(err error, ref, from string) (why, advice string) {
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		return err.Error(), ""
	}
	// The message names the reference and the file holding it, both of which
	// the caller already prints. Strip that exact prefix rather than searching
	// for a separator: the reason itself can contain one ("it uses the file:
	// scheme"), and cutting at the wrong colon leaves the word "scheme".
	prefix := fmt.Sprintf("invalid reference %q in %s: ", ref, from)
	if strings.HasPrefix(ve.Message, prefix) {
		return ve.Message[len(prefix):], ve.Hint
	}
	return ve.Message, ve.Hint
}

// whyNotPublished separates "there is no such file" from "the file is there but
// the walker did not take it", and names the level that stopped it. Telling
// someone a file is missing when they can see it in the directory reads as a
// bug in the tool; telling them a leaf is missing when the symbolic link is two
// directories up sends them looking in the wrong place.
func whyNotPublished(root, rel string) (why, advice string) {
	segments := strings.Split(rel, "/")
	for i := range segments {
		partial := strings.Join(segments[:i+1], "/")
		//nolint:forbidigo // fileio exposes no Lstat, and the distinction being drawn here is exactly the one Stat erases by following the link.
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(partial)))
		if err != nil {
			return "the published directory has no " + rel, adviceCreateOrDropReference
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return rel + " is reached through the symbolic link " + partial + ", which is never published",
				"replace " + partial + " with a copy of what it points at"
		}
		if i < len(segments)-1 && !info.IsDir() {
			return rel + " is not reachable: " + partial + " is not a directory", adviceCreateOrDropReference
		}
	}
	//nolint:forbidigo // same rationale as above.
	if info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && !info.Mode().IsRegular() {
		return rel + " is not a regular file, so it is not published", adviceCreateOrDropReference
	}
	return rel + " is present but was not published", adviceCreateOrDropReference
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
