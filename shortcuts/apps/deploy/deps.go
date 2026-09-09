// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"golang.org/x/net/html"
)

// Dependency scanning bounds. A page that references a file which references
// another is a graph, not a list: without these the walk is only bounded by the
// payload itself, and the zip is assembled in memory.
const (
	// maxDepFiles caps the closure, entry file included.
	maxDepFiles = 200
	// maxDepDepth caps how far a reference chain is followed.
	maxDepDepth = 16
	// maxScanBytes caps the bytes read to look for references inside one file.
	// A file above it is still published, just not scanned.
	maxScanBytes = 20 * 1024 * 1024
	// maxSkipNotes caps the reported skips so a broken page cannot flood stderr.
	maxSkipNotes = 100
)

// refStatus is what resolveRef decided about one raw reference.
type refStatus int

const (
	// refIgnore means the reference does not name a local file at all —
	// an absolute URL, a bare fragment, a data: URI. Not worth reporting.
	refIgnore refStatus = iota
	// refOutside means it names a file above the entry's directory. The
	// published layout cannot express that, so it is reported and dropped.
	refOutside
	// refOK means it resolved to a path inside the payload root.
	refOK
)

// schemeRe matches an absolute URL prefix (http:, data:, mailto:, tel:,
// javascript:). Anything carrying a scheme is external to the payload.
var schemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// htmlRefAttrs lists the attributes that carry a subresource the page needs in
// order to render. Attributes that merely navigate (a[href], form[action]) are
// deliberately absent: pulling them in would drag the whole site behind one
// page, which is what --dir is for.
var htmlRefAttrs = map[string]map[string]bool{
	"link":   {"href": true},
	"script": {"src": true},
	"img":    {"src": true, "srcset": true},
	"source": {"src": true, "srcset": true},
	"video":  {"src": true, "poster": true},
	"audio":  {"src": true},
	"track":  {"src": true},
	"iframe": {"src": true},
	"embed":  {"src": true},
	"object": {"data": true},
	"image":  {"href": true, "xlink:href": true},
	"use":    {"href": true, "xlink:href": true},
}

// scanRefs returns the raw reference strings written inside one payload file.
// Files whose type carries no references are not read at all.
func scanRefs(rel string, raw []byte) []string {
	switch strings.ToLower(path.Ext(rel)) {
	case ".html", ".htm":
		return scanHTMLRefs(raw)
	case ".css":
		return scanCSSRefs(raw)
	case ".js", ".mjs":
		return scanJSRefs(raw)
	default:
		return nil
	}
}

// isScannable reports whether scanRefs would look inside this file.
func isScannable(rel string) bool {
	switch strings.ToLower(path.Ext(rel)) {
	case ".html", ".htm", ".css", ".js", ".mjs":
		return true
	default:
		return false
	}
}

// scanHTMLRefs walks the markup with a real tokenizer rather than a regex, so
// a reference sitting inside a comment or an attribute value that contains
// angle brackets is classified the way a browser would classify it.
func scanHTMLRefs(raw []byte) []string {
	var refs []string
	z := html.NewTokenizer(bytes.NewReader(raw))
	// rawText names the element whose text content is currently being read,
	// for the two elements that embed another language inline.
	rawText := ""
	for {
		switch z.Next() {
		case html.ErrorToken:
			return refs

		case html.TextToken:
			switch rawText {
			case "style":
				refs = append(refs, scanCSSRefs(z.Text())...)
			case "script":
				refs = append(refs, scanJSRefs(z.Text())...)
			}

		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := strings.ToLower(string(name))
			attrs := make([][2]string, 0, 4)
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs = append(attrs, [2]string{strings.ToLower(string(k)), string(v)})
			}
			rawText = ""
			if tag == "style" {
				rawText = tag
			}
			if tag == "script" && !hasAttrNamed(attrs, "src") {
				// A script with src has no meaningful inline body; one
				// without src may be a module that imports siblings.
				rawText = tag
			}
			for _, a := range attrs {
				key, val := a[0], a[1]
				if key == "style" {
					refs = append(refs, scanCSSRefs([]byte(val))...)
					continue
				}
				if !htmlRefAttrs[tag][key] {
					continue
				}
				if key == "srcset" {
					refs = append(refs, splitSrcset(val)...)
					continue
				}
				refs = append(refs, val)
			}

		case html.EndTagToken:
			rawText = ""
		}
	}
}

