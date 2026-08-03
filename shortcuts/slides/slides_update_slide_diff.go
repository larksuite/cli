// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
)

// This file turns "here is the page I want" into the element-level parts the
// slide.replace endpoint accepts.
//
// A whole-page part is not an option: ReplacePart.block_id is validated as a
// short ELEMENT id (it must start with "b"), so neither the page's own id nor
// the background fill's id (which starts with "f") can be addressed. Verified
// against the live API — a slide-rooted replacement and a fill-targeted
// replacement are both rejected with 3350001, while element-level parts on the
// same page succeed. So the CLI diffs the caller's page against the current one
// and emits one part per changed element.
//
// Comparison is canonical (attributes sorted, insignificant whitespace
// dropped) because the server returns pretty-printed XML with normalized
// attribute order and injected style defaults; a raw string compare would call
// every element changed. What gets SENT is the caller's exact bytes, so their
// formatting and attribute order survive into the page.

// contentError reports an edit --content asks for that element-level parts
// cannot express. Every one of these is a refusal to guess: the alternative is
// applying part of the edit, or returning success with it silently dropped.
func contentError(format string, args ...any) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...).WithParam("--content")
}

// smlNamespaces are the namespace forms a page may declare on its root — the
// official identifier plus the two read-back spellings the server emits.
// Mirrors ACCEPTED_SML_NAMESPACES in
// skills/lark-slides/scripts/sxsd_validator.py; keep the two lists in sync.
var smlNamespaces = map[string]bool{
	"http://www.larkoffice.com/sml/2.0":  true,
	"https://www.larkoffice.com/sml/2.0": true,
	"/sml/2.0":                           true,
}

// placeholderTag is the element the server substitutes for objects it cannot
// export as SML — a whiteboard read without its export option, video and audio
// embeds. The caller cannot see what is behind one, and no self-contained
// endpoint test can prove that a page rewrite preserves it (boards cannot be
// created programmatically — the CLI has no whiteboard-create and SML has no
// whiteboard element), so pages carrying one are refused outright rather than
// edited on an unverifiable assumption. The element-level escape hatch is
// +replace-slide.
const placeholderTag = "undefined"

// pageNode is one addressable node of a page.
type pageNode struct {
	Tag string
	// ID is the short id carried by the node, empty when it has none. Only
	// "b"-prefixed ids are addressable as a part's block_id.
	ID string
	// Raw is the caller's (or server's) exact bytes for this node.
	Raw string
	// Canon is the comparison form: attributes sorted, whitespace collapsed.
	Canon string
}

// pageDoc is a decomposed <slide> document.
type pageDoc struct {
	// RootTag is the local name of the first element, "" when there is none.
	RootTag string
	RootID  string
	// Style and Note are the <style> / <note> children of <slide>, nil when
	// absent.
	Style *pageNode
	Note  *pageNode
	// Elements are the <data> children in document order.
	Elements []pageNode
	// TrailingTag and TrailingText report content after the root's close tag.
	TrailingTag  string
	TrailingText bool
	// Unsupported names the first slide-level structure the diff cannot
	// represent: an unknown direct child of <slide>, a duplicate <style> /
	// <data> / <note>, or stray text. Empty means the page decomposed cleanly.
	//
	// This must be an error, not a shrug: a diff computed from only the
	// recognized parts would drop the unrecognized edit and could even report
	// `unchanged` — a success claim for a change that never happened.
	Unsupported string
}

// elementByID indexes Elements by id, skipping nodes without one.
func (d pageDoc) elementByID() map[string]pageNode {
	out := make(map[string]pageNode, len(d.Elements))
	for _, el := range d.Elements {
		if el.ID != "" {
			out[el.ID] = el
		}
	}
	return out
}

// orderedIDs returns the ids of Elements that carry one, in document order.
func (d pageDoc) orderedIDs() []string {
	out := make([]string, 0, len(d.Elements))
	for _, el := range d.Elements {
		if el.ID != "" {
			out = append(out, el.ID)
		}
	}
	return out
}

