// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestConvertToIMMarkdownTitle(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "plain title",
			input: `<title>Roadmap</title>`,
			want:  "# Roadmap",
		},
		{
			name:  "trim title whitespace",
			input: "<title>\n  Roadmap  \n</title>",
			want:  "# Roadmap",
		},
		{
			name:  "preserve title inner markup",
			input: `<title><b>Bold</b> Title</title>`,
			want:  "# <b>Bold</b> Title",
		},
		{
			name:  "empty title",
			input: `<title>   </title>`,
			want:  "",
		},
	})
}

func TestConvertToIMMarkdownCallout(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "emoji and body",
			input: `<callout emoji="💡">Read **this**.</callout>`,
			want:  "---\n💡 Read **this**.\n---",
		},
		{
			name:  "body without emoji",
			input: `<callout>Plain body</callout>`,
			want:  "---\nPlain body\n---",
		},
		{
			name:  "emoji only",
			input: `<callout emoji="✅"></callout>`,
			want:  "---\n✅\n---",
		},
		{
			name:  "nested callout",
			input: `<callout emoji="✅">Outer <callout emoji="💡">Inner</callout></callout>`,
			want:  "---\n✅ Outer ---\n💡 Inner\n---\n---",
		},
		{
			name:  "callout contains registered tags",
			input: `<callout emoji="📝"><bookmark name="Spec" href="https://example.com"></bookmark></callout>`,
			want:  "---\n📝 [Spec](https://example.com)\n---",
		},
	})
}

func TestConvertToIMMarkdownGridAndColumn(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "two columns",
			input: `<grid><column width-ratio="0.5">Left</column><column width-ratio="0.5">Right</column></grid>`,
			want:  "Left\nRight",
		},
		{
			name:  "column converts nested registered tags",
			input: `<column><bookmark name="Spec" href="https://example.com"></bookmark></column>`,
			want:  "[Spec](https://example.com)\n",
		},
		{
			name:  "empty column",
			input: `<column>   </column>`,
			want:  "",
		},
		{
			name:  "nested grid",
			input: `<grid><column>A</column><column><grid><column>B</column><column>C</column></grid></column></grid>`,
			want:  "A\nB\nC",
		},
		{
			name:  "grid inside callout",
			input: `<callout emoji="📌"><grid><column>A</column><column>B</column></grid></callout>`,
			want:  "---\n📌 A\nB\n---",
		},
	})
}

func TestConvertToIMMarkdownTable(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "basic table",
			input: `<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`,
			want:  "| A | B |\n| - | - |\n| 1 | 2 |",
		},
		{
			name:  "table strips attrs and preserves cell line break",
			input: `<table><tr><th vertical-align="top">A</th><th>B</th></tr><tr><td rowspan="2">1</td><td><b>two</b><br/>lines</td></tr></table>`,
			want:  "| A | B |\n| - | - |\n| 1 | two<br>lines |",
		},
		{
			name:  "table escapes pipe",
			input: `<table><tr><th>A|B</th></tr><tr><td>x|y</td></tr></table>`,
			want:  "| A\\|B |\n| - |\n| x\\|y |",
		},
		{
			name:  "table pads ragged rows",
			input: `<table><tr><th>A</th><th>B</th></tr><tr><td>1</td></tr></table>`,
			want:  "| A | B |\n| - | - |\n| 1 |  |",
		},
		{
			name:  "table converts nested cite",
			input: `<table><tr><th>User</th></tr><tr><td><cite type="user" user-id="ou_1" user-name="Alice"></cite></td></tr></table>`,
			want:  "| User |\n| - |\n| <at user_id=\"ou_1\">Alice</at> |",
		},
		{
			name:  "table without rows falls back to inline code",
			input: `<table><tbody></tbody></table>`,
			want:  "`<table><tbody></tbody></table>`",
		},
	})
}

func TestConvertToIMMarkdownDiscardTags(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "figure discarded",
			input: `before<figure view-type="Card">hidden</figure>after`,
			want:  "beforeafter",
		},
		{
			name:  "figure with source discarded",
			input: `<figure view-type="Preview"><source href="https://example.com/a.md"/></figure>`,
			want:  "",
		},
		{
			name:  "self-closing source discarded",
			input: `a<source href="https://example.com/a.md"/>b`,
			want:  "ab",
		},
		{
			name:  "button discarded",
			input: `a<button>Click</button>b`,
			want:  "ab",
		},
		{
			name:  "time discarded",
			input: `a<time expire-time="123"></time>b`,
			want:  "ab",
		},
	})
}

