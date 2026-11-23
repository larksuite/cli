// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"regexp"
	"strings"
	"testing"
)

// noteOpenTagRe grabs the <note> opening tag; noteIDInTagRe finds an id
// attribute (any quote style, any spacing) inside it.
var (
	noteOpenTagRe = regexp.MustCompile(`<note\b[^>]*>`)
	noteIDInTagRe = regexp.MustCompile(`(?:^|\s)id\s*=`)
)

// noteTagHasID reports whether the <note> opening tag in s still carries an id
// attribute. Asserting on the attribute itself — rather than on a specific id
// value disappearing — catches an implementation that swaps the id for another
// value instead of removing it.
func noteTagHasID(s string) bool {
	return noteIDInTagRe.MatchString(noteOpenTagRe.FindString(s))
}

func TestStripSlideNoteID(t *testing.T) {
	in := `<slide id="p1"><style><fill id="f1"><fillColor color="rgba(1,2,3,1)"/></fill></style><data>` +
		`<shape type="text" id="bKZ" topLeftX="80"><content textType="title"><p>x</p></content></shape>` +
		// embed carries an SVG with an internal gradient referenced by url(#id):
		`<embed id="bmU" width="35"><svg xmlns="http://www.w3.org/2000/svg" id="svgroot">` +
		`<defs><linearGradient id="grad"/></defs><path id="pp" d="M0 0" fill="url(#grad)"/><use href="#pp"/></svg></embed>` +
		`</data><note id="blw"><content><p>n</p></content></note></slide>`
	out := stripSlideNoteID(in)

	// The <note> tag no longer carries an id attribute at all.
	if noteTagHasID(out) {
		t.Errorf("expected note id attribute to be removed, got: %s", out)
	}
	// The note element itself and its content survive.
	for _, kept := range []string{`<note`, `<p>n</p>`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %s to survive, got: %s", kept, out)
		}
	}

	// Every visible element keeps its id (updated in place, not rebuilt).
	for _, kept := range []string{`id="p1"`, `id="f1"`, `id="bKZ"`, `id="bmU"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %s to be preserved, got: %s", kept, out)
		}
	}
	// SVG-subtree ids and their references are untouched.
	for _, kept := range []string{`id="svgroot"`, `id="grad"`, `id="pp"`, `fill="url(#grad)"`, `href="#pp"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %s to be preserved, got: %s", kept, out)
		}
	}
	// Non-id attributes and content survive.
	for _, kept := range []string{`type="text"`, `topLeftX="80"`, `width="35"`, `<p>x</p>`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %s to survive, got: %s", kept, out)
		}
	}
}

// A page with no <note> is passed through unchanged.
func TestStripSlideNoteIDNoNote(t *testing.T) {
	in := `<slide id="p1"><data><shape type="text" id="bKZ"><content><p>x</p></content></shape></data></slide>`
	if out := stripSlideNoteID(in); out != in {
		t.Errorf("expected unchanged output, got: %s", out)
	}
}

// The note id is stripped regardless of attribute order on the <note> tag.
func TestStripSlideNoteIDAttrOrder(t *testing.T) {
	in := `<slide id="p1"><data/><note foo="bar" id="blw" baz="qux"><content><p>n</p></content></note></slide>`
	out := stripSlideNoteID(in)
	if noteTagHasID(out) {
		t.Errorf("expected note id attribute to be removed, got: %s", out)
	}
	for _, kept := range []string{`foo="bar"`, `baz="qux"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected %s to survive, got: %s", kept, out)
		}
	}
}

// The id is stripped across the quote styles and spacing valid XML allows and
// the backend accepts — single quotes and whitespace around '='. A verified
// gap: a single-quoted stale note id used to slip through and reproduce the
// "block is not NoteBlock" crash this strip prevents.
func TestStripSlideNoteIDQuoteVariants(t *testing.T) {
	cases := []struct {
		name string
		note string
	}{
		{"single-quote", `<note id='blw'><content><p>n</p></content></note>`},
		{"space-around-eq", `<note id = "blw"><content><p>n</p></content></note>`},
		{"single-quote-space", `<note id =  'blw'><content><p>n</p></content></note>`},
		{"single-quote-attr-order", `<note foo='bar' id='blw'><content><p>n</p></content></note>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := stripSlideNoteID(`<slide id="p1"><data/>` + c.note + `</slide>`)
			if noteTagHasID(out) {
				t.Errorf("expected note id attribute to be removed, got: %s", out)
			}
			if !strings.Contains(out, "<note") || !strings.Contains(out, "<p>n</p>") {
				t.Errorf("expected note element and content to survive, got: %s", out)
			}
		})
	}
}