// parsePageDoc walks a <slide> document once and records every addressable
// node together with the exact bytes it came from.
//
// Raw slices are cut from the input using the decoder's byte offsets rather
// than re-serialized, so a replacement carries the caller's own formatting
// instead of whatever encoding/xml would emit.
func parsePageDoc(pageXML string) (pageDoc, error) {
	var doc pageDoc
	decoder := xml.NewDecoder(strings.NewReader(pageXML))
	var (
		stack      []string
		rootClosed bool
		dataSeen   int
		// capture is the node currently being sliced out: its start offset and
		// the depth at which it ends.
		capturing  bool
		captureAt  int64
		captureTag string
		captureID  string
		captureIn  string // "root" for <slide> children, "data" for <data> children
	)
	unsupported := func(format string, args ...any) {
		if doc.Unsupported == "" {
			doc.Unsupported = fmt.Sprintf(format, args...)
		}
	}
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return doc, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch {
			case len(stack) == 0 && rootClosed:
				if doc.TrailingTag == "" {
					doc.TrailingTag = t.Name.Local
				}
			case len(stack) == 0:
				doc.RootTag, doc.RootID = t.Name.Local, attrValue(t, "id")
				// The diff carries the root's id and nothing else, so any
				// other attribute would be accepted and then neither compared
				// nor sent — an edit to it would vanish into `unchanged`.
				// Namespace declarations are no exception: a binding inherited
				// from the root changes what every descendant name means, and
				// the canonical comparison reads local names precisely because
				// legitimate pages differ only in carrying the one official
				// declaration or not. Anything else must not be waved through.
				for _, attr := range t.Attr {
					switch {
					case attr.Name.Space == "" && attr.Name.Local == "id":
					case attr.Name.Space == "" && attr.Name.Local == "xmlns":
						if !smlNamespaces[attr.Value] {
							unsupported("an unsupported xmlns %q on <%s>", attr.Value, t.Name.Local)
						}
					case attr.Name.Space == "xmlns":
						unsupported("a prefixed namespace declaration %q on <%s>", "xmlns:"+attr.Name.Local, t.Name.Local)
					default:
						unsupported("an unsupported attribute %q on <%s>", attrDisplayName(attr), t.Name.Local)
					}
				}
			case !capturing && len(stack) == 1:
				// Direct children of <slide>: exactly one <style>, one <data>
				// and one <note> are representable; anything else has no
				// element-level part it could become.
				switch t.Name.Local {
				case "style":
					if doc.Style != nil {
						unsupported("a second <style> element")
						break
					}
					capturing, captureAt = true, elementStart(pageXML, before)
					captureTag, captureID, captureIn = t.Name.Local, attrValue(t, "id"), "root"
				case "note":
					if doc.Note != nil {
						unsupported("a second <note> element")
						break
					}
					capturing, captureAt = true, elementStart(pageXML, before)
					captureTag, captureID, captureIn = t.Name.Local, attrValue(t, "id"), "root"
				case "data":
					dataSeen++
					if dataSeen > 1 {
						unsupported("a second <data> element")
					}
					// <data> is pure structure to the diff; an attribute on it
					// — namespace declarations included, since captured Raw
					// slices would not carry an inherited binding — has no
					// element-level part it could travel in.
					for _, attr := range t.Attr {
						unsupported("an unsupported attribute %q on <data>", attrDisplayName(attr))
					}
				default:
					unsupported("an unknown <%s> element directly under <slide>", t.Name.Local)
				}
			case !capturing && len(stack) == 2 && stack[1] == "data":
				capturing, captureAt = true, elementStart(pageXML, before)
				captureTag, captureID, captureIn = t.Name.Local, attrValue(t, "id"), "data"
			}
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			closingDepth := len(stack)
			if closingDepth > 0 {
				stack = stack[:closingDepth-1]
			}
			if len(stack) == 0 {
				rootClosed = true
			}
			// A captured node ends when the stack returns to the depth it
			// started at: "root" children start at depth 1, "data" children at
			// depth 2.
			startDepth := 1
			if captureIn == "data" {
				startDepth = 2
			}
			if capturing && len(stack) == startDepth {
				raw := strings.TrimSpace(pageXML[captureAt:decoder.InputOffset()])
				canon, err := canonicalizeElement(raw)
				if err != nil {
					return doc, err
				}
				node := pageNode{Tag: captureTag, ID: captureID, Raw: raw, Canon: canon}
				switch {
				case captureIn == "root" && captureTag == "style":
					doc.Style = &node
				case captureIn == "root" && captureTag == "note":
					doc.Note = &node
				case captureIn == "data":
					doc.Elements = append(doc.Elements, node)
				}
				capturing = false
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) == "" {
				break // pretty-printed indentation, never meaningful
			}
			switch {
			case rootClosed && len(stack) == 0:
				doc.TrailingText = true
			case !capturing && len(stack) == 1:
				unsupported("text directly inside <slide>")
			case !capturing && len(stack) == 2 && stack[1] == "data":
				unsupported("text directly inside <data>")
			}
		}
	}
	return doc, nil
}

