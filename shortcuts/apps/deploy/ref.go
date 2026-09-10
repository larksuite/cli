// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
)

// The reference grammar below mirrors the web client's collector exactly. The
// two implementations must produce the same file set from the same directory,
// because the publish carries a fingerprint of that set and the GUI compares it
// against its own. A file the two sides disagree about does not surface as an
// error anywhere -- it surfaces as a page the GUI reports as permanently out of
// sync, with nothing to explain why.

// externalReferenceRe matches references that do not name a file in the
// payload: anything with a scheme, a protocol-relative URL, a bare fragment.
var externalReferenceRe = regexp.MustCompile(`(?i)^(?:[a-z][a-z\d+.\-]*:|//|#)`)

// windowsDriveRe and fileSchemeRe are rejected outright rather than skipped:
// they name a location on the machine that a published page could never reach,
// so a payload containing one is malformed rather than merely incomplete.
var (
	windowsDriveRe = regexp.MustCompile(`(?i)^[a-z]:[\\/]`)
	fileSchemeRe   = regexp.MustCompile(`(?i)^file:`)
)

// invalidReferenceError is the publish-stopping error for a reference the
// payload must not contain.
func invalidReferenceError(ref, from, why string) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"invalid reference %q in %s: %s", ref, from, why).
		// Deliberately not "use --dir": the entry is published as index.html at
		// the payload root, so a reference above it cannot be expressed in any
		// mode. Pointing --dir at the parent does not help either, because the
		// entry has to sit at that directory's own root.
		WithHint("the entry has to sit at or above everything it references: move those files under the entry's directory, or move the entry up to the directory that holds them and publish from there")
}

// resolveReference turns one raw reference written inside importerRel into a
// payload-relative path. It returns skip=true for references that name nothing
// local, and an error for references the payload must not contain at all.
//
// The order of the checks is load-bearing: the dangerous forms are rejected
// before the external-reference test, so `file:///etc/passwd` and `c:\secrets`
// fail rather than being quietly treated as external URLs.
func resolveReference(importerRel, ref string) (rel string, skip bool, err error) {
	trimmed := strings.TrimSpace(ref)
	// One leading and one trailing quote, matching the web client. Values
	// arriving from a CSS or JS parser are already unquoted; this only catches
	// the ones written with quotes inside an attribute.
	trimmed = strings.TrimPrefix(trimmed, `"`)
	trimmed = strings.TrimPrefix(trimmed, `'`)
	trimmed = strings.TrimSuffix(trimmed, `"`)
	trimmed = strings.TrimSuffix(trimmed, `'`)

	if trimmed == "" || strings.HasPrefix(trimmed, "?") {
		return "", true, nil
	}
	switch {
	case strings.Contains(trimmed, `\`):
		return "", false, invalidReferenceError(ref, importerRel, "it contains a backslash")
	case strings.ContainsRune(trimmed, 0):
		return "", false, invalidReferenceError(ref, importerRel, "it contains a NUL byte")
	case windowsDriveRe.MatchString(trimmed):
		return "", false, invalidReferenceError(ref, importerRel, "it names an absolute Windows path")
	case fileSchemeRe.MatchString(trimmed):
		return "", false, invalidReferenceError(ref, importerRel, "it uses the file: scheme")
	}
	if externalReferenceRe.MatchString(trimmed) {
		return "", true, nil
	}

	pathOnly := trimmed
	if i := strings.IndexAny(pathOnly, "?#"); i >= 0 {
		pathOnly = pathOnly[:i]
	}
	decoded, derr := url.PathUnescape(pathOnly)
	// decodeURIComponent rejects a malformed escape and a sequence that does
	// not decode to valid UTF-8; PathUnescape only catches the first, so the
	// second is checked here to keep the two implementations in step.
	if derr != nil || !utf8.ValidString(decoded) {
		return "", false, invalidReferenceError(ref, importerRel, "it is not a valid percent-encoded path")
	}
	pathOnly = decoded
	switch {
	case strings.Contains(pathOnly, `\`):
		return "", false, invalidReferenceError(ref, importerRel, "it decodes to a path containing a backslash")
	case strings.ContainsRune(pathOnly, 0):
		return "", false, invalidReferenceError(ref, importerRel, "it decodes to a path containing a NUL byte")
	case strings.Contains(pathOnly, ":"):
		return "", false, invalidReferenceError(ref, importerRel, "it decodes to a path containing a colon")
	}

	var segments []string
	if strings.HasPrefix(pathOnly, "/") {
		// Root-absolute: relative to the payload root, which for a single-file
		// publish is the entry file's own directory.
		segments = strings.Split(strings.TrimLeft(pathOnly, "/"), "/")
	} else {
		base := strings.Split(importerRel, "/")
		segments = append(base[:len(base)-1:len(base)-1], strings.Split(pathOnly, "/")...)
	}

	normalized := make([]string, 0, len(segments))
	for _, seg := range segments {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(normalized) == 0 {
				return "", false, invalidReferenceError(ref, importerRel, "it points above the entry file's directory")
			}
			normalized = normalized[:len(normalized)-1]
		default:
			normalized = append(normalized, seg)
		}
	}
	if len(normalized) == 0 {
		return "", false, invalidReferenceError(ref, importerRel, "it does not name a file")
	}
	return strings.Join(normalized, "/"), false, nil
}

// refExtension reads the extension the way both implementations do: the last
// dot in the last path segment, lowercased, empty when there is none.
func refExtension(rel string) string {
	name := rel
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// toDocumentRelative rewrites a reference that the browser would resolve
// against the document rather than against the file containing it -- fetch,
// Worker, XHR and service-worker URLs, and every path found inside JSON. The
// result is root-absolute, so it resolves from the payload root no matter which
// subdirectory the script or manifest sits in.
func toDocumentRelative(ref string) (string, bool) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" || externalReferenceRe.MatchString(trimmed) {
		return "", false
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed, true
	}
	return "/" + strings.TrimPrefix(trimmed, "./"), true
}

// resourcePathRe is the set of extensions that make a string inside a JSON
// document look like a reference to a payload file.
var resourcePathRe = regexp.MustCompile(
	`(?i)\.(?:aac|avif|css|csv|eot|gif|html?|ico|jpe?g|js|json|m4a|map|mjs|mp3|mp4|ogg|otf|png|svg|txt|ttf|wav|webm|webp|woff2?|xml|zip)(?:[?#]|$)`)

// parseableExts are the file types that can reference other files. Anything
// else is a leaf: images, fonts, media.
var parseableExts = map[string]bool{
	"css": true, "htm": true, "html": true, "js": true, "json": true, "mjs": true, "svg": true,
}
