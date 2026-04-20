// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "testing"

func TestTrimMarkdownCodeBlockTrailingBlanks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trims one trailing blank line before closing fence",
			in:   "before\n```bash\necho hello\n\n```\nafter\n",
			want: "before\n```bash\necho hello\n```\nafter\n",
		},
		{
			name: "trims accumulated trailing blank lines",
			in:   "```go\nfmt.Println(1)\n\n\n```\n",
			want: "```go\nfmt.Println(1)\n```\n",
		},
		{
			name: "keeps intentional blank lines inside code content",
			in:   "```\nline one\n\nline two\n\n```\n",
			want: "```\nline one\n\nline two\n```\n",
		},
		{
			name: "leaves non-code blank lines alone",
			in:   "one\n\n```text\nvalue\n```\n\ntwo\n",
			want: "one\n\n```text\nvalue\n```\n\ntwo\n",
		},
		{
			name: "supports tilde fences",
			in:   "~~~json\n{}\n\n~~~\n",
			want: "~~~json\n{}\n~~~\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := TrimMarkdownCodeBlockTrailingBlanks(tt.in); got != tt.want {
				t.Fatalf("TrimMarkdownCodeBlockTrailingBlanks() = %q, want %q", got, tt.want)
			}
		})
	}
}