// elementStart returns the offset of the '<' that opens the token beginning at
// or after off, so a captured slice starts at the tag rather than at the
// whitespace preceding it.
func elementStart(s string, off int64) int64 {
	for i := int(off); i < len(s); i++ {
		if s[i] == '<' {
			return int64(i)
		}
	}
	return off
}

func attrValue(el xml.StartElement, name string) string {
	for _, attr := range el.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// attrDisplayName renders an attribute name for error messages.
func attrDisplayName(attr xml.Attr) string {
	if attr.Name.Space != "" {
		return attr.Name.Space + ":" + attr.Name.Local
	}
	return attr.Name.Local
}

// canonicalizeElement renders an element fragment in a stable form: attributes
// sorted by name, indentation between structural elements dropped, paragraph
// text preserved verbatim and escaped.
//
// This exists so "unchanged" survives the round trip through the server, which
// re-orders attributes, indents the XML and injects style defaults that the
// caller never wrote.
//
// The whitespace rule is asymmetric by design. Outside <p>, whitespace-only
// character data is the pretty-printer's indentation and never content. Inside
// a <p> subtree it is kept verbatim: SML itself collapses a literal space
// between inline tags, but a preserved space written as &#32; decodes to the
// very same token, so dropping "insignificant" whitespace here would also drop
// a real &#32; edit as `unchanged`. Keeping both costs at most a spurious
// rewrite of identical content; dropping either loses an edit.
func canonicalizeElement(fragment string) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(fragment))
	var out strings.Builder
	pDepth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return out.String(), nil
		}
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.StartElement:
			// Element names stay namespace-free on purpose: a fragment cut
			// from a document with a default xmlns resolves its elements into
			// that namespace, while the same fragment from the server's plain
			// output does not — including it would make every round-trip look
			// changed. SML is a single vocabulary, so local names cannot clash.
			out.WriteString("<" + t.Name.Local)
			attrs := make([]string, 0, len(t.Attr))
			for _, attr := range t.Attr {
				name := attr.Name.Local
				// Attributes do not inherit the default namespace, so a
				// non-empty Space is explicit (xmlns declarations, prefixed
				// attributes) and part of the attribute's identity.
				if attr.Name.Space != "" {
					name = attr.Name.Space + ":" + name
				}
				// Quote the value: with bare name=value concatenation,
				// alt="foo rotateWithShape=true" and the two attributes
				// alt="foo" rotateWithShape="true" canonicalize identically,
				// and the diff would drop a real change as unchanged.
				attrs = append(attrs, name+"="+strconv.Quote(attr.Value))
			}
			sort.Strings(attrs)
			for _, attr := range attrs {
				out.WriteString(" " + attr)
			}
			out.WriteString(">")
			if t.Name.Local == "p" {
				pDepth++
			}
		case xml.EndElement:
			out.WriteString("</" + t.Name.Local + ">")
			if t.Name.Local == "p" && pDepth > 0 {
				pDepth--
			}
		case xml.CharData:
			if pDepth == 0 && strings.TrimSpace(string(t)) == "" {
				break // indentation between structural elements, never content
			}
			// Escaped, so text can never imitate markup in the comparison
			// stream: a paragraph holding the literal text "</p><p>" must not
			// compare equal to two empty paragraphs.
			if err := xml.EscapeText(&out, t); err != nil {
				return "", err
			}
		}
	}
}

