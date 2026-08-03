// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// currentPageXML is shaped like what the server actually returns: pretty
// printed, attributes in its own order, style defaults injected that the caller
// never wrote, and ids on the style fill ("f"-prefixed) and the note
// ("b"-prefixed).
const currentPageXML = `<slide id="piy">
  <style>
    <fill id="fiy">
      <fillColor color="rgba(255, 255, 255, 1)"/>
    </fill>
  </style>
  <data>
    <shape width="800" height="120" topLeftX="80" topLeftY="80" type="text" id="bbD">
      <content textType="title" fontSize="54" fontFamily="思源黑体">
        <p>BEFORE</p>
      </content>
    </shape>
    <shape width="400" height="80" topLeftX="80" topLeftY="260" type="text" id="bbv">
      <content fontSize="16" fontFamily="思源黑体">
        <p>SECOND</p>
      </content>
    </shape>
  </data>
  <note id="bbb">
    <content/>
  </note>
</slide>`

const currentStyleXML = `<style>
    <fill id="fiy">
      <fillColor color="rgba(255, 255, 255, 1)"/>
    </fill>
  </style>`

// wantPage assembles a page in the caller's own style, so the tests exercise
// the canonical comparison rather than string equality.
func wantPage(style, data, note string) string {
	return `<slide id="piy">` + style + `<data>` + data + `</data>` + note + `</slide>`
}

const (
	elemOne    = `<shape id="bbD" type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title" fontSize="54" fontFamily="思源黑体"><p>BEFORE</p></content></shape>`
	elemOneNew = `<shape id="bbD" type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title" fontSize="54" fontFamily="楷体"><p>BEFORE</p></content></shape>`
	elemTwo    = `<shape id="bbv" type="text" topLeftX="80" topLeftY="260" width="400" height="80"><content fontSize="16" fontFamily="思源黑体"><p>SECOND</p></content></shape>`
	noteKept   = `<note id="bbb"><content/></note>`
)

// registerPageRead stubs the read half. The write stub must be registered with
// Method POST, since httpmock matches the URL by substring and "/slide" is a
// prefix of "/slide/replace".
func registerPageRead(t *testing.T, reg *httpmock.Registry, pageXML string) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide":       map[string]interface{}{"slide_id": "piy", "content": pageXML},
				"revision_id": 7,
			},
		},
	})
}

func registerWriteStub(t *testing.T, reg *httpmock.Registry, revision int) *httpmock.Stub {
	t.Helper()
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": revision}},
	}
	reg.Register(stub)
	return stub
}

// forbidWrite registers a POST stub that fails the test if it is ever hit.
func forbidWrite(t *testing.T, reg *httpmock.Registry, why string) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/slide/replace",
		Optional: true,
		OnMatch:  func(*http.Request) { t.Error(why) },
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
}

func runUpdateSlide(t *testing.T, f *cmdutil.Factory, content string, extra ...string) error {
	t.Helper()
	args := append([]string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "piy",
		"--content", content,
	}, extra...)
	return runSlidesShortcut(t, f, nil, SlidesUpdateSlide, append(args, "--as", "user"))
}

func TestUpdateSlideDeclaredScopes(t *testing.T) {
	// The read scope is ENFORCED, not merely declared: every execution reads
	// the page before writing it, and ConditionalScopes would let a write-only
	// token reach the GET before failing.
	want := []string{"slides:presentation:read", "slides:presentation:update", "slides:presentation:write_only"}
	for _, sc := range []common.Shortcut{SlidesUpdateSlide, SlidesUpdate} {
		if got := sc.ScopesForIdentity("user"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s user preflight scopes = %#v, want %#v", sc.Command, got, want)
		}
		declared := sc.DeclaredScopesForIdentity("user")
		found := false
		for _, got := range declared {
			if got == "wiki:node:read" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s declared scopes %#v missing wiki:node:read", sc.Command, declared)
		}
	}
}

