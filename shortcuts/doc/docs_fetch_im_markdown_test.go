// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplyFetchIMMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     map[string]interface{}
		docInput string
		want     map[string]interface{}
	}{
		{
			name: "missing document leaves data unchanged",
			data: map[string]interface{}{
				"content": `<title>Roadmap</title>`,
			},
			docInput: "https://tenant.example.com/docx/doc_token",
			want: map[string]interface{}{
				"content": `<title>Roadmap</title>`,
			},
		},
		{
			name: "non string content leaves data unchanged",
			data: map[string]interface{}{
				"document": map[string]interface{}{
					"content": 123,
				},
			},
			docInput: "https://tenant.example.com/docx/doc_token",
			want: map[string]interface{}{
				"document": map[string]interface{}{
					"content": 123,
				},
			},
		},
		{
			name: "converts content with tenant base url",
			data: map[string]interface{}{
				"document": map[string]interface{}{
					"content": `<title>Roadmap</title>` + "\n" + `<sheet token="sht_token" sheet-id="S1"></sheet>`,
				},
			},
			docInput: "https://tenant.example.com/docx/doc_token",
			want: map[string]interface{}{
				"document": map[string]interface{}{
					"content": "# Roadmap\n[sheet S1](https://tenant.example.com/sheets/sht_token)",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			applyFetchIMMarkdown(tt.data, tt.docInput)
			if !reflect.DeepEqual(tt.data, tt.want) {
				t.Fatalf("data = %#v, want %#v", tt.data, tt.want)
			}
		})
	}
}

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
			name:  "convert title inner markup",
			input: `<title><b>Bold</b> Title</title>`,
			want:  "# **Bold** Title",
		},
		{
			name:  "empty title",
			input: `<title>   </title>`,
			want:  "",
		},
		{
			name:  "title followed by text",
			input: `<title>Roadmap</title>tail`,
			want:  "# Roadmaptail",
		},
		{
			name:  "uppercase title is handled case-insensitively",
			input: `<TITLE>Roadmap</TITLE>`,
			want:  "# Roadmap",
		},
		{
			name:  "missing closing title is preserved",
			input: `before<title>Roadmap`,
			want:  `before<title>Roadmap`,
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
			name:  "empty callout",
			input: `<callout></callout>`,
			want:  "---\n---",
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
		{
			name:  "callout contains grid and cite",
			input: `<callout emoji="📣"><grid><column><cite type="user" user-id="ou_1" user-name="Alice"></cite></column><column><bookmark name="Spec" href="https://example.com"></bookmark></column></grid></callout>`,
			want:  "---\n📣 <at user_id=\"ou_1\">Alice</at>\n[Spec](https://example.com)\n---",
		},
		{
			name:  "same-name nested callout with trailing text",
			input: `<callout emoji="1">a<callout emoji="2">b</callout>c</callout>d`,
			want:  "---\n1 a---\n2 b\n---c\n---d",
		},
		{
			name:  "missing closing callout is preserved",
			input: `before<callout emoji="💡">body`,
			want:  `before<callout emoji="💡">body`,
		},
	})
}

func TestConvertToIMMarkdownBlockquote(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "single paragraph",
			input: `<blockquote><p>quote <a href="https://example.com">link</a></p></blockquote>`,
			want:  "> quote [link](https://example.com)",
		},
		{
			name:  "multiple paragraphs keep line breaks",
			input: `<blockquote><p>first</p><p><b>second</b></p></blockquote>`,
			want:  "> first\n> **second**",
		},
		{
			name:  "nested blockquote keeps nested markers",
			input: `<blockquote><p>outer</p><blockquote><p>inner</p></blockquote></blockquote>`,
			want:  "> outer\n> > inner",
		},
		{
			name:  "blank line keeps quote marker",
			input: "<blockquote>first\n\nsecond</blockquote>",
			want:  "> first\n>\n> second",
		},
		{
			name:  "empty blockquote",
			input: `<blockquote> </blockquote>`,
			want:  "",
		},
		{
			name:  "plain adjacent paragraphs outside blockquote stay compact",
			input: `<p>first</p><p>second</p>`,
			want:  "firstsecond",
		},
	})
}