// replacePart is one entry of the request body.
type replacePartOut struct {
	Action              string
	BlockID             string
	Replacement         string
	Insertion           string
	InsertBeforeBlockID string
}

func (p replacePartOut) toMap() map[string]interface{} {
	m := map[string]interface{}{"action": p.Action}
	switch p.Action {
	case "block_replace":
		m["block_id"] = p.BlockID
		m["replacement"] = p.Replacement
	case "block_insert":
		m["insertion"] = p.Insertion
		if p.InsertBeforeBlockID != "" {
			m["insert_before_block_id"] = p.InsertBeforeBlockID
		}
	}
	return m
}

// pageDiff is the outcome of comparing the wanted page against the current one.
type pageDiff struct {
	Parts []replacePartOut
	// Counters describe what the parts do, for the result envelope.
	Replaced, Inserted, Deleted int
	NoteCleared, NoteReplaced   bool
}

// diffPage produces the parts that turn current into wanted.
//
// Semantics: --content is the page's target state. An element present in
// current but absent from wanted is deleted; an element without an id is
// created. The background is the one thing that cannot be expressed, so a
// changed <style> is an error rather than a silent no-op.
func diffPage(current, wanted pageDoc) (pageDiff, error) {
	var diff pageDiff

	if err := diffStyle(current, wanted); err != nil {
		return diff, err
	}

	currentByID := current.elementByID()
	// Pages carrying a placeholder are rejected on read (see readCurrentPage),
	// so one here can only be hand-authored content — and there is nothing it
	// could correctly mean: the object it stands for cannot be created, and a
	// page that really had one would never have reached the diff.
	for _, el := range wanted.Elements {
		if el.Tag == placeholderTag {
			return diff, contentError(
				"--content contains an <undefined> element; it is only the server's stand-in for an object it could not export, and pages carrying one cannot be edited by this command — use `slides +replace-slide`",
			)
		}
	}
	wantedIDs := map[string]bool{}
	for _, el := range wanted.Elements {
		if el.ID == "" {
			continue
		}
		if _, ok := currentByID[el.ID]; !ok {
			return diff, contentError(
				"element id %q in --content does not exist on slide %s; drop the id to create it as a new element, or re-read the page",
				el.ID, current.RootID,
			)
		}
		if wantedIDs[el.ID] {
			return diff, contentError("element id %q appears twice in --content", el.ID)
		}
		wantedIDs[el.ID] = true
	}

	// Surviving elements must keep their relative order: there is no move
	// operation, so a reorder cannot be expressed and must not be silently
	// dropped.
	if err := checkOrderPreserved(current.orderedIDs(), wanted.orderedIDs(), wantedIDs); err != nil {
		return diff, err
	}

	// Deletions first so later inserts land at the positions the caller meant.
	for _, el := range current.Elements {
		if el.ID != "" && !wantedIDs[el.ID] {
			diff.Parts = append(diff.Parts, replacePartOut{
				Action:  "block_replace",
				BlockID: el.ID,
				// An empty replacement deletes the block.
				Replacement: "",
			})
			diff.Deleted++
		}
	}

	for i, el := range wanted.Elements {
		switch {
		case el.ID == "":
			diff.Parts = append(diff.Parts, replacePartOut{
				Action:              "block_insert",
				Insertion:           el.Raw,
				InsertBeforeBlockID: nextSurvivingID(wanted.Elements, i, wantedIDs),
			})
			diff.Inserted++
		case el.Canon != currentByID[el.ID].Canon:
			diff.Parts = append(diff.Parts, replacePartOut{
				Action:      "block_replace",
				BlockID:     el.ID,
				Replacement: el.Raw,
			})
			diff.Replaced++
		}
	}

	notePart, err := diffNote(current, wanted)
	if err != nil {
		return diff, err
	}
	if notePart != nil {
		diff.Parts = append(diff.Parts, *notePart)
		if wanted.Note == nil {
			diff.NoteCleared = true
		} else {
			diff.NoteReplaced = true
		}
	}

	return diff, nil
}