func TestUpdateSlideIsRegisteredWithAlias(t *testing.T) {
	canonical := findSlidesShortcut(t, "+update-slide")
	alias := findSlidesShortcut(t, "+update")

	if canonical.Hidden {
		t.Error("+update-slide must be visible in --help")
	}
	if !alias.Hidden {
		t.Error("+update alias must stay hidden so only the canonical name is advertised")
	}
	if alias.Service != canonical.Service || alias.Risk != canonical.Risk ||
		!reflect.DeepEqual(alias.AuthTypes, canonical.AuthTypes) ||
		!reflect.DeepEqual(alias.Scopes, canonical.Scopes) ||
		!reflect.DeepEqual(alias.ConditionalScopes, canonical.ConditionalScopes) ||
		!reflect.DeepEqual(alias.Flags, canonical.Flags) ||
		!reflect.DeepEqual(alias.Tips, canonical.Tips) ||
		alias.Description != canonical.Description {
		t.Error("alias metadata drifted from +update-slide; it must be derived, not re-declared")
	}
}

// TestUpdateSlideSendsOnePartPerChangedElement is the core contract: only what
// differs is touched, the part addresses the element by its own id, and the
// replacement carries the caller's bytes rather than a re-serialized form.
func TestUpdateSlideSendsOnePartPerChangedElement(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 8)

	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOneNew+elemTwo, noteKept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := decodeUpdateSlideParts(t, stub.CapturedBody)
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1 (only the restyled element differs): %#v", len(parts), parts)
	}
	if parts[0].Action != "block_replace" || parts[0].BlockID != "bbD" {
		t.Errorf("part = %+v, want block_replace on bbD", parts[0])
	}
	if !strings.Contains(parts[0].Replacement, `fontFamily="楷体"`) {
		t.Errorf("replacement lost the edit: %q", parts[0].Replacement)
	}
	if parts[0].Replacement != elemOneNew {
		t.Errorf("replacement should be the caller's exact bytes:\n got %q\nwant %q", parts[0].Replacement, elemOneNew)
	}

	data := decodeShortcutData(t, stdout)
	if data["parts_count"] != float64(1) || data["replaced"] != float64(1) ||
		data["inserted"] != float64(0) || data["deleted"] != float64(0) {
		t.Errorf("counters = %v", data)
	}
	if data["revision_id"] != float64(8) {
		t.Errorf("revision_id = %v, want 8", data["revision_id"])
	}
}

// TestUpdateSlideCanonicalComparison pins that the server's own formatting does
// not read as a change. The caller here reorders attributes, collapses the
// indentation and self-closes differently, but means the same page.
func TestUpdateSlideCanonicalComparison(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	forbidWrite(t, reg, "an unchanged page must not be written")

	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+elemTwo, noteKept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	if data["unchanged"] != true || data["parts_count"] != float64(0) {
		t.Fatalf("an identical page should report unchanged, got %v", data)
	}
}

func TestUpdateSlideDeletesDroppedElements(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 9)

	// bbv is dropped from the target page.
	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne, noteKept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := decodeUpdateSlideParts(t, stub.CapturedBody)
	if len(parts) != 1 {
		t.Fatalf("parts = %#v, want a single delete", parts)
	}
	if parts[0].BlockID != "bbv" || parts[0].Replacement != "" {
		t.Errorf("part = %+v, want an empty replacement on bbv (delete)", parts[0])
	}
	if data := decodeShortcutData(t, stdout); data["deleted"] != float64(1) {
		t.Errorf("deleted = %v, want 1", data["deleted"])
	}
}

func TestUpdateSlideInsertsNewElementsInPlace(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 10)

	// A new element without an id, written between the two existing ones.
	fresh := `<img src="tok" topLeftX="500" topLeftY="100" width="200" height="150"/>`
	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+fresh+elemTwo, noteKept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := decodeUpdateSlideParts(t, stub.CapturedBody)
	if len(parts) != 1 || parts[0].Action != "block_insert" {
		t.Fatalf("parts = %#v, want a single block_insert", parts)
	}
	if parts[0].Insertion != fresh {
		t.Errorf("insertion = %q, want the caller's bytes", parts[0].Insertion)
	}
	// Position is expressed by naming the element it precedes; without it the
	// new element would land at the end of the page.
	if parts[0].InsertBeforeBlockID != "bbv" {
		t.Errorf("insert_before_block_id = %q, want bbv", parts[0].InsertBeforeBlockID)
	}
	if data := decodeShortcutData(t, stdout); data["inserted"] != float64(1) {
		t.Errorf("inserted = %v, want 1", data["inserted"])
	}
}

