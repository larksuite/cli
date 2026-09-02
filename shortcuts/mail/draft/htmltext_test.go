// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

func TestPlainTextFromHTML(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "strips inline style tag",
			html: `<html><head><style>body{color:red}</style></head><body><p>Hello</p></body></html>`,
			want: "Hello",
		},
		{
			name: "strips script tag",
			html: `<html><body><p>Before</p><script>alert("xss")</script><p>After</p></body></html>`,
			want: "Before\nAfter",
		},
		{
			name: "strips noscript tag",
			html: `<html><body><p>Visible</p><noscript>Fallback text</noscript></body></html>`,
			want: "Visible",
		},
		{
			name: "strips head and title",
			html: `<html><head><title>Page Title</title></head><body>Content</body></html>`,
			want: "Content",
		},
		{
			name: "plain text passthrough",
			html: `<div>Line one</div><div>Line two</div>`,
			want: "Line one\nLine two",
		},
		{
			name: "mixed non-text and text tags",
			html: `<html><head><style>.a{}</style><link rel="stylesheet"/><meta charset="utf-8"/></head><body><div>Only this</div></body></html>`,
			want: "Only this",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plainTextFromHTML(tt.html)
			if got != tt.want {
				t.Errorf("plainTextFromHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlainTextFromHTMLDeepNesting(t *testing.T) {
	// Build HTML with 10000 levels of nesting — would overflow the stack
	// with the old recursive implementation, and exceeds the 512-node open
	// element limit that x/net/html's parser enforces, so this exercises the
	// tokenizer fallback: block boundaries, skipped script content and
	// entity unescaping must still behave like the parsed path.
	const depth = 10_000
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString("<div>")
	}
	b.WriteString("<p>deep &amp; nested</p><script>alert(1)</script><p>end</p>")
	for i := 0; i < depth; i++ {
		b.WriteString("</div>")
	}
	got := plainTextFromHTML(b.String())
	if want := "deep & nested\nend"; got != want {
		t.Errorf("deep nesting: got %q, want %q", got, want)
	}
}

func TestPlainTextFromHTMLTokensKeepsBodyWithoutHeadEndTag(t *testing.T) {
	got := plainTextFromHTMLTokens("<html><head><title>T</title><meta charset=utf-8><body><p>Hello <b>world</b></p><style>p{}</style>bye")
	if want := "Hello world\nbye"; got != want {
		t.Errorf("plainTextFromHTMLTokens() = %q, want %q", got, want)
	}
}

// TestPlainTextFromHTMLFallbackMatchesParsedPath feeds the same document to
// the parsed path (shallow) and to the tokenizer fallback (the same document
// with a 600-level <template> wrapper that trips the parser's 512-node limit)
// and requires identical output, so the fallback cannot leak content the
// parser drops — notably anything inside <head>.
func TestPlainTextFromHTMLFallbackMatchesParsedPath(t *testing.T) {
	deepWrap := func(inner string) string {
		return strings.Repeat("<template>", 600) + inner + strings.Repeat("</template>", 600)
	}
	for _, test := range []struct {
		name    string
		shallow string
		deep    string
		want    string
	}{
		{
			name:    "template inside head is dropped",
			shallow: "<html><head><template>HIDDEN</template></head><body><p>VISIBLE</p></body></html>",
			deep:    "<html><head>" + deepWrap("HIDDEN") + "</head><body><p>VISIBLE</p></body></html>",
			want:    "VISIBLE",
		},
		{
			name:    "template inside body is kept",
			shallow: "<html><body><template>T</template><p>VISIBLE</p></body></html>",
			deep:    "<html><body>" + deepWrap("T") + "<p>VISIBLE</p></body></html>",
			want:    "T\nVISIBLE",
		},
		{
			name:    "omitted head end tag still drops head content",
			shallow: "<html><head><title>T</title><meta charset=utf-8><template>H</template><p>VISIBLE</p>",
			deep:    "<html><head><title>T</title><meta charset=utf-8>" + deepWrap("H") + "<p>VISIBLE</p>",
			want:    "VISIBLE",
		},
		{
			name:    "noframes inside head is dropped",
			shallow: "<html><head><noframes>NF</noframes></head><body>VISIBLE</body></html>",
			deep:    "<html><head><noframes>NF</noframes>" + deepWrap("") + "</head><body>VISIBLE</body></html>",
			want:    "VISIBLE",
		},
		{
			name:    "text directly in head implicitly ends head",
			shallow: "<html><head>STRAY<title>T</title></head><body>VISIBLE</body></html>",
			deep:    "<html><head>STRAY<title>T</title>" + deepWrap("") + "</head><body>VISIBLE</body></html>",
			want:    "STRAY VISIBLE",
		},
		{
			name:    "explicit body after head metadata",
			shallow: "<html><head><title>T</title><link rel=stylesheet href=x><meta charset=utf-8></head><body><p>Hello</p></body></html>",
			deep:    "<html><head><title>T</title><link rel=stylesheet href=x><meta charset=utf-8>" + deepWrap("") + "</head><body><p>Hello</p></body></html>",
			want:    "Hello",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := xhtml.Parse(strings.NewReader(test.shallow)); err != nil {
				t.Fatalf("shallow input unexpectedly rejected by the parser: %v", err)
			}
			if _, err := xhtml.Parse(strings.NewReader(test.deep)); err == nil {
				t.Fatal("deep input was accepted by the parser, so the fallback was not exercised")
			}
			if got := plainTextFromHTML(test.shallow); got != test.want {
				t.Errorf("parsed path = %q, want %q", got, test.want)
			}
			if got := plainTextFromHTML(test.deep); got != test.want {
				t.Errorf("fallback path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsHTMLNonTextTag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{"script", true},
		{"style", true},
		{"head", true},
		{"meta", true},
		{"noscript", true},
		{"link", true},
		{"title", true},
		{"div", false},
		{"p", false},
		{"span", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			n := &xhtml.Node{Type: xhtml.ElementNode, Data: tt.tag}
			if got := isHTMLNonTextTag(n); got != tt.want {
				t.Errorf("isHTMLNonTextTag(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestPlainTextFromHTMLExported(t *testing.T) {
	got := PlainTextFromHTML("<p>Hello world</p>")
	if !strings.Contains(got, "Hello world") {
		t.Fatalf("PlainTextFromHTML: expected to contain \"Hello world\", got %q", got)
	}
}