// Everything except the one note id must survive byte-for-byte: no
// re-serialization, so svg subtrees, quote styles, attribute order, whitespace,
// CDATA, and every other element are untouched. Asserting exact equality against
// "the input with only that id removed" proves nothing else moved.
func TestStripSlideNoteIDPreservesEverythingElse(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		removed string // the exact substring that must be the only change
	}{
		{
			name: "full slide with inline svg",
			in: `<slide id="p1"><style><fill id="f1"><fillColor color="rgba(1,2,3,1)"/></fill></style>` +
				`<data><shape type="text" id="bKZ"><content textType="title"><p>x</p></content></shape>` +
				`<embed id="bmU"><svg xmlns="http://www.w3.org/2000/svg" id="svgroot">` +
				`<defs><linearGradient id="grad"/></defs><path id="pp" d="M0 0" fill="url(#grad)"/><use href="#pp"/></svg></embed>` +
				`</data><note id="blw"><content><p>n</p></content></note></slide>`,
			removed: ` id="blw"`,
		},
		{
			name: "cdata and > attribute stay verbatim",
			in: `<slide id="p1"><data><shape type="text"><content><![CDATA[<note id="fake">]]></content></shape></data>` +
				`<note data=">" id="blw"><content><p>n</p></content></note></slide>`,
			removed: ` id="blw"`,
		},
		{
			name:    "single quotes",
			in:      `<slide id="p1"><data/><note id='blw'><content><p>n</p></content></note></slide>`,
			removed: ` id='blw'`,
		},
		{
			// &#32; (a numeric char ref) decodes to a space, but InputOffset is
			// an input-byte offset, so the note span must not drift even with
			// several of them ahead of the note. The refs stay verbatim.
			name: "numeric char refs before the note",
			in: `<slide id="p1"><data><shape type="text"><content><p>a&#32;b&#32;c</p></content></shape></data>` +
				`<note id="blw"><content><p>n</p></content></note></slide>`,
			removed: ` id="blw"`,
		},
		{
			// A char ref inside the id value itself: the whole attribute goes.
			name:    "char ref inside note id value",
			in:      `<slide id="p1"><data/><note id="a&#32;b"><content><p>n</p></content></note></slide>`,
			removed: ` id="a&#32;b"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(c.in, c.removed) != 1 {
				t.Fatalf("test setup: %q must appear exactly once in input", c.removed)
			}
			want := strings.Replace(c.in, c.removed, "", 1)
			if got := stripSlideNoteID(c.in); got != want {
				t.Errorf("only the note id should change.\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// Locating the <note> tag with the XML tokenizer (rather than a raw scan) means
// note-like text in comments/CDATA, a nested <note>, and '>' inside an attribute
// value are all handled correctly.
func TestStripSlideNoteIDXMLAware(t *testing.T) {
	t.Run("attr value contains >", func(t *testing.T) {
		in := `<slide id="p1"><data/><note data=">" id="blw"><content><p>n</p></content></note></slide>`
		out := stripSlideNoteID(in)
		if strings.Contains(out, `id="blw"`) {
			t.Errorf("expected note id to be stripped, got: %s", out)
		}
		if !strings.Contains(out, `data=">"`) {
			t.Errorf("expected the '>'-bearing attribute to survive, got: %s", out)
		}
	})

	t.Run("note-like text in CDATA is untouched", func(t *testing.T) {
		in := `<slide id="p1"><data><shape type="text"><content>` +
			`<![CDATA[see <note id="fake"> here]]>` +
			`</content></shape></data><note id="real"><content><p>n</p></content></note></slide>`
		out := stripSlideNoteID(in)
		if !strings.Contains(out, `id="fake"`) {
			t.Errorf("expected CDATA text to be preserved verbatim, got: %s", out)
		}
		if strings.Contains(out, `id="real"`) {
			t.Errorf("expected the real slide-level note id to be stripped, got: %s", out)
		}
	})

	t.Run("note-like text in a comment is untouched", func(t *testing.T) {
		in := `<slide id="p1"><!-- <note id="cmt"> --><data/><note id="real"><content><p>n</p></content></note></slide>`
		out := stripSlideNoteID(in)
		if !strings.Contains(out, `id="cmt"`) {
			t.Errorf("expected comment text to be preserved, got: %s", out)
		}
		if strings.Contains(out, `id="real"`) {
			t.Errorf("expected the real note id to be stripped, got: %s", out)
		}
	})

	t.Run("only a slide-level note is touched", func(t *testing.T) {
		in := `<slide id="p1"><data><group><note id="deep"/></group></data><note id="top"><content><p>n</p></content></note></slide>`
		out := stripSlideNoteID(in)
		if !strings.Contains(out, `id="deep"`) {
			t.Errorf("expected a non-slide-level note id to be preserved, got: %s", out)
		}
		if strings.Contains(out, `id="top"`) {
			t.Errorf("expected the slide-level note id to be stripped, got: %s", out)
		}
	})
}

// A single-quoted id on a visible element is left alone — only <note> is touched.
func TestStripSlideNoteIDLeavesSingleQuotedShape(t *testing.T) {
	in := `<slide id="p1"><data><shape id='bKZ' type='text'><content><p>x</p></content></shape></data><note id='blw'><content><p>n</p></content></note></slide>`
	out := stripSlideNoteID(in)
	if noteTagHasID(out) {
		t.Errorf("expected note id attribute to be removed, got: %s", out)
	}
	if !strings.Contains(out, `id='bKZ'`) {
		t.Errorf("expected shape id to be preserved, got: %s", out)
	}
}
