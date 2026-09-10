// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// Bounds on the walk. A page that references a file which references another is
// a graph, not a list, and the zip is assembled in memory. Both values match
// the web client; exceeding either stops the publish rather than trimming the
// payload, because a trimmed payload is a different payload and the GUI would
// have no way to tell.
const (
	maxDepFiles = 200
	maxDepDepth = 16
	// maxSkipNotes caps the reported skips so a broken page cannot flood stderr.
	maxSkipNotes = 100
)

// SkipKind classifies why a referenced file was not published. Callers group by
// kind to give one piece of advice per problem rather than repeating it.
type SkipKind int

const (
	// SkipMissing: the referenced file is not on disk.
	SkipMissing SkipKind = iota
	// SkipUnreadable: it exists but cannot be read, or is not a plain file.
	SkipUnreadable
	// SkipUnparsed: it was published but could not be read for further
	// references, so anything it in turn references is absent.
	SkipUnparsed
	// SkipDynamic: a reference exists but is computed at run time, so no
	// implementation can know which file it names.
	SkipDynamic
	// SkipOutsideDir: a --dir payload references something outside the
	// directory being published.
	SkipOutsideDir
)

// Skip is one reference that was found but not published, or one file that was
// published without being searched.
type Skip struct {
	Ref  string
	From string
	Why  string
	Kind SkipKind
}

func (s Skip) String() string {
	if s.Ref == s.From {
		return fmt.Sprintf("%s: %s", s.From, s.Why)
	}
	return fmt.Sprintf("%s (referenced by %s): %s", s.Ref, s.From, s.Why)
}

// collector walks the reference graph rooted at one entry file, breadth first.
type collector struct {
	fio       fileio.FileIO
	root      string // directory holding the entry, as passed to FileIO
	rootAbs   string // same directory, absolute and symlink-resolved
	entryRel  string
	visited   map[string]bool
	validated map[string]bool
	cands     []Candidate
	skipped   []Skip
	via       map[string]string
}

type queueItem struct {
	rel   string
	depth int
}

func (c *collector) join(rel string) string {
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

func (c *collector) note(kind SkipKind, ref, from, why string) {
	if len(c.skipped) >= maxSkipNotes {
		return
	}
	c.skipped = append(c.skipped, Skip{Ref: ref, From: from, Why: why, Kind: kind})
}

// limitError stops the publish when the payload outgrows what a single-file
// publish is meant to carry.
func limitError(what string) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", what).
		WithHint("publish the whole directory with --dir instead: it packs what is there and follows no references, so neither limit applies")
}

// symlinkError stops the publish on a symbolic link anywhere in the payload.
// Following one would publish a file from outside the directory the caller
// named, and refusing is also what the web client does, so a payload that
// publishes here is a payload the GUI can reproduce.
func symlinkError(rel string) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"%s is a symbolic link; symbolic links cannot be published", rel).
		// Not "use --dir": that walker skips symbolic links too, and skips them
		// silently, so it turns this refusal into a page that publishes without
		// the file and says nothing.
		WithHint("replace the link with a copy of the file it points at")
}

// ensurePathSafe rejects a symbolic link at any level of rel, the payload root
// included. Checking the parents matters as much as the leaf: a linked
// directory would otherwise smuggle in whatever it points at.
func (c *collector) ensurePathSafe(rel string) error {
	segments := strings.Split(rel, "/")
	for i := range segments {
		partial := strings.Join(segments[:i+1], "/")
		abs := c.join(partial)
		if c.validated[abs] {
			continue
		}
		//nolint:forbidigo // fileio exposes no Lstat, and Stat follows links, which is exactly what must be detected here; the path is inside the cwd-checked root.
		info, err := os.Lstat(abs)
		if err != nil {
			// Absence is not a safety problem; the read below reports it.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return symlinkError(partial)
		}
		c.validated[abs] = true
	}
	return nil
}

// read returns the bytes of one payload file. required marks the entry, whose
// absence stops the publish; a missing dependency is only reported.
func (c *collector) read(rel string, required bool) ([]byte, int64, bool, error) {
	if err := c.ensurePathSafe(rel); err != nil {
		return nil, 0, false, err
	}
	p := c.join(rel)
	// Name the file that asked for this one. The caller wrote a reference, not
	// a file list, so "assets/logo.png is missing" is only actionable once they
	// know which page to look in.
	from := c.viaOf(rel)
	if from == "" {
		from = rel
	}
	st, err := c.fio.Stat(p)
	if err != nil {
		if required {
			return nil, 0, false, inputPathError("--file-path", p, err)
		}
		switch {
		case errors.Is(err, fs.ErrNotExist):
			c.note(SkipMissing, rel, from, "the file does not exist")
		case errors.Is(err, fs.ErrPermission):
			c.note(SkipUnreadable, rel, from, "the file cannot be read: permission denied")
		default:
			c.note(SkipUnreadable, rel, from, "the path cannot be read")
		}
		return nil, 0, false, nil
	}
	if !st.Mode().IsRegular() {
		if required {
			return nil, 0, false, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"--file-path %q is not a regular file", p).WithParam("--file-path")
		}
		c.note(SkipUnreadable, rel, from, "the path is not a regular file")
		return nil, 0, false, nil
	}
	f, err := c.fio.Open(p)
	if err != nil {
		if required {
			return nil, 0, false, inputPathError("--file-path", p, err)
		}
		c.note(SkipUnreadable, rel, from, "the file cannot be opened")
		return nil, 0, false, nil
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		if required {
			return nil, 0, false, errs.NewInternalError(errs.SubtypeFileIO, "read %q: %v", p, err).WithCause(err)
		}
		c.note(SkipUnreadable, rel, from, "the file could not be read to the end")
		return nil, 0, false, nil
	}
	return raw, int64(len(raw)), true, nil
}