func hasAttrNamed(attrs [][2]string, name string) bool {
	for _, a := range attrs {
		if a[0] == name {
			return true
		}
	}
	return false
}

// splitSrcset pulls the URLs out of a candidate list such as
// "a.png 1x, b@2x.png 2x". Each candidate is a URL followed by an optional
// descriptor, separated by commas.
func splitSrcset(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexAny(part, " \t\n\r\f"); i >= 0 {
			part = part[:i]
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

var (
	cssURLRe    = regexp.MustCompile(`(?i)url\(\s*(?:"([^"\n]*)"|'([^'\n]*)'|([^)'"\s]+))\s*\)`)
	cssImportRe = regexp.MustCompile(`(?i)@import\s+(?:"([^"\n]*)"|'([^'\n]*)')`)
)

// scanCSSRefs collects url(...) targets and quoted @import targets. An
// @import written as `@import url("x.css")` is picked up by the url() pattern.
func scanCSSRefs(raw []byte) []string {
	var refs []string
	for _, re := range []*regexp.Regexp{cssURLRe, cssImportRe} {
		for _, m := range re.FindAllSubmatch(raw, -1) {
			for _, group := range m[1:] {
				if len(group) > 0 {
					refs = append(refs, string(group))
					break
				}
			}
		}
	}
	return refs
}

var (
	jsFromRe    = regexp.MustCompile(`(?:^|[^\w$.])from\s*["']([^"'\n]+)["']`)
	jsDynamicRe = regexp.MustCompile(`(?:^|[^\w$.])import\s*\(\s*["']([^"'\n]+)["']`)
	jsBareRe    = regexp.MustCompile(`(?:^|[^\w$.])import\s+["']([^"'\n]+)["']`)
)

// scanJSRefs collects module specifiers from static import/export, dynamic
// import() and side-effect imports. Only relative and root-absolute specifiers
// are kept: a bare specifier such as "react" names a package, not a file in the
// payload, and following it would be wrong rather than merely useless.
//
// The patterns are deliberately loose — a "from" inside a string literal can
// match. That is harmless: a false positive that names no file on disk is
// dropped by the existence check without a note.
func scanJSRefs(raw []byte) []string {
	var refs []string
	for _, re := range []*regexp.Regexp{jsFromRe, jsDynamicRe, jsBareRe} {
		for _, m := range re.FindAllSubmatch(raw, -1) {
			spec := string(m[1])
			if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/") {
				refs = append(refs, spec)
			}
		}
	}
	return refs
}

// resolveRef turns one raw reference written inside fromRel into a payload
// path. A reference starting with "/" is read as site-root-absolute, which for
// this payload means the entry file's directory.
func resolveRef(fromRel, ref string) (string, refStatus) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return "", refIgnore
	}
	// "//cdn.example.com/x.js" inherits the page's scheme — still external.
	if strings.HasPrefix(ref, "//") || schemeRe.MatchString(ref) {
		return "", refIgnore
	}
	// Whichever of the two comes first ends the path portion.
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" {
		return "", refIgnore
	}
	// %20 and friends are how a file name with a space is written in markup.
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	if strings.HasSuffix(ref, "/") {
		// A directory reference relies on server-side index resolution, which
		// the published layout does not provide.
		return "", refIgnore
	}

	var rel string
	if strings.HasPrefix(ref, "/") {
		rel = path.Clean(strings.TrimPrefix(ref, "/"))
	} else {
		rel = path.Join(path.Dir(fromRel), ref)
	}
	if rel == "" || rel == "." {
		return "", refIgnore
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", refOutside
	}
	if isUnsafeRel(rel) {
		return "", refOutside
	}
	return rel, refOK
}