// diffStyle rejects a background change instead of dropping it.
//
// <style> has no id of its own and the <fill> inside it carries an "f"-prefixed
// id, which the endpoint's block_id validation rejects. There is therefore no
// way to change a page's background through this path at all — saying so beats
// returning success with the background untouched.
func diffStyle(current, wanted pageDoc) error {
	currentStyle, wantedStyle := "", ""
	if current.Style != nil {
		currentStyle = current.Style.Canon
	}
	if wanted.Style != nil {
		wantedStyle = wanted.Style.Canon
	}
	if currentStyle == wantedStyle {
		return nil
	}
	if wanted.Style == nil {
		return contentError(
			"--content has no <style> but the slide has one; the page background cannot be changed through this command — copy the existing <style> over from `slides +xml-get` output",
		)
	}
	return contentError(
		"--content changes <style> (the page background), which this command cannot express: the background has no addressable element id — keep the <style> from `slides +xml-get` output unchanged",
	)
}

// diffNote turns a note change into a part, or reports that it cannot be made.
func diffNote(current, wanted pageDoc) (*replacePartOut, error) {
	currentNote, wantedNote := "", ""
	if current.Note != nil {
		currentNote = current.Note.Canon
	}
	if wanted.Note != nil {
		wantedNote = wanted.Note.Canon
	}
	if currentNote == wantedNote {
		return nil, nil
	}
	// Editing or clearing the note both go through the existing note block, so
	// the current page has to have one to address.
	if current.Note == nil || current.Note.ID == "" {
		return nil, contentError("slide %s has no addressable <note>; speaker notes cannot be changed through this command", current.RootID)
	}
	replacement := fmt.Sprintf("<note id=%q><content/></note>", current.Note.ID)
	if wanted.Note != nil {
		replacement = wanted.Note.Raw
	}
	return &replacePartOut{
		Action:      "block_replace",
		BlockID:     current.Note.ID,
		Replacement: replacement,
	}, nil
}

// checkOrderPreserved verifies the surviving ids appear in the same relative
// order on both sides.
func checkOrderPreserved(currentIDs, wantedIDs []string, surviving map[string]bool) error {
	kept := make([]string, 0, len(currentIDs))
	for _, id := range currentIDs {
		if surviving[id] {
			kept = append(kept, id)
		}
	}
	if len(kept) != len(wantedIDs) {
		// Length mismatch is already covered by the unknown-id and duplicate
		// checks in diffPage; nothing to add here.
		return nil
	}
	for i := range kept {
		if kept[i] != wantedIDs[i] {
			return contentError(
				"--content reorders existing elements (expected %s at position %d, got %s); this command cannot move elements — delete and re-insert them, or keep the original order",
				kept[i], i+1, wantedIDs[i],
			)
		}
	}
	return nil
}

// nextSurvivingID finds the id-bearing element that follows position i, so a
// new element is inserted at the position the caller wrote it in. An empty
// result means "append to the end of the page".
func nextSurvivingID(elements []pageNode, i int, surviving map[string]bool) string {
	for _, el := range elements[i+1:] {
		if el.ID != "" && surviving[el.ID] {
			return el.ID
		}
	}
	return ""
}