func TestConvertToIMMarkdownParagraphHeadingAndListItemEdges(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "empty heading",
			input: `<h2> </h2>`,
			want:  "",
		},
		{
			name:  "empty paragraph",
			input: `<p> </p>`,
			want:  "",
		},
		{
			name:  "top level list item uses seq",
			input: "<li seq=\"7\">first\nsecond</li>",
			want:  "7. first\n  second\n",
		},
		{
			name:  "top level empty list item",
			input: `<li></li>`,
			want:  "",
		},
		{
			name:  "unordered list skips non item text and empty items",
			input: `<ul>prefix<li>first</li><li> </li><li>second</li></ul>`,
			want:  "- first\n- second",
		},
		{
			name:  "unclosed list item stops list scan",
			input: `<ul><li>first</li><li>second</ul>`,
			want:  "- first",
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
		{
			name:  "adjacent grids do not merge",
			input: `<grid><column>A</column></grid><grid><column>B</column></grid>`,
			want:  "AB",
		},
		{
			name:  "column with nested callout keeps recursive output",
			input: `<column><callout emoji="💡">Tip</callout></column>`,
			want:  "---\n💡 Tip\n---\n",
		},
		{
			name:  "missing closing grid is preserved",
			input: `<grid><column>A</column>`,
			want:  `<grid><column>A</column>`,
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
			want:  "| A | B |\n| - | - |\n| 1 | **two**<br>lines |",
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
			name:  "table converts nested bookmark and sheet",
			input: `<table><tr><th>Link</th><th>Sheet</th></tr><tr><td><bookmark name="Spec" href="https://example.com"></bookmark></td><td><sheet token="sht_1" sheet-id="S1"></sheet></td></tr></table>`,
			want:  "| Link | Sheet |\n| - | - |\n| [Spec](https://example.com) | [sheet S1](https://larkoffice.com/sheets/sht_1) |",
		},
		{
			name:  "table strips nested unknown html but preserves text",
			input: `<table><tr><th>A</th></tr><tr><td><span color="red">red</span> <u>under</u></td></tr></table>`,
			want:  "| A |\n| - |\n| red under |",
		},
		{
			name:  "table normalizes markdown hard breaks",
			input: "<table><tr><th>A</th></tr><tr><td>line1  \nline2</td></tr></table>",
			want:  "| A |\n| - |\n| line1<br>line2 |",
		},
		{
			name:  "table cell keeps nested table whole",
			input: `<table><tr><th>Outer</th></tr><tr><td>before <table><tr><th>Inner</th></tr><tr><td>x</td></tr></table> after</td></tr></table>`,
			want:  "| Outer |\n| - |\n| before \\| Inner \\|<br>\\| - \\|<br>\\| x \\| after |",
		},
		{
			name:  "table with only data row treats first row as header",
			input: `<table><tr><td>A</td><td>B</td></tr></table>`,
			want:  "| A | B |\n| - | - |",
		},
		{
			name:  "table without rows falls back to inline code",
			input: `<table><tbody></tbody></table>`,
			want:  "`<table><tbody></tbody></table>`",
		},
		{
			name:  "table row without cells falls back to inline code",
			input: `<table><tr></tr></table>`,
			want:  "`<table><tr></tr></table>`",
		},
		{
			name:  "table self closing row falls back to inline code",
			input: `<table><tr/></table>`,
			want:  "`<table><tr/></table>`",
		},
		{
			name:  "table empty cell stays empty",
			input: `<table><tr><td> </td></tr></table>`,
			want:  "|  |\n| - |",
		},
		{
			name:  "missing closing table is preserved",
			input: `before<table><tr><td>A</td></tr>`,
			want:  `before<table><tr><td>A</td></tr>`,
		},
	})
}