func TestConvertToIMMarkdownWhiteboard(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "paired whiteboard",
			input: `<whiteboard token="wb_token"></whiteboard>`,
			want:  "`<whiteboard token=\"wb_token\"></whiteboard>`",
		},
		{
			name:  "self-closing whiteboard",
			input: `<whiteboard token="wb_token"/>`,
			want:  "`<whiteboard token=\"wb_token\"/>`",
		},
		{
			name:  "whiteboard with backticks",
			input: "<whiteboard token=\"`wb`\"></whiteboard>",
			want:  "``<whiteboard token=\"`wb`\"></whiteboard>``",
		},
	})
}

func TestConvertToIMMarkdownSheet(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCasesWithContext(t, imMarkdownContext{baseURL: "https://bytedance.larkoffice.com"}, []imMarkdownCase{
		{
			name:  "sheet with sheet id",
			input: `<sheet token="sht_token" sheet-id="S1"></sheet>`,
			want:  "[sheet S1](https://bytedance.larkoffice.com/sheets/sht_token)",
		},
		{
			name:  "sheet without sheet id",
			input: `<sheet token="sht_token"></sheet>`,
			want:  "[sheet](https://bytedance.larkoffice.com/sheets/sht_token)",
		},
		{
			name:  "sheet without token falls back to inline code",
			input: `<sheet sheet-id="S1"></sheet>`,
			want:  "`<sheet sheet-id=\"S1\"></sheet>`",
		},
		{
			name:  "self-closing sheet",
			input: `<sheet token="sht_token" sheet-id="S1"/>`,
			want:  "[sheet S1](https://bytedance.larkoffice.com/sheets/sht_token)",
		},
	})
}

func TestConvertToIMMarkdownBookmark(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "name and href",
			input: `<bookmark name="Example" href="https://example.com"></bookmark>`,
			want:  "[Example](https://example.com)",
		},
		{
			name:  "title fallback",
			input: `<bookmark title="Example" href="https://example.com"></bookmark>`,
			want:  "[Example](https://example.com)",
		},
		{
			name:  "inner text fallback",
			input: `<bookmark href="https://example.com">Example</bookmark>`,
			want:  "[Example](https://example.com)",
		},
		{
			name:  "missing href returns label",
			input: `<bookmark name="Example"></bookmark>`,
			want:  "Example",
		},
		{
			name:  "escaped link label",
			input: `<bookmark name="A [B]" href="https://example.com"></bookmark>`,
			want:  "[A \\[B\\]](https://example.com)",
		},
	})
}

func TestConvertToIMMarkdownCiteUser(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "user id and name",
			input: `<cite type="user" user-id="ou_abc" user-name="Alice"></cite>`,
			want:  `<at user_id="ou_abc">Alice</at>`,
		},
		{
			name:  "open id fallback",
			input: `<cite type="user" open-id="ou_open" name="Bob"></cite>`,
			want:  `<at user_id="ou_open">Bob</at>`,
		},
		{
			name:  "name falls back to user id",
			input: `<cite type="user" user-id="ou_abc"></cite>`,
			want:  `<at user_id="ou_abc">ou_abc</at>`,
		},
		{
			name:  "missing user id returns name",
			input: `<cite type="user" user-name="Alice"></cite>`,
			want:  "Alice",
		},
		{
			name:  "escape at XML",
			input: `<cite type="user" user-id="ou_&quot;" user-name="A&B"></cite>`,
			want:  `<at user_id="ou_&#34;">A&amp;B</at>`,
		},
	})
}

func TestConvertToIMMarkdownCiteDoc(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCasesWithContext(t, imMarkdownContext{baseURL: "https://bytedance.larkoffice.com"}, []imMarkdownCase{
		{
			name:  "doc id to link",
			input: `<cite type="doc" doc-id="doc_token" file-type="docx" title="Spec"></cite>`,
			want:  "[Spec](https://bytedance.larkoffice.com/docx/doc_token)",
		},
		{
			name:  "href wins",
			input: `<cite type="doc" href="https://example.com/doc" title="Spec"></cite>`,
			want:  "[Spec](https://example.com/doc)",
		},
		{
			name:  "default title and file type",
			input: `<cite type="doc" token="doc_token"></cite>`,
			want:  "[document](https://bytedance.larkoffice.com/docx/doc_token)",
		},
		{
			name:  "missing doc id falls back to inline code",
			input: `<cite type="doc" title="Spec"></cite>`,
			want:  "`<cite type=\"doc\" title=\"Spec\"></cite>`",
		},
	})
}

func TestConvertToIMMarkdownCiteCitation(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "inner anchor",
			input: `<cite type="citation"><a href="https://example.com/ref">Ref</a></cite>`,
			want:  "[Ref](https://example.com/ref)",
		},
		{
			name:  "href attr",
			input: `<cite type="citation" href="https://example.com/ref" title="Ref"></cite>`,
			want:  "[Ref](https://example.com/ref)",
		},
		{
			name:  "plain inner fallback",
			input: `<cite type="citation">Plain Ref</cite>`,
			want:  "Plain Ref",
		},
	})
}