// depScanner walks the reference graph rooted at one entry file. Every file it
// accepts must live inside the entry's directory and must still be inside it
// after symlinks are resolved, so the scan cannot become a way to reach content
// the flag validation would have rejected.
type depScanner struct {
	fio     fileio.FileIO
	root    string // directory holding the entry, as passed to FileIO
	rootAbs string // same directory, absolute and symlink-resolved
	seen    map[string]bool
	cands   []Candidate
	skipped []string
}

func (s *depScanner) join(rel string) string {
	return filepath.Join(s.root, filepath.FromSlash(rel))
}

func (s *depScanner) note(ref, from, why string) {
	if len(s.skipped) >= maxSkipNotes {
		return
	}
	s.skipped = append(s.skipped, fmt.Sprintf("%s (referenced by %s): %s", ref, from, why))
}

// withinRoot reports whether abs names something strictly inside rootAbs.
// Both sides come from canonicalAbs, so both have symlinks already resolved and
// a prefix comparison is meaningful.
func withinRoot(rootAbs, abs string) bool {
	return strings.HasPrefix(abs, rootAbs+string(filepath.Separator))
}

// accept decides whether one resolved dependency joins the payload.
func (s *depScanner) accept(rel, ref, from string) (int64, bool) {
	p := s.join(rel)
	st, err := s.fio.Stat(p)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			s.note(ref, from, "the file does not exist")
		case errors.Is(err, fs.ErrPermission):
			s.note(ref, from, "the file cannot be read: permission denied")
		default:
			s.note(ref, from, "the path cannot be used")
		}
		return 0, false
	}
	if st.IsDir() {
		s.note(ref, from, "the path is a directory")
		return 0, false
	}
	if !st.Mode().IsRegular() {
		s.note(ref, from, "the path is not a regular file")
		return 0, false
	}
	abs, err := canonicalAbs(p)
	if err != nil {
		s.note(ref, from, "the path cannot be resolved")
		return 0, false
	}
	// Catches a symlink at the leaf and a symlinked directory anywhere above
	// it: after resolution the file must still sit under the entry's directory.
	if !withinRoot(s.rootAbs, abs) {
		s.note(ref, from, "it resolves outside the entry file's directory")
		return 0, false
	}
	return st.Size(), true
}

// refsOf reads one payload file and returns the references written inside it.
func (s *depScanner) refsOf(rel string, size int64) []string {
	if !isScannable(rel) {
		return nil
	}
	if size > maxScanBytes {
		s.note(rel, rel, fmt.Sprintf("it is %s, above the %s scan limit, so its references were not followed",
			HumanBytes(size), HumanBytes(maxScanBytes)))
		return nil
	}
	f, err := s.fio.Open(s.join(rel))
	if err != nil {
		return nil
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxScanBytes))
	if err != nil {
		return nil
	}
	return scanRefs(rel, raw)
}

// walk performs the breadth-first closure starting at the entry file.
func (s *depScanner) walk(entryRel string, entrySize int64) {
	type item struct {
		rel   string
		size  int64
		depth int
	}
	s.seen = map[string]bool{entryRel: true}
	s.cands = []Candidate{{RelPath: entryRel, AbsPath: s.join(entryRel), Size: entrySize}}
	queue := []item{{rel: entryRel, size: entrySize}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxDepDepth {
			continue
		}
		for _, ref := range s.refsOf(cur.rel, cur.size) {
			rel, status := resolveRef(cur.rel, ref)
			switch status {
			case refIgnore:
				continue
			case refOutside:
				s.note(ref, cur.rel, "it resolves outside the entry file's directory")
				continue
			}
			if s.seen[rel] {
				continue
			}
			s.seen[rel] = true
			if len(s.cands) >= maxDepFiles {
				s.note(ref, cur.rel, fmt.Sprintf("the %d-file limit for a single-file publish was reached; publish the directory with --dir instead", maxDepFiles))
				continue
			}
			size, ok := s.accept(rel, ref, cur.rel)
			if !ok {
				continue
			}
			s.cands = append(s.cands, Candidate{RelPath: rel, AbsPath: s.join(rel), Size: size})
			queue = append(queue, item{rel: rel, size: size, depth: cur.depth + 1})
		}
	}
}