func TestIMMarkdownElementExtractionEdges(t *testing.T) {
	t.Parallel()

	bodies := extractIMMarkdownElementBodies(`</tr><tr/> <tr><td>x</td></tr><tr>open`, imMarkdownRowTagRE)
	if want := []string{"", "<td>x</td>"}; !reflect.DeepEqual(bodies, want) {
		t.Fatalf("extractIMMarkdownElementBodies() = %#v, want %#v", bodies, want)
	}

	if _, _, ok := findIMMarkdownElementClosingTag(`<tr><td>x`, len("<tr>"), imMarkdownRowTagRE); ok {
		t.Fatal("findIMMarkdownElementClosingTag() found closing tag, want false")
	}

	start, end, ok := findIMMarkdownListItemClosingTag(`<li>outer<li/>tail</li>`, len("<li>"))
	if !ok {
		t.Fatal("findIMMarkdownListItemClosingTag() did not find closing tag")
	}
	if got, want := `<li>outer<li/>tail</li>`[start:end], "</li>"; got != want {
		t.Fatalf("closing tag = %q, want %q", got, want)
	}

	if _, _, ok := findIMMarkdownListItemClosingTag(`<li>open`, len("<li>")); ok {
		t.Fatal("findIMMarkdownListItemClosingTag() found closing tag, want false")
	}

	start, end, ok = findIMMarkdownListItemClosingTag(`<li>outer<li>inner</li>tail</li>`, len("<li>"))
	if !ok {
		t.Fatal("findIMMarkdownListItemClosingTag() did not find nested closing tag")
	}
	if got, want := `<li>outer<li>inner</li>tail</li>`[start:end], "</li>"; got != want {
		t.Fatalf("nested closing tag = %q, want %q", got, want)
	}

	if got := convertIMMarkdownListItems("plain text", false, imMarkdownContext{}); got != "" {
		t.Fatalf("convertIMMarkdownListItems() = %q, want empty", got)
	}
}