func TestUpdateSlideAppendsWhenNewElementIsLast(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 11)

	fresh := `<img src="tok" width="100" height="100"/>`
	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+elemTwo+fresh, noteKept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := decodeUpdateSlideParts(t, stub.CapturedBody)
	if len(parts) != 1 || parts[0].InsertBeforeBlockID != "" {
		t.Fatalf("a trailing new element must be appended, got %#v", parts)
	}
}

// TestUpdateSlideRejectsBackgroundChange is the honest-failure case. The
// background has no addressable element id, so the only alternative to an error
// is returning success with the background untouched.
func TestUpdateSlideRejectsBackgroundChange(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		style    string
		wantWord string
	}{
		{
			name:     "changed_fill",
			style:    `<style><fill id="fiy"><fillColor color="rgba(255, 0, 0, 1)"/></fill></style>`,
			wantWord: "background",
		},
		{
			name:     "style_dropped",
			style:    ``,
			wantWord: "background",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			registerPageRead(t, reg, currentPageXML)
			forbidWrite(t, reg, "a background change must be rejected before any write")

			err := runUpdateSlide(t, f, wantPage(tt.style, elemOne+elemTwo, noteKept))
			if err == nil {
				t.Fatal("expected a validation error")
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
			}
			if ve.Param != "--content" || !strings.Contains(ve.Message, tt.wantWord) {
				t.Fatalf("error should name --content and mention the background, got %q (param %q)", ve.Message, ve.Param)
			}
		})
	}
}

func TestUpdateSlideHandlesNote(t *testing.T) {
	t.Parallel()

	t.Run("replaced", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		registerPageRead(t, reg, currentPageXML)
		stub := registerWriteStub(t, reg, 12)

		newNote := `<note id="bbb"><content><p>talk track</p></content></note>`
		err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+elemTwo, newNote))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parts := decodeUpdateSlideParts(t, stub.CapturedBody)
		if len(parts) != 1 || parts[0].BlockID != "bbb" || parts[0].Replacement != newNote {
			t.Fatalf("parts = %#v, want a block_replace on the note id", parts)
		}
		if data := decodeShortcutData(t, stdout); data["note_replaced"] != true {
			t.Errorf("note_replaced = %v, want true", data["note_replaced"])
		}
	})

	t.Run("cleared_when_omitted", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		// A page whose note has content, so omitting <note> is a real change.
		withNote := strings.Replace(currentPageXML, `<note id="bbb">
    <content/>
  </note>`, `<note id="bbb"><content><p>old note</p></content></note>`, 1)
		registerPageRead(t, reg, withNote)
		stub := registerWriteStub(t, reg, 13)

		err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+elemTwo, ""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parts := decodeUpdateSlideParts(t, stub.CapturedBody)
		if len(parts) != 1 || parts[0].BlockID != "bbb" {
			t.Fatalf("parts = %#v, want the note cleared", parts)
		}
		if !strings.Contains(parts[0].Replacement, "<content/>") {
			t.Errorf("replacement = %q, want an empty note", parts[0].Replacement)
		}
		if data := decodeShortcutData(t, stdout); data["note_cleared"] != true {
			t.Errorf("note_cleared = %v, want true", data["note_cleared"])
		}
	})
}

// TestUpdateSlideRejectsUnexpressibleEdits covers the edits that element-level
// parts cannot describe. Each one must fail before any write rather than land
// partially.
func TestUpdateSlideRejectsUnexpressibleEdits(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		content  string
		wantWord string
	}{
		{
			name:     "reordered_elements",
			content:  wantPage(currentStyleXML, elemTwo+elemOne, noteKept),
			wantWord: "reorders",
		},
		{
			name:     "unknown_element_id",
			content:  wantPage(currentStyleXML, elemOne+elemTwo+`<shape id="bZZ" type="text"><content/></shape>`, noteKept),
			wantWord: "does not exist",
		},
		{
			name:     "duplicate_element_id",
			content:  wantPage(currentStyleXML, elemOne+elemOne+elemTwo, noteKept),
			wantWord: "twice",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			registerPageRead(t, reg, currentPageXML)
			forbidWrite(t, reg, "an unexpressible edit must be rejected before any write")

			err := runUpdateSlide(t, f, tt.content)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
			}
			if !strings.Contains(p.Message, tt.wantWord) {
				t.Fatalf("error message = %q, want it to mention %q", p.Message, tt.wantWord)
			}
		})
	}
}

