// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePresentationRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantKind  string
		wantToken string
		wantErr   string
	}{
		{name: "raw token", input: "slidesXXXXXXXXXXXXXXXXXXXXXX", wantKind: "slides", wantToken: "slidesXXXXXXXXXXXXXXXXXXXXXX"},
		{name: "slides URL", input: "https://x.feishu.cn/slides/abc123", wantKind: "slides", wantToken: "abc123"},
		{name: "slides URL with query", input: "https://x.feishu.cn/slides/abc123?from=share", wantKind: "slides", wantToken: "abc123"},
		{name: "slides URL with anchor", input: "https://x.feishu.cn/slides/abc123#p1", wantKind: "slides", wantToken: "abc123"},
		{name: "wiki URL", input: "https://x.feishu.cn/wiki/wikcn123", wantKind: "wiki", wantToken: "wikcn123"},
		{name: "trims whitespace", input: "  abc123  ", wantKind: "slides", wantToken: "abc123"},
		{name: "empty", input: "", wantErr: "cannot be empty"},
		{name: "blank", input: "   ", wantErr: "cannot be empty"},
		{name: "unsupported url", input: "https://x.feishu.cn/docx/foo", wantErr: "unsupported"},
		{name: "unsupported path", input: "foo/bar", wantErr: "unsupported"},
		// Regression: /slides/ inside a query string must NOT be treated as a slides marker.
		{name: "slides marker inside query", input: "https://x.feishu.cn/docx/foo?next=/slides/abc", wantErr: "unsupported"},
		// Regression: /wiki/ as a path segment but not a prefix must not match.
		{name: "wiki marker mid-path", input: "https://x.feishu.cn/docx/wiki/wikcn123", wantErr: "unsupported"},
		// Regression: bare relative path containing wiki/ is not a wiki ref.
		{name: "non-url wiki segment", input: "tmp/wiki/wikcn123", wantErr: "unsupported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePresentationRef(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != tt.wantKind || got.Token != tt.wantToken {
				t.Fatalf("got = %+v, want kind=%s token=%s", got, tt.wantKind, tt.wantToken)
			}
		})
	}
}

func TestEnsureShapeHasContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "self-closing shape gets content injected",
			in:   `<shape type="rect" width="100" height="50"/>`,
			want: `<shape type="rect" width="100" height="50"><content/></shape>`,
		},
		{
			name: "self-closing shape with id already injected",
			in:   `<shape type="rect" width="100" height="50" id="bUn"/>`,
			want: `<shape type="rect" width="100" height="50" id="bUn"><content/></shape>`,
		},
		{
			// If the user already wrote non-content children, injecting
			// <content/> as a sibling would make <p> a sibling of <content>
			// (schema-legal but semantically wrong per SML 2.0, which
			// requires <p> to live inside <content>). Leave that case to
			// the backend's 3350001 rather than silently rewrap.
			name: "open shape with non-content children is left untouched",
			in:   `<shape type="text"><p>hello</p></shape>`,
			want: `<shape type="text"><p>hello</p></shape>`,
		},
		{
			name: "empty open shape gets content injected",
			in:   `<shape type="text"></shape>`,
			want: `<shape type="text"><content/></shape>`,
		},
		{
			name: "shape with content already present is unchanged",
			in:   `<shape type="text"><content><p>hi</p></content></shape>`,
			want: `<shape type="text"><content><p>hi</p></content></shape>`,
		},
		{
			name: "shape with self-closing content is unchanged",
			in:   `<shape type="rect"><content/></shape>`,
			want: `<shape type="rect"><content/></shape>`,
		},
		{
			name: "img self-closing is not touched",
			in:   `<img src="tok_abc" width="100" height="80"/>`,
			want: `<img src="tok_abc" width="100" height="80"/>`,
		},
		{
			name: "img open tag is not touched",
			in:   `<img src="tok_abc" width="100" height="80"><crop/></img>`,
			want: `<img src="tok_abc" width="100" height="80"><crop/></img>`,
		},
		{
			name: "table is not touched",
			in:   `<table rows="3" cols="3"/>`,
			want: `<table rows="3" cols="3"/>`,
		},
		{
			name: "bare self-closing shape",
			in:   `<shape/>`,
			want: `<shape><content/></shape>`,
		},
		{
			name: "shape with trailing space before self-close",
			in:   `<shape type="rect" />`,
			want: `<shape type="rect"><content/></shape>`,
		},
		{
			// Regression: strings.Contains("<content") used to false-match tags
			// like <contention/> that merely start with "content". The regex
			// now requires the char after "content" to be \s, / or >, so the
			// shape is correctly classified as having no <content> child.
			// Even so, we don't inject — <contention/> counts as an existing
			// non-content child (same rule as the <p> case above), so the
			// shape is left untouched for the backend to reject.
			name: "shape with contention child is left untouched",
			in:   `<shape type="text"><contention/></shape>`,
			want: `<shape type="text"><contention/></shape>`,
		},
		{
			name: "malformed input returned as-is",
			in:   `not xml at all`,
			want: `not xml at all`,
		},
		{
			name: "empty string returned as-is",
			in:   ``,
			want: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ensureShapeHasContent(tt.in)
			if got != tt.want {
				t.Fatalf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestExtractImagePlaceholderPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no placeholders",
			in:   []string{`<slide><data><img src="https://x.com/a.png"/></data></slide>`},
			want: nil,
		},
		{
			name: "single placeholder",
			in:   []string{`<slide><data><img src="@./pic.png" topLeftX="10"/></data></slide>`},
			want: []string{"./pic.png"},
		},
		{
			name: "single quotes",
			in:   []string{`<img src='@./a.png'/>`},
			want: []string{"./a.png"},
		},
		{
			name: "dedup across slides",
			in: []string{
				`<slide><data><img src="@./shared.png"/></data></slide>`,
				`<slide><data><img src="@./shared.png" topLeftX="100"/><img src="@./other.png"/></data></slide>`,
			},
			want: []string{"./shared.png", "./other.png"},
		},
		{
			name: "ignores non-img src",
			in:   []string{`<icon src="@./fake.png"/><img src="@./real.png"/>`},
			want: []string{"./real.png"},
		},
		{
			name: "preserves order of first occurrence",
			in:   []string{`<img src="@b.png"/><img src="@a.png"/><img src="@b.png"/>`},
			want: []string{"b.png", "a.png"},
		},
		{
			// Regression: Go RE2 has no backreferences, so the regex captures
			// opening and closing quotes independently. Mismatched pairs must
			// be filtered out post-match instead of producing bogus paths.
			name: "rejects mismatched quotes",
			in:   []string{`<img src="@./oops.png'/>`},
			want: nil,
		},
		{
			// Regression: XML allows whitespace around `=`; placeholders in
			// `src = "@..."` form must still be detected.
			name: "tolerates whitespace around equals",
			in:   []string{`<img src = "@./spaced.png" />`},
			want: []string{"./spaced.png"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractImagePlaceholderPaths(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceImagePlaceholders(t *testing.T) {
	t.Parallel()

	tokens := map[string]string{
		"./pic.png": "tok_abc",
		"./b.png":   "tok_b",
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single replacement preserves siblings",
			in:   `<img src="@./pic.png" topLeftX="10" width="100"/>`,
			want: `<img src="tok_abc" topLeftX="10" width="100"/>`,
		},
		{
			name: "multiple replacements",
			in:   `<img src="@./pic.png"/><img src="@./b.png"/>`,
			want: `<img src="tok_abc"/><img src="tok_b"/>`,
		},
		{
			name: "single quotes",
			in:   `<img src='@./pic.png'/>`,
			want: `<img src='tok_abc'/>`,
		},
		{
			name: "leaves unknown placeholder untouched",
			in:   `<img src="@./missing.png"/>`,
			want: `<img src="@./missing.png"/>`,
		},
		{
			name: "leaves http url alone",
			in:   `<img src="https://x.com/a.png"/>`,
			want: `<img src="https://x.com/a.png"/>`,
		},
		{
			name: "leaves bare token alone",
			in:   `<img src="existing_token"/>`,
			want: `<img src="existing_token"/>`,
		},
		{
			// Regression: placeholders with whitespace around `=` must be
			// rewritten too (XML permits the form). Surrounding whitespace
			// is preserved so the rewritten attribute reads naturally.
			name: "tolerates whitespace around equals",
			in:   `<img src = "@./pic.png" topLeftX="10"/>`,
			want: `<img src = "tok_abc" topLeftX="10"/>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := replaceImagePlaceholders(tt.in, tokens)
			if got != tt.want {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestEnsureXMLRootID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantOut string
		wantErr string
	}{
		{
			name:    "injects id when absent on self-closing tag",
			in:      `<shape type="rect" width="100" height="50"/>`,
			want:    "bUn",
			wantOut: `<shape type="rect" width="100" height="50" id="bUn"/>`,
		},
		{
			name:    "injects id when absent on open tag",
			in:      `<shape type="text"><content><p>hi</p></content></shape>`,
			want:    "bUn",
			wantOut: `<shape type="text" id="bUn"><content><p>hi</p></content></shape>`,
		},
		{
			name:    "leaves id alone when already matching",
			in:      `<shape id="bUn" type="rect"/>`,
			want:    "bUn",
			wantOut: `<shape id="bUn" type="rect"/>`,
		},
		{
			name:    "overrides mismatched id value preserving quotes and attrs",
			in:      `<shape id="xxx" type="rect"/>`,
			want:    "bUn",
			wantOut: `<shape id="bUn" type="rect"/>`,
		},
		{
			name:    "overrides single-quoted id",
			in:      `<shape id='xxx' type='rect'/>`,
			want:    "bUn",
			wantOut: `<shape id='bUn' type='rect'/>`,
		},
		{
			name:    "tolerates whitespace around equals",
			in:      `<shape id = "xxx" type="rect"/>`,
			want:    "bUn",
			wantOut: `<shape id = "bUn" type="rect"/>`,
		},
		{
			name:    "tolerates leading whitespace and XML declaration",
			in:      `<?xml version="1.0"?><shape type="rect"/>`,
			want:    "bUn",
			wantOut: `<?xml version="1.0"?><shape type="rect" id="bUn"/>`,
		},
		{
			name:    "does not touch nested element id",
			in:      `<shape type="rect"><inner id="keepme"/></shape>`,
			want:    "bUn",
			wantOut: `<shape type="rect" id="bUn"><inner id="keepme"/></shape>`,
		},
		{
			name:    "no duplicate space before injected attr",
			in:      `<shape  type="rect" />`,
			want:    "bUn",
			wantOut: `<shape  type="rect" id="bUn" />`,
		},
		{
			name:    "bare tag gets id injected",
			in:      `<shape/>`,
			want:    "bUn",
			wantOut: `<shape id="bUn"/>`,
		},
		{
			name:    "empty string errors",
			in:      ``,
			want:    "bUn",
			wantErr: "no root element",
		},
		{
			name:    "whitespace-only errors",
			in:      "  \n\t  ",
			want:    "bUn",
			wantErr: "no root element",
		},
		{
			name:    "malformed no closing angle errors",
			in:      `<shape type="rect"`,
			want:    "bUn",
			wantErr: "no root element",
		},
		{
			// Regression: \bid matches the "id" suffix in data-id / xml:id.
			// The regex now uses (?:^|\s) so only a standalone id attribute fires.
			name:    "does not confuse data-id with id — injects fresh id",
			in:      `<shape data-id="old" type="rect"/>`,
			want:    "bUn",
			wantOut: `<shape data-id="old" type="rect" id="bUn"/>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ensureXMLRootID(tt.in, tt.want)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want error %q, got nil; out=%q", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.wantOut {
				t.Fatalf("got  %q\nwant %q", got, tt.wantOut)
			}
		})
	}
}