func TestNormalizeIMMarkdownTableCellStripsUnknownTags(t *testing.T) {
	t.Parallel()

	got := normalizeIMMarkdownTableCell(`<span style="x">red</span>`)
	if want := "red"; got != want {
		t.Fatalf("normalizeIMMarkdownTableCell() = %q, want %q", got, want)
	}
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
			name:  "source name becomes inline code",
			input: "a<source name=\"report`v1`.pdf\" href=\"https://example.com/a.md\"/>b",
			want:  "a``report`v1`.pdf``b",
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
		{
			name:  "colgroup discarded",
			input: `a<colgroup><col width="120"/></colgroup>b`,
			want:  "ab",
		},
		{
			name:  "col discarded",
			input: `a<col width="120"/>b`,
			want:  "ab",
		},
		{
			name:  "self-closing button discarded",
			input: `a<button/>b`,
			want:  "ab",
		},
		{
			name:  "missing closing discard tag is preserved",
			input: `a<figure>hidden`,
			want:  `a<figure>hidden`,
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
		{
			name:  "whiteboard preserves inner text as opaque",
			input: `<whiteboard token="wb">not exported</whiteboard>`,
			want:  "`<whiteboard token=\"wb\">not exported</whiteboard>`",
		},
		{
			name:  "missing closing whiteboard is preserved",
			input: `<whiteboard token="wb">`,
			want:  `<whiteboard token="wb">`,
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
		{
			name:  "sheet token is trimmed",
			input: `<sheet token="  sht_token  " sheet-id="S1"></sheet>`,
			want:  "[sheet S1](https://bytedance.larkoffice.com/sheets/sht_token)",
		},
		{
			name:  "sheet inside text",
			input: `before <sheet token="sht_token"></sheet> after`,
			want:  "before [sheet](https://bytedance.larkoffice.com/sheets/sht_token) after",
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
		{
			name:  "href is percent encoded",
			input: `<bookmark name="Spec" href="https://example.com/wiki/A B (draft)?q=x y#frag(1)"></bookmark>`,
			want:  "[Spec](https://example.com/wiki/A%20B%20%28draft%29?q=x%20y#frag%281%29)",
		},
		{
			name:  "href keeps existing percent escapes",
			input: `<bookmark name="Spec" href="https://example.com/wiki/A%20B"></bookmark>`,
			want:  "[Spec](https://example.com/wiki/A%20B)",
		},
		{
			name:  "href escapes invalid percent and unicode",
			input: `<bookmark name="Spec" href="https://example.com/wiki/研发%zz?x=1%"></bookmark>`,
			want:  "[Spec](https://example.com/wiki/%E7%A0%94%E5%8F%91%25zz?x=1%25)",
		},
		{
			name:  "href escapes markdown delimiter bytes",
			input: "<bookmark name=\"Spec\" href=\"https://example.com/a&lt;b&gt;|c`d\"></bookmark>",
			want:  "[Spec](https://example.com/a%3Cb%3E%7Cc%60d)",
		},
		{
			name:  "inner registered tag fallback",
			input: `<bookmark href="https://example.com"><cite type="user" user-id="ou_1" user-name="Alice"></cite></bookmark>`,
			want:  "[Alice](https://example.com)",
		},
		{
			name:  "href fallback as label",
			input: `<bookmark href="https://example.com"></bookmark>`,
			want:  "[https://example.com](https://example.com)",
		},
		{
			name:  "self-closing bookmark without href",
			input: `<bookmark name="Example"/>`,
			want:  "Example",
		},
	})
}

func TestConvertToIMMarkdownInlineEdges(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "empty strong emphasis and delete",
			input: `<b> </b><em> </em><del> </del>`,
			want:  "",
		},
		{
			name:  "anchor without href returns text",
			input: `<a>plain <b>text</b></a>`,
			want:  "plain **text**",
		},
		{
			name:  "anchor without text falls back to href",
			input: `<a href="https://example.com/a b"></a>`,
			want:  "[https://example.com/a b](https://example.com/a%20b)",
		},
		{
			name:  "latex escapes dollars",
			input: `<latex>price=$5</latex>`,
			want:  "$price=\\$5$",
		},
		{
			name:  "empty latex",
			input: `<latex> </latex>`,
			want:  "",
		},
		{
			name:  "image missing href",
			input: `<img alt="A"/>`,
			want:  "",
		},
		{
			name:  "image uses src and title fallback",
			input: `<img src="https://example.com/i 1.png" title="A [img]"/>`,
			want:  "![A \\[img\\]](https://example.com/i%201.png)",
		},
		{
			name:  "plain fenced code",
			input: `<pre><code>plain</code></pre>`,
			want:  "```\nplain\n```",
		},
		{
			name:  "code inline trims nested markup",
			input: `<code><b>x</b></code>`,
			want:  "`x`",
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
		{
			name:  "inner text fallback when attrs missing name",
			input: `<cite type="user" user-id="ou_abc">Alice</cite>`,
			want:  `<at user_id="ou_abc">Alice</at>`,
		},
		{
			name:  "self-closing user cite",
			input: `<cite type="user" user-id="ou_abc" user-name="Alice"/>`,
			want:  `<at user_id="ou_abc">Alice</at>`,
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
			input: `<cite type="doc" href="https://example.com/doc (draft)" title="Spec"></cite>`,
			want:  "[Spec](https://example.com/doc%20%28draft%29)",
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
		{
			name:  "wiki file type link",
			input: `<cite type="doc" doc-id="wiki_token" file-type="wiki" title="Wiki"></cite>`,
			want:  "[Wiki](https://bytedance.larkoffice.com/wiki/wiki_token)",
		},
		{
			name:  "doc title is escaped",
			input: `<cite type="doc" doc-id="doc_token" title="A [B]"></cite>`,
			want:  "[A \\[B\\]](https://bytedance.larkoffice.com/docx/doc_token)",
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
		{
			name:  "inner anchor text strips markup",
			input: `<cite type="citation"><a href="https://example.com/ref"><b>Ref</b></a></cite>`,
			want:  "[Ref](https://example.com/ref)",
		},
		{
			name:  "single quoted inner anchor falls back to href text",
			input: `<cite type="citation"><a href='https://example.com/ref'></a></cite>`,
			want:  "[https://example.com/ref](https://example.com/ref)",
		},
		{
			name:  "href attr falls back to href label",
			input: `<cite type="citation" href="https://example.com/ref"></cite>`,
			want:  "[https://example.com/ref](https://example.com/ref)",
		},
	})
}

func TestEscapeMarkdownLinkDestinationInvalidUTF8(t *testing.T) {
	t.Parallel()

	got := escapeMarkdownLinkDestination(string([]byte{'a', 0xff, 'b'}))
	if want := "a%FFb"; got != want {
		t.Fatalf("escapeMarkdownLinkDestination() = %q, want %q", got, want)
	}
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

func TestConvertToIMMarkdownScannerBoundaries(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "unknown tag preserved with known child untouched",
			input: `<unknown><bookmark name="Spec" href="https://example.com"></bookmark></unknown>`,
			want:  `<unknown>[Spec](https://example.com)</unknown>`,
		},
		{
			name:  "registered tag attributes single quotes",
			input: `<bookmark name='Spec' href='https://example.com'></bookmark>`,
			want:  "[Spec](https://example.com)",
		},
		{
			name:  "registered tag name with leading text",
			input: `alpha<title>Beta</title>gamma`,
			want:  "alpha# Betagamma",
		},
		{
			name:  "xml comment is preserved",
			input: `a<!-- comment --><title>T</title>`,
			want:  "a<!-- comment --># T",
		},
		{
			name:  "br is preserved",
			input: `a<br/>b`,
			want:  "a<br/>b",
		},
		{
			name:  "malformed attribute still allows handler",
			input: `<bookmark name=Spec href="https://example.com">Inner</bookmark>`,
			want:  "[Inner](https://example.com)",
		},
	})
}