// TestUpdateSlideRejectsBadContentBeforeReading pins that malformed input fails
// without spending an API call.
func TestUpdateSlideRejectsBadContentBeforeReading(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		content  string
		wantWord string
	}{
		{name: "element_root", content: `<shape type="text"><content/></shape>`, wantWord: "+replace-slide"},
		{name: "presentation_root", content: `<presentation><slide id="piy"><data/></slide></presentation>`, wantWord: "+replace-pages"},
		{name: "second_page", content: `<slide id="p1"><data/></slide><slide id="p2"><data/></slide>`, wantWord: "one page per call"},
		{name: "trailing_text", content: `<slide id="p1"><data/></slide>oops`, wantWord: "text after"},
		{name: "not_xml", content: `not xml at all`, wantWord: "no root element"},
		{name: "unclosed", content: `<slide id="p1"><data>`, wantWord: "well-formed"},
		{name: "blank", content: `   `, wantWord: "cannot be empty"},
		// Slide-level structure the diff cannot represent: accepting any of
		// these would silently drop the edit — and could even answer
		// `unchanged` for a change the caller asked for.
		{name: "unknown_slide_child", content: `<slide id="piy"><data/><foo requestedChange="true"/></slide>`, wantWord: "unknown <foo>"},
		{name: "duplicate_data", content: `<slide id="piy"><data/><data><shape type="text"><content/></shape></data></slide>`, wantWord: "second <data>"},
		{name: "duplicate_style", content: `<slide id="piy"><style/><style/><data/></slide>`, wantWord: "second <style>"},
		{name: "duplicate_note", content: `<slide id="piy"><data/><note><content/></note><note><content/></note></slide>`, wantWord: "second <note>"},
		{name: "text_in_slide", content: `<slide id="piy"><data/>stray</slide>`, wantWord: "text directly inside <slide>"},
		{name: "text_in_data", content: `<slide id="piy"><data><shape type="text"><content/></shape>stray</data></slide>`, wantWord: "text directly inside <data>"},
		// XML fetched for page A posted against page B: the root id is the
		// only reliable cross-check — element-id checks catch it merely
		// incidentally, and an empty page would sail through them.
		{name: "root_id_mismatch", content: `<slide id="pother"><data/></slide>`, wantWord: "read from a different page"},
		// Container attributes have no element-level part to travel in, so
		// accepting them means an edit that silently vanishes into unchanged.
		{name: "root_attr", content: `<slide id="piy" requestedChange="true"><data/></slide>`, wantWord: `unsupported attribute "requestedChange" on <slide>`},
		{name: "data_attr", content: `<slide id="piy"><data requestedChange="true"/></slide>`, wantWord: `unsupported attribute "requestedChange" on <data>`},
		// Namespace declarations are not a loophole: an inherited binding
		// changes what every descendant name means, and the canonicalizer
		// compares local names, so a binding-only edit would vanish into
		// unchanged. Only the official SML default namespace may appear, and
		// only on the root.
		{name: "wrong_default_xmlns", content: `<slide xmlns="urn:not-sml" id="piy"><data/></slide>`, wantWord: `unsupported xmlns "urn:not-sml"`},
		{name: "prefixed_xmlns_on_slide", content: `<slide xmlns:x="urn:a" id="piy"><data/></slide>`, wantWord: `prefixed namespace declaration "xmlns:x"`},
		{name: "xmlns_on_data", content: `<slide id="piy"><data xmlns="http://www.larkoffice.com/sml/2.0"/></slide>`, wantWord: `on <data>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			reg.Register(&httpmock.Stub{
				Method:   "GET",
				URL:      "/slide",
				Optional: true,
				OnMatch:  func(*http.Request) { t.Error("malformed --content must fail before reading the page") },
				Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
			})

			err := runUpdateSlide(t, f, tt.content)
			if err == nil {
				t.Fatal("expected a validation error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
			}
			if !strings.Contains(p.Message, tt.wantWord) {
				t.Fatalf("error message = %q, want it to mention %q", p.Message, tt.wantWord)
			}
		})
	}
}

// TestUpdateSlideRejectsUnsupportedCurrentPage covers the read side of the
// structure guard: a page whose slide-level structure the diff cannot see
// cannot be safely edited, because the comparison could neither preserve the
// unrecognized part nor notice the caller changing it.
func TestUpdateSlideRejectsUnsupportedCurrentPage(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, `<slide id="piy"><style/><data><shape id="bbD" type="text"><content/></shape></data><transition type="fade"/><note id="bbb"><content/></note></slide>`)
	forbidWrite(t, reg, "a page the diff cannot represent must never be written")

	err := runUpdateSlide(t, f, wantPage("<style/>", elemOne, noteKept))
	if err == nil {
		t.Fatal("expected an error for a page with unrepresentable structure")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition (the page state, not the flag, is the problem)", ve.Subtype)
	}
	if !strings.Contains(ve.Message, "<transition>") || !strings.Contains(ve.Message, "cannot represent") {
		t.Errorf("message should name the structure: %q", ve.Message)
	}
}

// TestUpdateSlideRootIDMayBeOmitted pins the other half of the root-id rule:
// only a non-empty mismatching id is rejected; hand-written pages without one
// apply normally.
func TestUpdateSlideRootIDMayBeOmitted(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 23)

	err := runUpdateSlide(t, f, `<slide>`+currentStyleXML+`<data>`+elemOneNew+elemTwo+`</data>`+noteKept+`</slide>`)
	if err != nil {
		t.Fatalf("a missing root id must be allowed: %v", err)
	}
	if parts := decodeUpdateSlideParts(t, stub.CapturedBody); len(parts) != 1 || parts[0].BlockID != "bbD" {
		t.Fatalf("parts = %#v, want the single font change", parts)
	}
}

// TestUpdateSlideCanonicalAttrCollision is the regression for ambiguous
// canonical encoding: with bare name=value concatenation these two elements
// canonicalize identically, and the edit would be dropped as unchanged.
func TestUpdateSlideCanonicalAttrCollision(t *testing.T) {
	t.Parallel()

	a, err := canonicalizeElement(`<img id="bbD" alt="foo rotateWithShape=true" src="x"/>`)
	if err != nil {
		t.Fatalf("canonicalize a: %v", err)
	}
	b, err := canonicalizeElement(`<img id="bbD" alt="foo" rotateWithShape="true" src="x"/>`)
	if err != nil {
		t.Fatalf("canonicalize b: %v", err)
	}
	if a == b {
		t.Fatalf("distinct attribute sets must not canonicalize identically:\n%s", a)
	}

	// And through the whole command: the change must become a replacement part.
	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, `<slide id="piy"><style/><data><img id="bbD" alt="foo rotateWithShape=true" src="x"/></data><note id="bbb"><content/></note></slide>`)
	stub := registerWriteStub(t, reg, 24)

	err = runUpdateSlide(t, f, `<slide id="piy"><style/><data><img id="bbD" alt="foo" rotateWithShape="true" src="x"/></data><note id="bbb"><content/></note></slide>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts := decodeUpdateSlideParts(t, stub.CapturedBody)
	if len(parts) != 1 || parts[0].BlockID != "bbD" {
		t.Fatalf("the attribute change must produce a replacement, got %#v", parts)
	}
}

// TestUpdateSlidePlaceholderRefusal pins the conservative contract for
// <undefined> placeholders — the server's stand-ins for objects it cannot
// export (a whiteboard read without its export option, video/audio embeds).
// Whether the whole-page rewrite preserves an untouched one is a server-owned
// behavior no self-contained test can pin down (boards cannot be created
// programmatically), so pages carrying one are refused outright rather than
// edited on an unverifiable assumption.
func TestUpdateSlidePlaceholderRefusal(t *testing.T) {
	t.Parallel()

	currentWithBoard := `<slide id="piy"><style/><data>` + elemOne + `<undefined id="bbW" type="whiteboard"/></data><note id="bbb"><content/></note></slide>`

	t.Run("page_with_placeholder_is_refused", func(t *testing.T) {
		f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		registerPageRead(t, reg, currentWithBoard)
		forbidWrite(t, reg, "a page carrying a placeholder must never be written")

		// Even an edit that does not touch the placeholder is refused: the
		// rewrite behind the endpoint is whole-page, and preservation of the
		// placeholder is exactly what cannot be proven.
		err := runUpdateSlide(t, f, `<slide id="piy"><style/><data>`+elemOneNew+`<undefined id="bbW" type="whiteboard"/></data><note id="bbb"><content/></note></slide>`)
		if err == nil {
			t.Fatal("expected a refusal")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
		}
		if ve.Subtype != errs.SubtypeFailedPrecondition {
			t.Errorf("subtype = %q, want failed_precondition (the page state, not the flag, is the problem)", ve.Subtype)
		}
		if !strings.Contains(ve.Message, "<undefined>") || !strings.Contains(ve.Message, "bbW") || !strings.Contains(ve.Message, "+replace-slide") {
			t.Errorf("message should name the placeholder and the escape hatch: %q", ve.Message)
		}
	})

	t.Run("placeholder_in_content_is_refused", func(t *testing.T) {
		f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		registerPageRead(t, reg, currentPageXML) // a clean page
		forbidWrite(t, reg, "hand-authored placeholders must be rejected before any write")

		err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOne+`<undefined type="whiteboard"/>`, noteKept))
		if err == nil {
			t.Fatal("expected a validation error")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
		}
		if ve.Param != "--content" || !strings.Contains(ve.Message, "<undefined>") {
			t.Fatalf("error = %q (param %q)", ve.Message, ve.Param)
		}
	})
}