func TestConvertToIMMarkdownCiteUnknown(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "unknown paired cite",
			input: `<cite type="unknown">x</cite>`,
			want:  "`<cite type=\"unknown\">x</cite>`",
		},
		{
			name:  "unknown self-closing cite",
			input: `<cite type="unknown"/>`,
			want:  "`<cite type=\"unknown\"/>`",
		},
	})
}

func TestConvertToIMMarkdownDeepRegisteredContainers(t *testing.T) {
	t.Parallel()

	deepGrid := "leaf"
	for i := 0; i < 32; i++ {
		deepGrid = "<grid><column>" + deepGrid + "</column></grid>"
	}
	if got := convertToIMMarkdown(deepGrid, imMarkdownContext{}); got != "leaf" {
		t.Fatalf("deep grid conversion = %q, want %q", got, "leaf")
	}

	deepCallout := "leaf"
	for i := 0; i < 16; i++ {
		deepCallout = `<callout emoji="💡">` + deepCallout + `</callout>`
	}
	got := convertToIMMarkdown(deepCallout, imMarkdownContext{})
	if !strings.Contains(got, "leaf") {
		t.Fatalf("deep callout conversion missing leaf:\n%s", got)
	}
	if count := strings.Count(got, "💡"); count != 16 {
		t.Fatalf("deep callout emoji count = %d, want 16\n%s", count, got)
	}
}

func TestConvertToIMMarkdownMixedDocumentSmoke(t *testing.T) {
	t.Parallel()

	imCtx := imMarkdownContext{baseURL: "https://bytedance.larkoffice.com"}
	input := strings.Join([]string{
		`<title>Roadmap</title>`,
		`<grid><column width-ratio="0.5">### Left</column><column width-ratio="0.5">Right</column></grid>`,
		`<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td><b>two</b><br/>lines</td></tr></tbody></table>`,
		`<cite type="user" user-id="ou_abc" user-name="Alice"></cite>`,
		`<cite type="doc" doc-id="doc_token" file-type="docx" title="Spec"></cite>`,
		`<cite type="citation"><a href="https://example.com/ref">Ref</a></cite>`,
		`<sheet token="sht_token" sheet-id="S1"></sheet>`,
		`<figure view-type="Preview"><source href="https://example.com/a.md"/></figure>`,
	}, "\n")

	got := convertToIMMarkdown(input, imCtx)

	for _, want := range []string{
		"# Roadmap",
		"### Left",
		"Right",
		"| A | B |\n| - | - |\n| 1 | two<br>lines |",
		`<at user_id="ou_abc">Alice</at>`,
		"[Spec](https://bytedance.larkoffice.com/docx/doc_token)",
		"[Ref](https://example.com/ref)",
		"[sheet S1](https://bytedance.larkoffice.com/sheets/sht_token)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("converted content missing %q:\n%s", want, got)
		}
	}
	for _, dropped := range []string{"<grid", "<column", "<table", "<cite", "<sheet", "<figure", "<source"} {
		if strings.Contains(got, dropped) {
			t.Fatalf("converted content still contains %q:\n%s", dropped, got)
		}
	}
}

func TestNewIMMarkdownContextExtractsBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "full URL",
			input: "https://bytedance.larkoffice.com/docx/doc_token?from=copy",
			want:  "https://bytedance.larkoffice.com",
		},
		{
			name:  "URL without scheme",
			input: "bytedance.larkoffice.com/docx/doc_token",
			want:  "https://bytedance.larkoffice.com",
		},
		{
			name:  "wiki URL without scheme",
			input: "bytedance.larkoffice.com/wiki/wiki_token",
			want:  "https://bytedance.larkoffice.com",
		},
		{
			name:  "legacy doc URL without scheme",
			input: "bytedance.larkoffice.com/doc/doc_token",
			want:  "https://bytedance.larkoffice.com",
		},
		{
			name:  "token",
			input: "doc_token",
			want:  "https://larkoffice.com",
		},
		{
			name:  "blank",
			input: " ",
			want:  "https://larkoffice.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := newIMMarkdownContext(tt.input).baseURL; got != tt.want {
				t.Fatalf("baseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

type imMarkdownCase struct {
	name  string
	input string
	want  string
}

func assertIMMarkdownCases(t *testing.T, cases []imMarkdownCase) {
	t.Helper()
	assertIMMarkdownCasesWithContext(t, imMarkdownContext{baseURL: "https://larkoffice.com"}, cases)
}

func assertIMMarkdownCasesWithContext(t *testing.T, imCtx imMarkdownContext, cases []imMarkdownCase) {
	t.Helper()

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := convertToIMMarkdown(tt.input, imCtx); got != tt.want {
				t.Fatalf("convertToIMMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}
