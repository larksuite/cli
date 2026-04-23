// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// presentationRef holds a parsed --presentation input.
//
// Slides shortcuts accept three input shapes:
//   - a raw xml_presentation_id token
//   - a slides URL like https://<host>/slides/<token>
//   - a wiki URL like https://<host>/wiki/<token> (must resolve to obj_type=slides)
type presentationRef struct {
	Kind  string // "slides" | "wiki"
	Token string
}

// parsePresentationRef extracts a presentation token from a token, slides URL, or wiki URL.
// Wiki tokens are returned unresolved; callers must run resolveWikiToSlidesToken to
// obtain the real xml_presentation_id and verify obj_type=slides.
func parsePresentationRef(input string) (presentationRef, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return presentationRef{}, output.ErrValidation("--presentation cannot be empty")
	}
	// URL inputs: parse properly and only honor /slides/ or /wiki/ when they
	// appear as a prefix of the URL path. Substring matching previously let
	// e.g. `https://x/docx/foo?next=/slides/abc` resolve to token "abc".
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Path == "" {
			return presentationRef{}, output.ErrValidation("unsupported --presentation input %q: use an xml_presentation_id, a /slides/ URL, or a /wiki/ URL", raw)
		}
		if token, ok := tokenAfterPathPrefix(u.Path, "/slides/"); ok {
			return presentationRef{Kind: "slides", Token: token}, nil
		}
		if token, ok := tokenAfterPathPrefix(u.Path, "/wiki/"); ok {
			return presentationRef{Kind: "wiki", Token: token}, nil
		}
		return presentationRef{}, output.ErrValidation("unsupported --presentation input %q: use an xml_presentation_id, a /slides/ URL, or a /wiki/ URL", raw)
	}
	// Non-URL input must be a bare token — anything with path/query/fragment
	// chars is rejected so partial-path inputs like `tmp/wiki/wikcn123` don't
	// get silently accepted.
	if strings.ContainsAny(raw, "/?#") {
		return presentationRef{}, output.ErrValidation("unsupported --presentation input %q: use an xml_presentation_id, a /slides/ URL, or a /wiki/ URL", raw)
	}
	return presentationRef{Kind: "slides", Token: raw}, nil
}