// TestUpdateSlideAcceptsSMLNamespaces pins that every namespace form the
// repository's own SXSD validator accepts — the official identifier plus the
// two server read-back spellings — round-trips on both sides of the diff. The
// primary workflow copies `+xml-get` output back in, so rejecting a read-back
// form would break the feature's core contract.
func TestUpdateSlideAcceptsSMLNamespaces(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{
		"http://www.larkoffice.com/sml/2.0",
		"https://www.larkoffice.com/sml/2.0",
		"/sml/2.0",
	} {
		t.Run(ns, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			// The server itself may return the namespace on the read side.
			currentNS := `<slide xmlns="` + ns + `" id="piy">` + currentStyleXML + `<data>` + elemOne + elemTwo + `</data>` + noteKept + `</slide>`
			registerPageRead(t, reg, currentNS)
			stub := registerWriteStub(t, reg, 25)

			err := runUpdateSlide(t, f, `<slide xmlns="`+ns+`" id="piy">`+currentStyleXML+`<data>`+elemOneNew+elemTwo+`</data>`+noteKept+`</slide>`)
			if err != nil {
				t.Fatalf("namespace %q must be accepted on both sides: %v", ns, err)
			}
			if parts := decodeUpdateSlideParts(t, stub.CapturedBody); len(parts) != 1 || parts[0].BlockID != "bbD" {
				t.Fatalf("parts = %#v, want the single font change", parts)
			}
		})
	}
}

