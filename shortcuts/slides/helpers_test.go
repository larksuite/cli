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