// tokenAfterPathPrefix extracts the first path segment after prefix from path.
// Returns ("", false) if path doesn't start with prefix or the segment is empty.
func tokenAfterPathPrefix(path, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := path[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return rest, true
}

// resolvePresentationID resolves a parsed ref into an xml_presentation_id.
// Slides refs pass through; wiki refs are looked up via wiki.spaces.get_node and
// must resolve to obj_type=slides.
func resolvePresentationID(runtime *common.RuntimeContext, ref presentationRef) (string, error) {
	switch ref.Kind {
	case "slides":
		return ref.Token, nil
	case "wiki":
		data, err := runtime.CallAPI(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": ref.Token},
			nil,
		)
		if err != nil {
			return "", err
		}
		node := common.GetMap(data, "node")
		objType := common.GetString(node, "obj_type")
		objToken := common.GetString(node, "obj_token")
		if objType == "" || objToken == "" {
			return "", output.Errorf(output.ExitAPI, "api_error", "wiki get_node returned incomplete node data")
		}
		if objType != "slides" {
			return "", output.ErrValidation("wiki resolved to %q, but slides shortcuts require a slides presentation", objType)
		}
		return objToken, nil
	default:
		return "", output.ErrValidation("unsupported presentation ref kind %q", ref.Kind)
	}
}

// imgSrcPlaceholderRegex matches `src="@<path>"` or `src='@<path>'` inside <img> tags.
// The "@" prefix is the magic marker for "this is a local file path; upload it and
// replace with file_token".
//
// Match groups:
//
//	1: opening quote character (so we can replace symmetrically)
//	2: the path string (everything inside the quotes after the leading @)
//
// We deliberately scope to <img ... src="@..."> rather than any src= so other
// schema elements (like icon/iconType) aren't accidentally rewritten.
// `\s*=\s*` tolerates `src = "..."` style attributes (XML allows whitespace
// around `=`); without it we'd silently leave such placeholders unrewritten.
var imgSrcPlaceholderRegex = regexp.MustCompile(`(?s)<img\b[^>]*?\bsrc\s*=\s*(["'])@([^"']+)(["'])`)

// extractImagePlaceholderPaths returns the de-duplicated list of local paths
// referenced via <img src="@path"> in the given slide XML strings.
//
// Order is preserved (first occurrence wins) so dry-run / progress messages are
// stable across runs.
func extractImagePlaceholderPaths(slideXMLs []string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, xml := range slideXMLs {
		matches := imgSrcPlaceholderRegex.FindAllStringSubmatch(xml, -1)
		for _, m := range matches {
			if m[1] != m[3] {
				// Mismatched opening/closing quotes — Go's RE2 has no backreferences,
				// so we filter it here. Treat as malformed XML and skip.
				continue
			}
			path := strings.TrimSpace(m[2])
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

// xmlRootOpenTagRegex matches the first opening tag of an XML fragment:
// skipping leading whitespace, XML declaration (<?...?>), and comments
// (<!-- ... -->).
//
// Match groups:
//
//	1: leading prefix (whitespace / decl / comments) — preserved on rewrite
//	2: tag name
//	3: attributes span (may be empty; leading whitespace included)
//	4: closing marker — "/>" (self-closing) or ">" (open tag)
//
// Regex is (?s) so "." crosses newlines; we anchor with \A so the opener
// really is the fragment's root, not any nested <el> later in the string.
var xmlRootOpenTagRegex = regexp.MustCompile(`(?s)\A(\s*(?:<\?[^?]*(?:\?[^>][^?]*)*\?>\s*)?(?:<!--.*?-->\s*)*)<([A-Za-z_][\w.-]*)((?:\s[^>]*?)?)(/?>)`)

// xmlIdAttrRegex matches a standalone `id="..."` or `id='...'` attribute
// (with optional whitespace around `=`). Group 1 is the quote char, group 2
// the value. Case-sensitive: XML attribute names are case-sensitive and the
// SML 2.0 schema uses lowercase `id`.
//
// Uses (?:^|\s) instead of \b so that attributes whose names merely contain
// "id" as a suffix (e.g. data-id, xml:id) are not accidentally matched —
// \b treats the '-' / ':' before "id" as a word boundary and would fire.
var xmlIdAttrRegex = regexp.MustCompile(`(?s)(?:^|\s)id\s*=\s*(["'])(.*?)(["'])`)

// ensureXMLRootID parses xmlFragment as XML, locates the root element's
// opening tag, and ensures it carries id="want". Behavior:
//
//   - root has no id → inject ` id="want"` into the attributes span
//   - root has id and value == want → returned unchanged
//   - root has id but value != want → value overridden with want
//
// Whitespace, surrounding attributes, and self-closing form are preserved.
// Nested elements are never touched. Returns an error when no root element
// can be found (empty/malformed fragment).
//
// The regex approach matches the pattern used by imgSrcPlaceholderRegex
// elsewhere in this package: preserve caller formatting instead of round-
// tripping through encoding/xml (which reformats whitespace and loses
// attribute order).
func ensureXMLRootID(xmlFragment, want string) (string, error) {
	m := xmlRootOpenTagRegex.FindStringSubmatchIndex(xmlFragment)
	if m == nil {
		return "", fmt.Errorf("no root element found in XML fragment")
	}
	prefix := xmlFragment[m[2]:m[3]]
	tagName := xmlFragment[m[4]:m[5]]
	attrs := xmlFragment[m[6]:m[7]]
	closer := xmlFragment[m[8]:m[9]]
	rest := xmlFragment[m[1]:]

	// Check for existing id in the attrs span.
	if sub := xmlIdAttrRegex.FindStringSubmatchIndex(attrs); sub != nil {
		if attrs[sub[4]:sub[5]] == want {
			return xmlFragment, nil
		}
		// Override: replace only the value between the existing quotes;
		// the original quote style is preserved because we only touch [sub[4]:sub[5]].
		newAttrs := attrs[:sub[4]] + want + attrs[sub[5]:]
		return prefix + "<" + tagName + newAttrs + closer + rest, nil
	}

	// No id → inject ` id="want"` at the end of the attrs span, preserving
	// any pre-closer whitespace (e.g. the " " in `<shape  type="rect" />`
	// before `/>`). We split the span into (content, trailing-ws), append
	// our attr to the content side, then put the trailing whitespace back.
	trimmed := strings.TrimRight(attrs, " \t\n\r")
	trailing := attrs[len(trimmed):]
	injected := trimmed + fmt.Sprintf(` id="%s"`, want) + trailing
	return prefix + "<" + tagName + injected + closer + rest, nil
}

// xmlContentTagRegex matches a <content> opening tag in its various valid
// forms (open tag, self-closing, or with attributes). The character after
// "content" must be whitespace, '/', or '>' — this ensures that tags whose
// names merely start with "content" (e.g. <contention/>) are not matched.
var xmlContentTagRegex = regexp.MustCompile(`<content(?:\s|/|>)`)

// ensureShapeHasContent ensures that a <shape> root element has a <content/>
// child. The SML 2.0 schema requires every <shape> to carry <content/>; a
// self-closing <shape .../> or an open <shape> without <content> causes the
// backend to return 3350001 (invalid param). Auto-injecting here mirrors the
// id-injection done by ensureXMLRootID — users write natural XML and the CLI
// patches in the required boilerplate.
//
// Only <shape> elements are affected; <img>, <table>, <chart> etc. are left
// untouched because they have different child-element schemas.
func ensureShapeHasContent(xmlFragment string) string {
	m := xmlRootOpenTagRegex.FindStringSubmatchIndex(xmlFragment)
	if m == nil {
		return xmlFragment
	}
	tagName := xmlFragment[m[4]:m[5]]
	if tagName != "shape" {
		return xmlFragment
	}
	closer := xmlFragment[m[8]:m[9]]

	if closer == "/>" {
		prefix := xmlFragment[m[2]:m[3]]
		attrs := xmlFragment[m[6]:m[7]]
		trimmed := strings.TrimRight(attrs, " \t\n\r")
		rest := xmlFragment[m[1]:]
		return prefix + "<" + tagName + trimmed + "><content/></" + tagName + ">" + rest
	}

	afterOpen := xmlFragment[m[1]:]
	if xmlContentTagRegex.MatchString(afterOpen) {
		return xmlFragment
	}

	closeTag := "</" + tagName + ">"
	closeIdx := strings.Index(afterOpen, closeTag)
	if closeIdx < 0 {
		return xmlFragment
	}
	// Only inject when the shape body is empty. If the user already wrote
	// non-content children (e.g. `<shape type="text"><p>hi</p></shape>`),
	// prepending `<content/>` would make <p> a sibling of <content> — per
	// SML 2.0 <p> must live inside <content>, so the result would be schema-
	// legal but semantically wrong (empty content + stray <p>). Leave that
	// case to the backend's 3350001 rather than silently rewrap children.
	if strings.TrimSpace(afterOpen[:closeIdx]) != "" {
		return xmlFragment
	}
	return xmlFragment[:m[1]] + "<content/>" + afterOpen
}

// replaceImagePlaceholders rewrites <img src="@path"> occurrences in the input
// XML by looking up each path in tokens. Paths missing from the map are left
// untouched (callers should ensure the map is complete).
func replaceImagePlaceholders(slideXML string, tokens map[string]string) string {
	return imgSrcPlaceholderRegex.ReplaceAllStringFunc(slideXML, func(match string) string {
		sub := imgSrcPlaceholderRegex.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		quote, path, closeQuote := sub[1], sub[2], sub[3]
		if quote != closeQuote {
			// Mismatched quotes — see extractImagePlaceholderPaths.
			return match
		}
		token, ok := tokens[strings.TrimSpace(path)]
		if !ok {
			return match
		}
		// Replace only the `"@<path>"` segment (quotes inclusive) so any
		// surrounding attrs and whitespace around `=` stay intact. Looking up
		// by the literal `@<path>"` (with closing quote) avoids accidentally
		// matching the same path elsewhere in the tag.
		oldQuoted := fmt.Sprintf("%s@%s%s", quote, path, closeQuote)
		newQuoted := fmt.Sprintf("%s%s%s", quote, token, closeQuote)
		return strings.Replace(match, oldQuoted, newQuoted, 1)
	})
}