// collectReferences returns the references written inside one payload file.
// A file whose type carries no references is a leaf and is never read.
func collectReferences(rel string, raw []byte) ([]string, int, error) {
	switch refExtension(rel) {
	case "html", "htm":
		return scanHTML(raw)
	case "svg":
		return scanSVG(raw)
	case "css":
		return scanCSS(raw)
	case "js", "mjs":
		return scanJS(raw)
	case "json":
		return scanJSON(raw)
	default:
		return nil, 0, nil
	}
}

// walk performs the breadth-first closure. It mirrors the web client's batching
// so that the file-count check fires on the same reference, and so a reference
// reachable two ways is queued and deduplicated the same way.
func (c *collector) walk() error {
	c.visited = map[string]bool{}
	queue := []queueItem{{rel: c.entryRel}}

	for len(queue) > 0 {
		pending := queue
		queue = nil

		batch := make([]queueItem, 0, len(pending))
		for _, item := range pending {
			if c.visited[item.rel] {
				continue
			}
			c.visited[item.rel] = true
			batch = append(batch, item)
		}
		if len(c.visited) > maxDepFiles {
			return limitError(fmt.Sprintf(
				"the page's references reach %d files, past the %d-file limit for a single-file publish",
				len(c.visited), maxDepFiles))
		}
		if len(batch) == 0 {
			continue
		}

		for _, item := range batch {
			isEntry := item.rel == c.entryRel
			raw, size, ok, err := c.read(item.rel, isEntry)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			via := ""
			if !isEntry {
				via = c.viaOf(item.rel)
			}
			c.cands = append(c.cands, Candidate{
				RelPath: item.rel, AbsPath: c.join(item.rel), Size: size, Via: via,
			})
			if !parseableExts[refExtension(item.rel)] {
				continue
			}
			next, err := c.expand(item, raw)
			if err != nil {
				return err
			}
			queue = append(queue, next...)
		}
	}
	return nil
}

// expand reads one file's references and returns the queue items they produce.
func (c *collector) expand(item queueItem, raw []byte) ([]queueItem, error) {
	refs, unsupported, err := collectReferences(item.rel, raw)
	if err != nil {
		var pe *parseError
		if errors.As(err, &pe) {
			// The file still ships; it is only left unexpanded, so anything it
			// references is absent from the payload. Saying so beats letting
			// the page arrive with pieces missing and no explanation.
			c.note(SkipUnparsed, item.rel, item.rel, unparsedReason(pe.code))
			return nil, nil
		}
		return nil, err
	}
	if unsupported > 0 {
		c.note(SkipDynamic, item.rel, item.rel, fmt.Sprintf(
			"%d reference(s) are computed at run time and cannot be followed", unsupported))
	}

	var out []queueItem
	for _, ref := range refs {
		rel, skip, err := resolveReference(item.rel, ref)
		if err != nil {
			return nil, err
		}
		if skip || c.visited[rel] {
			continue
		}
		if item.depth >= maxDepDepth {
			return nil, limitError(fmt.Sprintf(
				"references nest more than %d levels deep: %s (%d levels below the entry) references %q",
				maxDepDepth, item.rel, item.depth, ref))
		}
		c.recordVia(rel, item.rel)
		out = append(out, queueItem{rel: rel, depth: item.depth + 1})
	}
	return out, nil
}

func unparsedReason(code string) string {
	switch code {
	case "unsupported_base":
		return "it declares <base href>, so its own references were not followed"
	case "invalid_json":
		return "it is not valid JSON, so its own references were not followed"
	case "invalid_javascript":
		return "it could not be parsed as JavaScript, so its own references were not followed"
	default:
		return "it could not be parsed, so its own references were not followed"
	}
}

// viaOf and recordVia remember which file first pointed at each dependency,
// used to explain a collision the caller never wrote down.
func (c *collector) recordVia(rel, from string) {
	if c.via == nil {
		c.via = map[string]string{}
	}
	if _, seen := c.via[rel]; !seen {
		c.via[rel] = from
	}
}

func (c *collector) viaOf(rel string) string { return c.via[rel] }