func TestConvertToIMMarkdownCompositeNesting(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCasesWithContext(t, imMarkdownContext{baseURL: "https://tenant.example.com"}, []imMarkdownCase{
		{
			name:  "callout grid table and resources",
			input: `<callout emoji="📌"><grid><column><table><tr><th>Owner</th><th>Doc</th></tr><tr><td><cite type="user" user-id="ou_1" user-name="Alice"></cite></td><td><cite type="doc" doc-id="doc_1" title="Spec"></cite></td></tr></table></column><column><sheet token="sht_1" sheet-id="S1"></sheet></column></grid></callout>`,
			want:  "---\n📌 | Owner | Doc |\n| - | - |\n| <at user_id=\"ou_1\">Alice</at> | [Spec](https://tenant.example.com/docx/doc_1) |\n[sheet S1](https://tenant.example.com/sheets/sht_1)\n---",
		},
		{
			name:  "grid inside table cell",
			input: `<table><tr><th>Outer</th></tr><tr><td><grid><column>A</column><column>B</column></grid></td></tr></table>`,
			want:  "| Outer |\n| - |\n| A<br>B |",
		},
		{
			name:  "table inside table cell",
			input: `<table><tr><th>Outer</th><th>Tail</th></tr><tr><td><table><tr><th>Inner</th></tr><tr><td>x</td></tr></table></td><td>done</td></tr></table>`,
			want:  "| Outer | Tail |\n| - | - |\n| \\| Inner \\|<br>\\| - \\|<br>\\| x \\| | done |",
		},
		{
			name:  "bookmark wraps callout fallback text",
			input: `<bookmark href="https://example.com"><callout emoji="💡">Tip</callout></bookmark>`,
			want:  "[💡 Tip](https://example.com)",
		},
	})
}