// TestUpdateSlideDetectsTextEdits is the regression for lossy text
// canonicalization. Both edits were previously reported as `unchanged`: the
// whitespace-only node between inline runs was trimmed away (dropping a
// preserved &#32; space, which decodes to the same token as a literal one),
// and unescaped text let literal markup collide with real elements.
func TestUpdateSlideDetectsTextEdits(t *testing.T) {
	t.Parallel()

	page := func(paragraph string) string {
		return `<slide id="piy"><style/><data><shape id="bbD" type="text"><content>` + paragraph + `</content></shape></data><note id="bbb"><content/></note></slide>`
	}
	for _, tt := range []struct {
		name    string
		current string
		wanted  string
	}{
		{
			name:    "space_between_inline_runs",
			current: page(`<p><strong>Hello</strong> <em>world</em></p>`),
			wanted:  page(`<p><strong>Hello</strong><em>world</em></p>`),
		},
		{
			name:    "escaped_literal_markup_vs_elements",
			current: page(`<p>&lt;/p&gt;&lt;p&gt;</p>`),
			wanted:  page(`<p></p><p></p>`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The pair must not canonicalize identically…
			a, err := canonicalizeElement(tt.current)
			if err != nil {
				t.Fatalf("canonicalize current: %v", err)
			}
			b, err := canonicalizeElement(tt.wanted)
			if err != nil {
				t.Fatalf("canonicalize wanted: %v", err)
			}
			if a == b {
				t.Fatalf("distinct content must not canonicalize identically:\n%s", a)
			}

			// …and the edit must become a replacement, never `unchanged`.
			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			registerPageRead(t, reg, tt.current)
			stub := registerWriteStub(t, reg, 26)

			if err := runUpdateSlide(t, f, tt.wanted); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			parts := decodeUpdateSlideParts(t, stub.CapturedBody)
			if len(parts) != 1 || parts[0].Action != "block_replace" || parts[0].BlockID != "bbD" {
				t.Fatalf("the text edit must produce a replacement, got %#v", parts)
			}
			if data := decodeShortcutData(t, stdout); data["unchanged"] == true {
				t.Fatal("a real text edit was reported as unchanged")
			}
		})
	}
}