func TestConvertToIMMarkdownUnclosedFragments(t *testing.T) {
	t.Parallel()

	assertIMMarkdownCases(t, []imMarkdownCase{
		{
			name:  "unclosed title preserves nested registered tag",
			input: `before<title><bookmark name="Spec" href="https://example.com"></bookmark>`,
			want:  `before<title><bookmark name="Spec" href="https://example.com"></bookmark>`,
		},
		{
			name:  "unclosed callout preserves nested registered tag",
			input: `before<callout emoji="💡"><bookmark name="Spec" href="https://example.com"></bookmark>`,
			want:  `before<callout emoji="💡"><bookmark name="Spec" href="https://example.com"></bookmark>`,
		},
		{
			name:  "unclosed grid preserves closed child",
			input: `before<grid><column>A</column>`,
			want:  `before<grid><column>A</column>`,
		},
		{
			name:  "unclosed column preserves nested registered tag",
			input: `before<column><bookmark name="Spec" href="https://example.com"></bookmark>`,
			want:  `before<column><bookmark name="Spec" href="https://example.com"></bookmark>`,
		},
		{
			name:  "unclosed table preserves nested cite",
			input: `before<table><tr><td><cite type="user" user-id="ou_1" user-name="Alice"></cite></td></tr>`,
			want:  `before<table><tr><td><cite type="user" user-id="ou_1" user-name="Alice"></cite></td></tr>`,
		},
		{
			name:  "unclosed figure preserves nested source",
			input: `before<figure><source href="https://example.com/a.md"/>`,
			want:  `before<figure><source href="https://example.com/a.md"/>`,
		},
		{
			name:  "unclosed whiteboard preserves nested registered tag",
			input: `before<whiteboard token="wb"><bookmark name="Spec" href="https://example.com"></bookmark>`,
			want:  `before<whiteboard token="wb"><bookmark name="Spec" href="https://example.com"></bookmark>`,
		},
		{
			name:  "unclosed sheet preserves nested registered tag",
			input: `before<sheet token="sht"><bookmark name="Spec" href="https://example.com"></bookmark>`,
			want:  `before<sheet token="sht"><bookmark name="Spec" href="https://example.com"></bookmark>`,
		},
		{
			name:  "unclosed bookmark preserves nested cite",
			input: `before<bookmark href="https://example.com"><cite type="user" user-id="ou_1" user-name="Alice"></cite>`,
			want:  `before<bookmark href="https://example.com"><cite type="user" user-id="ou_1" user-name="Alice"></cite>`,
		},
		{
			name:  "unclosed cite preserves inner anchor",
			input: `before<cite type="citation"><a href="https://example.com/ref">Ref</a>`,
			want:  `before<cite type="citation"><a href="https://example.com/ref">Ref</a>`,
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

func TestConvertToIMMarkdownDocumentExpectedTagsAndEscaping(t *testing.T) {
	t.Parallel()

	imCtx := imMarkdownContext{baseURL: "https://bytedance.larkoffice.com"}
	input := strings.Join([]string{
		`<h1>Roadmap <span text-color="red">Q1</span></h1>`,
		`<h7>Deep Heading</h7>`,
		`<p>plain<br/>next <b>Bold</b> <em>Italic</em> <del>Gone</del> <u>Under</u> <span background-color="yellow">Plain</span> <a href="https://example.com/a(b)">A [B]</a></p>`,
		`<blockquote><p>quote <a type="url-preview" href="https://example.com/card">Card</a></p></blockquote>`,
		`<ul><li>first</li><li><b>second</b></li></ul>`,
		`<ol><li seq="auto">one</li><li seq="3">three</li></ol>`,
		`<pre lang="Go"><code>fmt.Println(&quot;hi&quot;)` + "\n```" + `</code></pre>`,
		`<p><code>` + "`edge`" + `</code> <latex>E=mc^2</latex> <hr/> <img href="https://example.com/i(1).png" alt="A [img]"/></p>`,
		`<source name="report` + "`v1`" + `.pdf"/><source href="https://example.com/no-name"/>`,
		`<task task-id="task_1"></task><task></task><chat_card chat-id="chat_1"></chat_card><chat_card></chat_card>`,
		`<bitable></bitable><base_refer></base_refer><okr></okr><poll></poll><agenda></agenda><folder_manager></folder_manager><wiki_catalog></wiki_catalog><wiki_recent_update></wiki_recent_update><chart_refer_host_perm></chart_refer_host_perm><synced_reference></synced_reference><synced-source></synced-source><mindnote></mindnote>`,
	}, "\n")

	want := strings.Join([]string{
		`# Roadmap Q1`,
		`###### Deep Heading`,
		`plain<br/>next **Bold** *Italic* ~~Gone~~ Under Plain [A \[B\]](https://example.com/a%28b%29)`,
		`> quote [Card](https://example.com/card)`,
		`- first`,
		`- **second**`,
		`1. one`,
		`3. three`,
		"````Go\nfmt.Println(\"hi\")\n```\n````",
		"`` `edge` `` $E=mc^2$ --- ![A \\[img\\]](https://example.com/i%281%29.png)",
		"``report`v1`.pdf``",
		"`Task``Chat card`",
		"`Base``Base``OKR`",
	}, "\n")

	if got := convertToIMMarkdown(input, imCtx); got != want {
		t.Fatalf("convertToIMMarkdown() = %q, want %q", got, want)
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
		"| A | B |\n| - | - |\n| 1 | **two**<br>lines |",
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
		{
			name:  "empty trimmed prefix falls back",
			input: "////docx/doc_token",
			want:  "https://larkoffice.com",
		},
		{
			name:  "scheme candidate inside schemeless URL",
			input: "//https://bytedance.larkoffice.com/docx/doc_token",
			want:  "https://bytedance.larkoffice.com",
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

func TestIMMarkdownBaseURLFromInputEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "empty candidate before marker is skipped",
			input: "///docx/doc_token",
		},
		{
			name:  "scheme candidate before marker is returned",
			input: "//https://tenant.example.com/docx/doc_token",
			want:  "https://tenant.example.com",
			ok:    true,
		},
		{
			name:  "host without dot before marker is rejected",
			input: "tenant/docx/doc_token",
		},
		{
			name:  "no document marker is rejected",
			input: "tenant.example.com/path/doc_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := imMarkdownBaseURLFromInput(tt.input)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("baseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIMMarkdownHandlerDirectFallbackBranches(t *testing.T) {
	t.Parallel()

	ctx := imMarkdownContext{baseURL: "https://tenant.example.com"}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "empty heading",
			got:  handleIMMarkdownHeading(2)("", " ", nil, ctx),
			want: "",
		},
		{
			name: "empty paragraph",
			got:  handleIMMarkdownParagraph("", " ", nil, ctx),
			want: "",
		},
		{
			name: "list item seq trims trailing dot",
			got:  handleIMMarkdownListItem("", "first\nsecond", map[string]string{"seq": "7."}, ctx),
			want: "7. first\n  second\n",
		},
		{
			name: "list item seq auto uses bullet",
			got:  handleIMMarkdownListItem("", "first", map[string]string{"seq": "auto"}, ctx),
			want: "- first\n",
		},
		{
			name: "empty list item",
			got:  handleIMMarkdownListItem("", " ", map[string]string{"seq": "3"}, ctx),
			want: "",
		},
		{
			name: "empty callout",
			got:  handleIMMarkdownCallout("", "", nil, ctx),
			want: "---\n---",
		},
		{
			name: "empty blockquote",
			got:  handleIMMarkdownBlockquote("", " ", nil, ctx),
			want: "",
		},
		{
			name: "blockquote preserves blank quote lines",
			got:  handleIMMarkdownBlockquote("", "first\n\nsecond", nil, ctx),
			want: "> first\n>\n> second",
		},
		{
			name: "empty latex",
			got:  handleIMMarkdownLatex("", " ", nil, ctx),
			want: "",
		},
		{
			name: "image without URL",
			got:  handleIMMarkdownImage("", "", map[string]string{"alt": "A"}, ctx),
			want: "",
		},
		{
			name: "empty strong",
			got:  handleIMMarkdownStrong("", " ", nil, ctx),
			want: "",
		},
		{
			name: "empty emphasis",
			got:  handleIMMarkdownEmphasis("", " ", nil, ctx),
			want: "",
		},
		{
			name: "empty delete",
			got:  handleIMMarkdownDelete("", " ", nil, ctx),
			want: "",
		},
		{
			name: "anchor without href",
			got:  handleIMMarkdownAnchor("", "<b>plain</b>", nil, ctx),
			want: "**plain**",
		},
		{
			name: "table skips rows without cells",
			got:  handleIMMarkdownTable("<table><tr></tr></table>", "<tr></tr>", nil, ctx),
			want: "`<table><tr></tr></table>`",
		},
		{
			name: "empty normalized table cell",
			got:  normalizeIMMarkdownTableCell("<span> </span>"),
			want: "",
		},
		{
			name: "plain fenced code uses minimum fence",
			got:  imMarkdownFencedCode("plain", ""),
			want: "```\nplain\n```",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestIMMarkdownExtractionAndListBreakBranches(t *testing.T) {
	t.Parallel()

	rowBodies := extractIMMarkdownElementBodies(`</tr><tr/><tr>open`, imMarkdownRowTagRE)
	if want := []string{""}; !reflect.DeepEqual(rowBodies, want) {
		t.Fatalf("extractIMMarkdownElementBodies() = %#v, want %#v", rowBodies, want)
	}

	if _, _, ok := findIMMarkdownElementClosingTag(`<tr><td>open</td>`, len("<tr>"), imMarkdownRowTagRE); ok {
		t.Fatal("findIMMarkdownElementClosingTag() found closing tag, want false")
	}

	if got := convertIMMarkdownListItems("", false, imMarkdownContext{}); got != "" {
		t.Fatalf("empty list conversion = %q, want empty", got)
	}
	if got := convertIMMarkdownListItems("<li>open", false, imMarkdownContext{}); got != "" {
		t.Fatalf("unclosed list conversion = %q, want empty", got)
	}
	if _, _, ok := findIMMarkdownListItemClosingTag(`<li>outer<li>inner</li>`, len("<li>")); ok {
		t.Fatal("findIMMarkdownListItemClosingTag() found closing tag for unbalanced nested item")
	}
}

func TestIMMarkdownLinkAndEncodingFallbackBranches(t *testing.T) {
	t.Parallel()

	text, href, ok := extractIMMarkdownInnerLink(`<a href='https://example.com/ref'></a>`)
	if !ok {
		t.Fatal("extractIMMarkdownInnerLink() ok = false, want true")
	}
	if text != "https://example.com/ref" || href != "https://example.com/ref" {
		t.Fatalf("inner link = (%q, %q), want href fallback", text, href)
	}

	if got := escapeMarkdownLinkDestination("a%zz%"); got != "a%25zz%25" {
		t.Fatalf("escaped invalid percent = %q, want %q", got, "a%25zz%25")
	}
	if got := escapeMarkdownLinkDestination("研发"); got != "%E7%A0%94%E5%8F%91" {
		t.Fatalf("escaped unicode = %q, want encoded UTF-8 bytes", got)
	}
	if got := escapeMarkdownLinkDestination(string([]byte{'a', 0xff, 'b'})); got != "a%FFb" {
		t.Fatalf("escaped invalid UTF-8 = %q, want %q", got, "a%FFb")
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