// TestUpdateSlideFailedReasonIsAFailure pins that a rejected batch is reported
// as a command failure rather than as a field on a success envelope.
func TestUpdateSlideFailedReasonIsAFailure(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"failed_reason": "block with id 'bbD' not found"},
		},
	})

	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOneNew+elemTwo, noteKept))
	if err == nil {
		t.Fatal("a rejected batch must fail the command")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI || !strings.Contains(p.Message, "not found") {
		t.Fatalf("error = %+v, want an API error carrying the backend reason", p)
	}
}

func TestUpdateSlideDryRunShowsBothCalls(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/slide",
		Optional: true,
		OnMatch:  func(*http.Request) { t.Error("--dry-run must not call the API") },
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "piy",
		"--content", wantPage(currentStyleXML, elemOne, noteKept),
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// Both halves must be visible: the parts cannot be, because they depend on
	// the page's current state and dry-run must not fetch it.
	if !strings.Contains(out, `"GET"`) || !strings.Contains(out, `"POST"`) {
		t.Errorf("dry-run should show the read and the write: %s", out)
	}
	if !strings.Contains(out, "/slide/replace") {
		t.Errorf("dry-run missing the write endpoint: %s", out)
	}
}

func TestUpdateSlideForwardsRevisionAndTID(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var readQuery, writeQuery string
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		OnMatch: func(req *http.Request) { readQuery = req.URL.RawQuery },
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"slide": map[string]interface{}{"content": currentPageXML}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/slide/replace",
		OnMatch: func(req *http.Request) { writeQuery = req.URL.RawQuery },
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 20}},
	})

	err := runUpdateSlide(t, f, wantPage(currentStyleXML, elemOneNew+elemTwo, noteKept),
		"--revision-id", "19", "--tid", "tid_9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The same revision is used for both calls so the parts apply to the
	// snapshot they were computed from.
	for name, query := range map[string]string{"read": readQuery, "write": writeQuery} {
		if !strings.Contains(query, "revision_id=19") {
			t.Errorf("%s query = %q, want revision_id=19", name, query)
		}
		if !strings.Contains(query, "tid=tid_9") {
			t.Errorf("%s query = %q, want tid=tid_9", name, query)
		}
	}
}

func TestUpdateSlideAliasSharesBehavior(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 21)

	err := runSlidesShortcut(t, f, nil, SlidesUpdate, []string{
		"+update",
		"--presentation", "pres_abc",
		"--slide-id", "piy",
		"--content", wantPage(currentStyleXML, elemOneNew+elemTwo, noteKept),
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parts := decodeUpdateSlideParts(t, stub.CapturedBody); len(parts) != 1 || parts[0].BlockID != "bbD" {
		t.Fatalf("alias sent %#v, want the same single part", parts)
	}
}

func TestUpdateSlideAcceptsContentFlagAlias(t *testing.T) {
	t.Parallel()

	sc := findSlidesShortcut(t, "+update-slide")
	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	registerPageRead(t, reg, currentPageXML)
	stub := registerWriteStub(t, reg, 22)

	err := runSlidesShortcut(t, f, nil, sc, []string{
		"+update-slide",
		"--token", "pres_abc",
		"--slide-id", "piy",
		"--xml", wantPage(currentStyleXML, elemOneNew+elemTwo, noteKept),
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("--token / --xml aliases should resolve: %v", err)
	}
	if parts := decodeUpdateSlideParts(t, stub.CapturedBody); len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
}

// findSlidesShortcut returns the registered shortcut for command, failing the
// test when it is not wired into Shortcuts().
func findSlidesShortcut(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, sc := range Shortcuts() {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("shortcut %s is not registered in Shortcuts()", command)
	return common.Shortcut{}
}

type updateSlidePart struct {
	Action              string `json:"action"`
	BlockID             string `json:"block_id"`
	Replacement         string `json:"replacement"`
	Insertion           string `json:"insertion"`
	InsertBeforeBlockID string `json:"insert_before_block_id"`
}

func decodeUpdateSlideParts(t *testing.T, raw []byte) []updateSlidePart {
	t.Helper()
	var body struct {
		Parts []updateSlidePart `json:"parts"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body: %v\nraw=%s", err, raw)
	}
	return body.Parts
}
