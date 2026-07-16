// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"regexp"
	"strings"
	"testing"

	gast "github.com/yuin/goldmark/ast"
)

func TestParseXMLBuildsBlockDistribution(t *testing.T) {
	result, err := Parse(`<title>T</title><p>P</p><ul><li>A</li><li>B</li></ul>`, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<title>T</title><p>P</p><ul><li>A</li><li>B</li></ul>` {
		t.Fatalf("XML = %q", result.XML)
	}
	if result.Profile.BlockCount != 5 {
		t.Fatalf("block total = %d, want 5", result.Profile.BlockCount)
	}
	shares := map[string]BlockShare{}
	for _, share := range result.Profile.Blocks {
		shares[share.Type] = share
	}
	if got := shares["li"]; got.Count != 2 || got.Ratio != 0.4 {
		t.Fatalf("li share = %+v, want count=2 ratio=0.4", got)
	}
	for _, typ := range []string{"title", "p", "ul"} {
		if got := shares[typ]; got.Count != 1 || got.Ratio != 0.2 {
			t.Errorf("%s share = %+v, want count=1 ratio=0.2", typ, got)
		}
	}
}

func TestParseXMLRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "missing closing tag", source: `<p>one`},
		{name: "mismatched closing tag", source: `<Unknown>x</unknown>`},
		{name: "closing void tag", source: `<img></img>`},
		{name: "malformed block id", source: `<block_id="8,9"/>`},
		{name: "unterminated cdata", source: `<code><![CDATA[a < b</code>`},
		{name: "tag spacing", source: `< p>text< / p>`},
		{name: "self closing slash spacing", source: `<p/ >`},
		{name: "unquoted attribute", source: `<p align=center>text</p>`},
		{name: "invalid entity", source: `<p>one &unknown;</p>`},
		{name: "invalid attribute entity", source: `<img href="https://example.com/&unknown;"/>`},
		{name: "bare attribute ampersand", source: `<img href="https://example.com/?a=1&b=2"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.source, FormatXML); err == nil {
				t.Fatalf("Parse(%q) succeeded, want validation error", tt.source)
			}
		})
	}
}

func TestParseXMLChecksSyntaxWithoutBusinessSchema(t *testing.T) {
	source := `<extension arbitrary="value"><p>known</p></extension>` +
		`<span>x<table><tr><td>nested</td></tr></table></span>` +
		`<td>orphan</td><img><task/><whiteboard/>`
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original %q", result.XML, source)
	}
	for tag, want := range map[string]int{
		"p": 1, "table": 1, "tr": 1, "img": 1, "task": 1, "whiteboard": 1,
	} {
		if got := blockCountForTest(result.Profile.Blocks, tag); got != want {
			t.Errorf("%s blocks = %d, want %d; profile=%+v", tag, got, want, result.Profile)
		}
	}
	if result.Profile.BlockCount != 6 {
		t.Fatalf("profile = %+v, want six known blocks", result.Profile)
	}
	for _, tag := range []string{"extension", "td"} {
		if got := blockCountForTest(result.Profile.Blocks, tag); got != 0 {
			t.Errorf("%s blocks = %d, want 0", tag, got)
		}
	}
}

func TestParseAutoDetectsXMLAndMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		source string
		blocks int
	}{
		{name: "xml", source: `<title>T</title><p>P</p>`, blocks: 2},
		{name: "markdown", source: "# T\n\nP", blocks: 2},
		{name: "URI autolink", source: `<https://example.com>`, blocks: 1},
		{name: "email autolink", source: `<user@example.com>`, blocks: 1},
		{name: "comment before Markdown", source: "<!-- note -->\n# Heading", blocks: 1},
		{name: "XML container before Markdown", source: "<callout>\n<p>note</p>\n</callout>\n# Heading", blocks: 3},
		{name: "comment before plain Markdown", source: "<!-- note -->\nPlain **body**", blocks: 1},
		{name: "comment before Setext heading", source: "<!-- note -->\nHeading\n=======", blocks: 1},
		{name: "comment before Markdown table", source: "<!-- note -->\n| A |\n| - |\n| B |", blocks: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := ParseAuto(tt.source)
			if err != nil {
				t.Fatalf("ParseAuto() error = %v", err)
			}
			if profile.BlockCount != tt.blocks {
				t.Fatalf("profile = %+v, want %d blocks", profile, tt.blocks)
			}
		})
	}
}

func TestDetectFormatKeepsMarkdownAfterLeadingXMLConstructs(t *testing.T) {
	for _, source := range []string{
		"<!-- note -->\n# Heading",
		"<callout>\n<p>note</p>\n</callout>\n# Heading",
		"<callout>\n## Nested heading\n</callout>",
		"<!-- note -->\nPlain **body**",
	} {
		if got := DetectFormat(source); got != FormatMarkdown {
			t.Errorf("DetectFormat(%q) = %q, want %q", source, got, FormatMarkdown)
		}
	}
	if got := DetectFormat(`<callout><p># literal XML text</p></callout>`); got != FormatXML {
		t.Fatalf("DetectFormat() = %q, want XML for markup-contained text", got)
	}
	const inlineXML = `<div>plain text</div>`
	if got := DetectFormat(inlineXML); got != FormatXML {
		t.Fatalf("DetectFormat(%q) = %q, want XML", inlineXML, got)
	}
	if profile, err := ParseAuto(inlineXML); err != nil || profile.BlockCount != 1 {
		t.Fatalf("ParseAuto(%q) profile = %+v, error = %v", inlineXML, profile, err)
	}
}

func TestParseAutoRepairsMalformedXMLForProfile(t *testing.T) {
	tests := []struct {
		name   string
		source string
		blocks map[string]int
		total  int
	}{
		{
			name: "missing closes and final bracket",
			source: `<title>标题</title><unknown><p>one</p></unknown>` +
				`<ul><li>A<li>B</ul><p>尾声</p`,
			blocks: map[string]int{"title": 1, "p": 2, "ul": 1, "li": 2},
			total:  6,
		},
		{
			name:   "invalid tag spacing and unquoted attributes",
			source: `< p align=center>one< / p><img src=https://example.com/a.png>`,
			blocks: map[string]int{"p": 1, "img": 1},
			total:  2,
		},
		{
			name:   "block interrupts inline nesting",
			source: `<span>x<table><tr><td>y</td></tr></table></span>`,
			blocks: map[string]int{"table": 1, "tr": 1},
			total:  2,
		},
		{
			name:   "truncated cdata keeps later blocks",
			source: `<code><![CDATA[a < b</code><p>after</p>`,
			blocks: map[string]int{"code": 1, "p": 1},
			total:  2,
		},
		{
			name:   "legacy block id does not hide following content",
			source: `<block_insert><parameter><block_id="8,9"/><content><p>x</p></content></parameter></block_insert>`,
			blocks: map[string]int{"p": 1},
			total:  1,
		},
		{
			name:   "orphan close is ignored",
			source: `</div><h1>x</h1>`,
			blocks: map[string]int{"h1": 1},
			total:  1,
		},
		{
			name:   "unterminated comment resumes at later block",
			source: `<p>one</p><!-- broken <h1>two</h1>`,
			blocks: map[string]int{"p": 1, "h1": 1},
			total:  2,
		},
		{
			name:   "unterminated opening tag is inferred",
			source: `<p`,
			blocks: map[string]int{"p": 1},
			total:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := ParseAuto(tt.source)
			if err != nil {
				t.Fatalf("ParseAuto() error = %v", err)
			}
			if profile.BlockCount != tt.total {
				t.Fatalf("profile = %+v, want %d blocks", profile, tt.total)
			}
			for tag, want := range tt.blocks {
				if got := blockCountForTest(profile.Blocks, tag); got != want {
					t.Errorf("%s blocks = %d, want %d; profile=%+v", tag, got, want, profile)
				}
			}
		})
	}
}

func TestCompatibleXMLNormalizationDoesNotRewriteProtectedText(t *testing.T) {
	source := `<p><![CDATA[<block_id="8,9">]]></p><!-- <block_id="10"> --><?note <block_id="11">?>`
	if got := normalizeCompatibleXMLInput(source); got != source {
		t.Fatalf("normalizeCompatibleXMLInput() = %q, want protected source unchanged %q", got, source)
	}
	profile, err := ParseAuto(source)
	if err != nil {
		t.Fatalf("ParseAuto() error = %v", err)
	}
	if profile.BlockCount != 1 || blockCountForTest(profile.Blocks, "p") != 1 {
		t.Fatalf("profile = %+v, want one paragraph", profile)
	}
}

func TestCompatibleXMLNormalizationDoesNotRewriteAttributeText(t *testing.T) {
	source := `<p title='<block_id="8,9">'>visible</p>`
	if got := normalizeCompatibleXMLInput(source); got != source {
		t.Fatalf("normalizeCompatibleXMLInput() = %q, want attribute source unchanged %q", got, source)
	}
}

func TestCompatibleBlockIDPatternsOnlyInspectCurrentToken(t *testing.T) {
	source := `<p>x</p><block_id="8,9"/>`
	for _, expression := range []*regexp.Regexp{
		compatibleBlockIDSelfClosing,
		compatibleBlockIDWithClosing,
		compatibleBlockIDOpen,
	} {
		if match := expression.FindStringIndex(source); match != nil {
			t.Fatalf("legacy block_id expression scanned past the current token: match=%v", match)
		}
	}
}

func TestDetectFormatHandlesManyMismatchedClosersLinearly(t *testing.T) {
	source := strings.Repeat("<a>", 20_000) + strings.Repeat("</b>", 20_000)
	if got := DetectFormat(source); got != FormatXML {
		t.Fatalf("DetectFormat() = %q, want XML", got)
	}
}

func TestParseAutoAcceptsLocalImagePath(t *testing.T) {
	profile, err := ParseAuto(`<title>Local image</title><img path="@diagram.png" caption="diagram"/>`)
	if err != nil {
		t.Fatalf("ParseAuto() error = %v", err)
	}
	if profile.BlockCount != 2 || blockCountForTest(profile.Blocks, "img") != 1 {
		t.Fatalf("profile = %+v, want one title and one img block", profile)
	}
}

func TestParseAutoDoesNotSupportLegacyQAImage(t *testing.T) {
	profile, err := ParseAuto(`<qa_image><image_key=img_v3_abc w=320 h=200></qa_image>`)
	if err != nil {
		t.Fatalf("ParseAuto() error = %v", err)
	}
	if blockCountForTest(profile.Blocks, "img") != 0 {
		t.Fatalf("profile = %+v, legacy qa_image must not be converted to img", profile)
	}
}

func TestParseAutoCompatibilityKeepsGlobalSafetyErrorsFatal(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "unsafe declaration", source: `<!DOCTYPE foo><p>text</p>`},
		{name: "invalid utf8", source: string([]byte{'<', 'p', '>', 0xff, '<', '/', 'p', '>'})},
		{name: "XML control character", source: "<p>before\x0bafter</p>"},
		{name: "XML noncharacter", source: "<p>before\ufffeafter</p>"},
		{name: "excessive nesting", source: strings.Repeat("<span>", MaxNestingDepth+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseAuto(tt.source); err == nil {
				t.Fatalf("ParseAuto(%q) succeeded, want safety error", tt.source)
			}
		})
	}
}

func TestParseRejectsXML10ForbiddenCharactersInEveryTextLocation(t *testing.T) {
	for _, source := range []string{
		"<p>before\x01after</p>",
		"<p title=\"before\x0bafter\">text</p>",
		"<code><![CDATA[before\x0cafter]]></code>",
	} {
		if _, err := Parse(source, FormatXML); err == nil {
			t.Errorf("Parse(%q) succeeded, want XML 1.0 character error", source)
		}
	}
}

func TestParseAutoDoesNotCountRawWhiteboardTagsAsBlocks(t *testing.T) {
	profile, err := ParseAuto(`<whiteboard type="svg"><svg><image href="x"/><text>raw</text></svg></whiteboard><p>visible</p>`)
	if err != nil {
		t.Fatalf("ParseAuto() error = %v", err)
	}
	if profile.BlockCount != 2 || blockCountForTest(profile.Blocks, "whiteboard") != 1 || blockCountForTest(profile.Blocks, "p") != 1 {
		t.Fatalf("profile = %+v, want only whiteboard and p blocks", profile)
	}
	if blockCountForTest(profile.Blocks, "img") != 0 {
		t.Fatalf("profile = %+v, raw whiteboard image must not be counted", profile)
	}
}

func TestParseXMLDoesNotNormalizeTagAliases(t *testing.T) {
	source := `<P>one<strong>two</strong></P><image href="https://example.com/image.png"></image><p>known</p><img>`
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original %q", result.XML, source)
	}
	if result.Profile.BlockCount != 2 ||
		blockCountForTest(result.Profile.Blocks, "p") != 1 ||
		blockCountForTest(result.Profile.Blocks, "img") != 1 {
		t.Fatalf("profile = %+v, want only canonical p and img blocks", result.Profile)
	}
}

func TestParseXMLAcceptsArbitraryAttributesWithoutChangingInput(t *testing.T) {
	source := `<callout color="blue" icon="💡"><p>x</p></callout><at id="ou_legacy"></at><img url="https://example.com/image.png"/>`
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original %q", result.XML, source)
	}
	if result.Profile.BlockCount != 3 {
		t.Fatalf("profile = %+v, want callout, p, and img blocks", result.Profile)
	}
}

func TestParseAutoCompatibilityAcceptsBareAmpersandsInAttributes(t *testing.T) {
	source := `<block_insert><parameter><block_id>-1</block_id><content><img href="https://picsum.photos/320/200?seed=lark-cli&raw=1"/></content></parameter></block_insert>`
	profile, err := ParseAuto(source)
	if err != nil {
		t.Fatalf("ParseAuto() error = %v", err)
	}
	if profile.BlockCount != 1 || blockCountForTest(profile.Blocks, "img") != 1 {
		t.Fatalf("profile = %+v, want one img block", profile)
	}
}

func TestParseXMLPreservesValidCDATA(t *testing.T) {
	source := `<code><![CDATA[a < b && c > d]]></code>`
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original %q", result.XML, source)
	}
}

func TestParseXMLAllowsDeclarationTextInsideCDATAAndComments(t *testing.T) {
	source := `<p><![CDATA[<!DOCTYPE literal>]]></p><!-- <!ENTITY literal> --><p>ok</p>`
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original %q", result.XML, source)
	}
}

func TestParseXMLPreservesWordBoundaryAcrossNewline(t *testing.T) {
	result, err := Parse("<p>Hello\nworld</p>", FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != "<p>Hello\nworld</p>" {
		t.Fatalf("XML = %q, want source unchanged", result.XML)
	}
	if result.Profile.WordCount != 2 || result.Profile.CharCount != 10 {
		t.Fatalf("profile = %+v, want word_count=2 char_count=10", result.Profile)
	}
}

func TestParseXMLPreservesUTF8BOM(t *testing.T) {
	source := "\uFEFF<p>text</p>"
	result, err := Parse(source, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != source {
		t.Fatalf("XML = %q, want original input", result.XML)
	}
}

func TestParseMarkdownConvertsLarkOpenCLIBlocks(t *testing.T) {
	source := "# 标题\n\nHello **world**.\n\n- [x] Done\n- [ ] Todo\n\n" +
		"| A | B |\n| --- | --- |\n| 1 | 2 |\n\n" +
		"```go\nfmt.Println(\"x\")\n```\n\n$E=mc^2$\n"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	for _, fragment := range []string{
		`<h1>标题</h1>`,
		`<p>Hello <b>world</b>.</p>`,
		`<checkbox done="true">Done</checkbox>`,
		`<checkbox done="false">Todo</checkbox>`,
		`<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr></tbody></table>`,
		`<pre lang="go"><code>fmt.Println("x")</code></pre>`,
		`<p><latex>E=mc^2</latex></p>`,
	} {
		if !strings.Contains(result.XML, fragment) {
			t.Errorf("XML missing %q:\n%s", fragment, result.XML)
		}
	}
}

func TestParseMarkdownDefinitionTermPreservesWordBoundaries(t *testing.T) {
	result, err := Parse("*Term* *alpha* *beta*\n: detail", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(result.XML, `<p><b><em>Term</em> <em>alpha</em> <em>beta</em></b></p>`) {
		t.Fatalf("XML = %q, want spaces between adjacent inline emphasis nodes preserved", result.XML)
	}
	if result.Profile.WordCount != 4 || result.Profile.Breakdown.EnglishWords != 4 {
		t.Fatalf("profile = %+v, want four English words", result.Profile)
	}
}

func TestParseMarkdownPreservesLineBreakSemantics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "soft breaks become spaces",
			source: "**文号：桂汛旱指〔2026〕17号**\n**签发人：XXX**\n**发布日期：2026年7月13日**",
			want:   `<p><b>文号：桂汛旱指〔2026〕17号</b> <b>签发人：XXX</b> <b>发布日期：2026年7月13日</b></p>`,
		},
		{
			name:   "hard breaks remain line breaks",
			source: "**文号：A**  \n**签发人：B**",
			want:   `<p><b>文号：A</b><br/><b>签发人：B</b></p>`,
		},
		{
			name:   "blank lines remain paragraph breaks",
			source: "**文号：A**\n\n**签发人：B**",
			want:   `<p><b>文号：A</b></p><p><b>签发人：B</b></p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source, FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if result.XML != tt.want {
				t.Fatalf("XML = %q, want %q", result.XML, tt.want)
			}
		})
	}
}

func TestParseMarkdownContainerKeepsMarkdownChildren(t *testing.T) {
	source := "<callout emoji=\"💡\">\n\n## Note\n\n- item\n\n</callout>\n"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<callout emoji="💡"><h2>Note</h2><ul><li>item</li></ul></callout>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownRejectsUnclosedContainer(t *testing.T) {
	_, err := Parse("<callout>\n\ncontent", FormatMarkdown)
	if err == nil || !strings.Contains(err.Error(), "missing closing tag") {
		t.Fatalf("Parse() error = %v, want missing container close", err)
	}
}

func TestParseMarkdownHandlesEscapedMathDollar(t *testing.T) {
	result, err := Parse(`$a\$b$`, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<p><latex>a$b</latex></p>` {
		t.Fatalf("XML = %q", result.XML)
	}
}

func TestParseMarkdownHandlesEscapedMathDollarAcrossLines(t *testing.T) {
	result, err := Parse("$a\\$\nb$", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != "<p><latex>a$ b</latex></p>" {
		t.Fatalf("XML = %q", result.XML)
	}
}

func TestParseMarkdownHandlesEscapedDollarAdjacentToBlockMathCloser(t *testing.T) {
	result, err := Parse(`$$price: \$$$`, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<p><latex>price: $</latex></p>` {
		t.Fatalf("XML = %q", result.XML)
	}
}

func TestParseMarkdownPreservesLatexBackslashes(t *testing.T) {
	result, err := Parse(`$x\_1 \& y$`, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<p><latex>x\_1 \&amp; y</latex></p>` {
		t.Fatalf("XML = %q", result.XML)
	}
}

func TestParseMarkdownContainerAllowsGreaterThanInQuotedAttribute(t *testing.T) {
	result, err := Parse("<callout title=\"A > B\">\n\ntext\n\n</callout>", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<callout title="A &gt; B"><p>text</p></callout>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownContainerAllowsUnquotedAttributes(t *testing.T) {
	result, err := Parse("<callout type=info>\n\ntext\n\n</callout>", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<callout type="info"><p>text</p></callout>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownDoesNotRepairWhitespaceAfterContainerOpenBracket(t *testing.T) {
	result, err := Parse("< callout>\ntext\n</callout>", FormatMarkdown)
	if err == nil && strings.Contains(result.XML, "<callout>") {
		t.Fatalf("Parse() silently repaired invalid container spacing: %q", result.XML)
	}
}

func TestParseMarkdownPreservesNativeLatexEscapes(t *testing.T) {
	result, err := Parse(`<latex>\$</latex>`, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<p><latex>\$</latex></p>` {
		t.Fatalf("XML = %q, want native latex escape preserved", result.XML)
	}
}

func TestParseMarkdownPreservesMixedPlainAndTaskItems(t *testing.T) {
	result, err := Parse("- plain\n- [x] done", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<ul><li>plain</li></ul><checkbox done="true">done</checkbox>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownPreservesOrderedMixedPlainAndTaskItems(t *testing.T) {
	result, err := Parse("1. plain\n2. [x] done", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<ol><li>plain</li></ol><checkbox done="true">done</checkbox>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownPreservesOrderedMixedListSequence(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "3. plain\n4. [x] done", want: `<ol><li seq="3">plain</li></ol><checkbox done="true">done</checkbox>`},
		{source: "1. [x] done\n2. plain", want: `<checkbox done="true">done</checkbox><ol><li seq="2">plain</li></ol>`},
	}
	for _, test := range tests {
		result, err := Parse(test.source, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", test.source, err)
		}
		if result.XML != test.want {
			t.Errorf("Parse(%q) XML = %q, want %q", test.source, result.XML, test.want)
		}
	}
}

func TestNormalizeListIndentClearsCompletedListContext(t *testing.T) {
	source := "- a\n\nparagraph\n  - b"
	want := "- a\n\nparagraph\n- b"
	if got := normalizeListIndent(source); got != want {
		t.Fatalf("normalizeListIndent() = %q, want %q", got, want)
	}
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.XML != `<ul><li>a</li></ul><p>paragraph</p><ul><li>b</li></ul>` {
		t.Fatalf("XML = %q", result.XML)
	}
}

func TestNormalizeListIndentKeepsMismatchedFenceMarkerOpen(t *testing.T) {
	source := "````\n~~~\n  - literal code\n````\n- outside"
	if got := normalizeListIndent(source); got != source {
		t.Fatalf("normalizeListIndent() = %q, want fenced source unchanged %q", got, source)
	}
}

func TestParseMarkdownKeepsFenceInsideNestedList(t *testing.T) {
	source := "- outer\n  - inner\n    ```text\n    - literal\n    ```"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<ul><li>outer<ul><li>inner<pre lang="text"><code>- literal</code></pre></li></ul></li></ul>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownDoesNotCloseFenceWithOverIndentedMarker(t *testing.T) {
	source := "```text\n    ```\n  - literal\n```"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := "<pre lang=\"text\"><code>    ```<br/>  - literal</code></pre>"
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownClosesNestedFenceWithWideListPadding(t *testing.T) {
	source := "- outer\n  -   inner\n      ```text\n      body\n         ```\n- root"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<ul><li>outer<ul><li>inner<pre lang="text"><code>body</code></pre></li></ul></li><li>root</li></ul>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownKeepsLazyContinuationListContext(t *testing.T) {
	source := "- outer\ncontinuation\n  - nested"
	result, err := Parse(source, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `<ul><li>outer continuation<ul><li>nested</li></ul></li></ul>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownKeepsInlineLazyContinuationListContext(t *testing.T) {
	tests := []struct {
		name   string
		middle string
	}{
		{name: "inline HTML", middle: "<span>continuation</span>"},
		{name: "autolink", middle: "<https://example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := "- outer\n" + tt.middle + "\n  - nested"
			result, err := Parse(source, FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !strings.Contains(result.XML, `<li>outer `) || !strings.Contains(result.XML, `<ul><li>nested</li></ul></li>`) {
				t.Fatalf("nested list escaped lazy continuation context: %s", result.XML)
			}
		})
	}
}

func TestParseMarkdownThematicBreakEndsLazyListContext(t *testing.T) {
	for _, separator := range []string{"____", "*****", "* * *"} {
		t.Run(separator, func(t *testing.T) {
			result, err := Parse("- outer\n"+separator+"\n  - nested", FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			want := `<ul><li>outer</li></ul><hr/><ul><li>nested</li></ul>`
			if result.XML != want {
				t.Fatalf("XML = %q, want %q", result.XML, want)
			}
		})
	}
}

func TestParseMarkdownThematicBreakKeepsCRLFAndNestedListContext(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "root CRLF",
			source: "- outer\r\n____\r\n  - nested",
			want:   `<ul><li>outer</li></ul><hr/><ul><li>nested</li></ul>`,
		},
		{
			name:   "nested indentation",
			source: "- outer\n  - inner\n    ****\n    after",
			want:   `<ul><li>outer<ul><li>inner<hr/>after</li></ul></li></ul>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source, FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if result.XML != tt.want {
				t.Fatalf("XML = %q, want %q", result.XML, tt.want)
			}
		})
	}
}

func TestParseMarkdownDoesNotTreatIndentedBackticksAsFence(t *testing.T) {
	result, err := Parse("    ```\n**你好。**S", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(result.XML, `<b>你好。</b>S`) {
		t.Fatalf("XML lost CJK emphasis after indented code: %s", result.XML)
	}
}

func TestParseMarkdownDoesNotFlattenNestedCJKMarkup(t *testing.T) {
	result, err := Parse(`**中文 *重点*。**下一步`, FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	const want = `<p><b>中文 <em>重点</em>。</b>下一步</p>`
	if result.XML != want {
		t.Fatalf("XML = %q, want nested emphasis %q", result.XML, want)
	}
}

func TestParseMarkdownKeepsNestedLinkAndCodeInCJKMarkup(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "link",
			source: `**中文 [链接](https://example.com)。**下一步`,
			want:   `<p><b>中文 <a href="https://example.com">链接</a>。</b>下一步</p>`,
		},
		{
			name:   "code",
			source: "**中文 `code`。**下一步",
			want:   `<p><b>中文 <code>code</code>。</b>下一步</p>`,
		},
		{
			name:   "code with delimiter",
			source: "**中文 `code ** literal`。**下一步",
			want:   `<p><b>中文 <code>code ** literal</code>。</b>下一步</p>`,
		},
		{
			name:   "link destination with delimiter",
			source: `**中文 [链接](https://example.com/a**b)。**下一步`,
			want:   `<p><b>中文 <a href="https://example.com/a**b">链接</a>。</b>下一步</p>`,
		},
		{
			name:   "link title with parenthesis and delimiter",
			source: `**中文 [link](https://example.com "title ) ** literal")。**next`,
			want:   `<p><b>中文 <a href="https://example.com" title="title ) ** literal">link</a>。</b>next</p>`,
		},
		{
			name:   "link destination with apostrophe",
			source: `**中文 [link](https://example.com/a'b)。**next`,
			want:   `<p><b>中文 <a href="https://example.com/a&#39;b">link</a>。</b>next</p>`,
		},
		{
			name:   "code closer preceded by backslash",
			source: "**中文 `code ** literal\\`。**next",
			want:   `<p><b>中文 <code>code ** literal\</code>。</b>next</p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source, FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if result.XML != tt.want {
				t.Fatalf("XML = %q, want %q", result.XML, tt.want)
			}
		})
	}
}

func TestParseMarkdownKeepsMultilineCodeSpanLiteral(t *testing.T) {
	result, err := Parse("`code\n**中文。**next\n`", FormatMarkdown)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	const want = `<p><code>code<br/>**中文。**next</code></p>`
	if result.XML != want {
		t.Fatalf("XML = %q, want %q", result.XML, want)
	}
}

func TestParseMarkdownCJKCompatibilityRespectsBlockAndInlineBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "nested list",
			source: "- outer\n  - **你好。**S",
			want:   `<ul><li>outer<ul><li><b>你好。</b>S</li></ul></li></ul>`,
		},
		{
			name:   "heading interrupts paragraph",
			source: "`literal\n# **中文。**S\n`",
			want:   "<p>`literal</p><h1><b>中文。</b>S</h1><p>`</p>",
		},
		{
			name:   "over-indented blockquote fence closer",
			source: "> ```text\n>     ```\n> **中文。**S\n> ```",
			want:   "<blockquote><pre lang=\"text\"><code>    ```<br/>**中文。**S</code></pre></blockquote>",
		},
		{
			name:   "multiline link title",
			source: "[link](https://example.com\n \"**中文。**S\")",
			want:   `<p><a href="https://example.com" title="**中文。**S">link</a></p>`,
		},
		{
			name:   "inline raw code",
			source: `<code>**raw。**S</code>**你好。**S`,
			want:   `<p><code>**raw。**S</code><b>你好。</b>S</p>`,
		},
		{
			name:   "raw code delimiter cannot escape",
			source: `<code>**raw</code>。**S`,
			want:   `<p><code>**raw</code>。**S</p>`,
		},
		{
			name:   "balanced raw code markup keeps native semantics",
			source: `<code>__under__ **star** ~~tilde~~</code>`,
			want:   `<p><code><b>under</b> <b>star</b> <del>tilde</del></code></p>`,
		},
		{
			name:   "outer markup can wrap raw code",
			source: `**before <code>inside</code> after**`,
			want:   `<p><b>before <code>inside</code> after</b></p>`,
		},
		{
			name:   "raw code delimiter cannot consume outer opener",
			source: `**before <code>inside**</code> after**`,
			want:   `<p><b>before <code>inside**</code> after</b></p>`,
		},
		{
			name:   "raw code underscore cannot consume outer opener",
			source: `__before <code>inside__</code> after__`,
			want:   `<p><b>before <code>inside__</code> after</b></p>`,
		},
		{
			name:   "multiline raw code opening tag",
			source: "<code\n class=\"x\">**raw。**S</code>",
			want:   `<p><code class="x">**raw。**S</code></p>`,
		},
		{
			name:   "mismatched delimiter runs stay literal",
			source: `~plain。~~next and *plain。**next`,
			want:   `<p>~plain。~~next and *plain。**next</p>`,
		},
		{
			name:   "relaxed closer skips nearer mismatched opener",
			source: `**outer ***inner。**S`,
			want:   `<p><b>outer ***inner。</b>S</p>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source, FormatMarkdown)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if result.XML != tt.want {
				t.Fatalf("XML = %q, want %q", result.XML, tt.want)
			}
		})
	}
}

func TestParseMarkdownMatchesLarkOpenCLIFixtures(t *testing.T) {
	t.Run("deep nested list", func(t *testing.T) {
		result, err := Parse("1. 第一层\n   - 第二层\n     - 第三层\n       - 第四层\n", FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if strings.Contains(result.XML, "<pre>") || strings.Contains(result.XML, "<code>") || !strings.Contains(result.XML, "第四层") {
			t.Fatalf("nested list converted incorrectly: %s", result.XML)
		}
	})

	t.Run("fenced mermaid", func(t *testing.T) {
		result, err := Parse("```mermaid\nflowchart LR\nA-->B\n```", FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		want := `<whiteboard type="mermaid">flowchart LR<br/>A--&gt;B</whiteboard>`
		if result.XML != want {
			t.Fatalf("XML = %q, want %q", result.XML, want)
		}
	})

	t.Run("raw whiteboard source", func(t *testing.T) {
		source := "<whiteboard type=\"mermaid\">\nflowchart LR\n  A --> B\n</whiteboard>"
		result, err := Parse(source, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		want := `<whiteboard type="mermaid">flowchart LR<br/>  A --&gt; B</whiteboard>`
		if result.XML != want {
			t.Fatalf("XML = %q, want %q", result.XML, want)
		}
	})

	t.Run("raw code stays literal", func(t *testing.T) {
		source := "<code lang=\"go\">\nif a < b && c > d {\n  fmt.Println(\"**raw**\")\n}\n</code>"
		result, err := Parse(source, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		want := `<code lang="go">if a &lt; b &amp;&amp; c &gt; d {<br/>  fmt.Println("**raw**")<br/>}</code>`
		if result.XML != want {
			t.Fatalf("XML = %q, want %q", result.XML, want)
		}
	})

	t.Run("underscore tags", func(t *testing.T) {
		result, err := Parse(`text <synced_reference src-block-id="abc" src-token="def"/> more`, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if !strings.Contains(result.XML, `<synced_reference`) || strings.Contains(result.XML, `&lt;synced_reference`) {
			t.Fatalf("underscore tag was not preserved: %s", result.XML)
		}
	})

	t.Run("canonical user cite", func(t *testing.T) {
		result, err := Parse(`hello <cite type="user" user-id="ou_user"></cite>`, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		for _, want := range []string{`<cite`, `type="user"`, `user-id="ou_user"`} {
			if !strings.Contains(result.XML, want) {
				t.Errorf("XML missing %q: %s", want, result.XML)
			}
		}
	})

	t.Run("raw tag alias stays unchanged", func(t *testing.T) {
		result, err := Parse(`hello <strong>world</strong>`, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if result.XML != `<p>hello <strong>world</strong></p>` {
			t.Fatalf("XML = %q", result.XML)
		}
	})

	t.Run("raw cite alias and attributes stay unchanged", func(t *testing.T) {
		result, err := Parse(`hello <at id="ou_legacy"></at>`, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if result.XML != `<p>hello <at id="ou_legacy"></at></p>` {
			t.Fatalf("XML = %q", result.XML)
		}
	})

	t.Run("markdown backslash escapes", func(t *testing.T) {
		result, err := Parse(`"source\_token": \[abc\] path\\to`, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		for _, want := range []string{`source_token`, `[abc]`, `path\to`} {
			if !strings.Contains(result.XML, want) {
				t.Errorf("XML missing %q: %s", want, result.XML)
			}
		}
	})

	t.Run("adjacent CJK emphasis", func(t *testing.T) {
		source := `***你好。***S 和 ~~再见。~~T。**agent team 做 brownfield 项目，带来的感知会强烈得多**——前提。**这个时刻，才是真正属于 agent team 的"闪光时刻"。**翟霖`
		result, err := Parse(source, FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		for _, want := range []string{
			`<em><b>你好。</b></em>S`,
			`<del>再见。</del>T`,
			`<b>agent team 做 brownfield 项目，带来的感知会强烈得多</b>`,
			`<b>这个时刻，才是真正属于 agent team 的"闪光时刻"。</b>翟霖`,
		} {
			if !strings.Contains(result.XML, want) {
				t.Errorf("XML missing %q: %s", want, result.XML)
			}
		}
	})

	t.Run("div parses markdown children", func(t *testing.T) {
		result, err := Parse("<div>\n\n**bold**\n\n</div>", FormatMarkdown)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if result.XML != `<div><p><b>bold</b></p></div>` {
			t.Fatalf("XML = %q", result.XML)
		}
	})
}

func TestTextProfileMatchesLarkOpenCLIContract(t *testing.T) {
	result, err := Parse(`<title>标题</title><p>一个苹果是 an apple。</p>`, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := result.Profile
	if profile.WordCount != 10 || profile.CharCount != 15 {
		t.Fatalf("profile = %+v, want word_count=10 char_count=15", profile)
	}
	if profile.Breakdown.HanChars != 7 || profile.Breakdown.EnglishWords != 2 || profile.Breakdown.ChinesePunctuations != 1 {
		t.Fatalf("breakdown = %+v", profile.Breakdown)
	}
}

func TestTextProfileMatchesAuthoringCounterCases(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		words     int
		chars     int
		blocks    int
		english   int
		numbers   int
		han       int
		listItems int
	}{
		{
			name:   "english number and punctuation",
			source: `<p>Hello world 123.45。</p>`,
			words:  4, chars: 17, blocks: 1, english: 2, numbers: 1,
		},
		{
			name:   "list and checkbox markers",
			source: `<ul><li>甲</li><li>two</li></ul><checkbox done="true">完成</checkbox>`,
			words:  7, chars: 9, blocks: 4, english: 1, han: 3, listItems: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source, FormatXML)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			profile := result.Profile
			if profile.WordCount != tt.words || profile.CharCount != tt.chars || profile.BlockCount != tt.blocks {
				t.Fatalf("profile = %+v, want words=%d chars=%d blocks=%d", profile, tt.words, tt.chars, tt.blocks)
			}
			if profile.Breakdown.EnglishWords != tt.english || profile.Breakdown.NumberWords != tt.numbers || profile.Breakdown.HanChars != tt.han {
				t.Fatalf("breakdown = %+v", profile.Breakdown)
			}
			if got := blockCountForTest(profile.Blocks, "li"); got != tt.listItems {
				t.Fatalf("li count = %d, want %d", got, tt.listItems)
			}
		})
	}
}

func TestTextProfileCountsNumericCodeLexeme(t *testing.T) {
	result, err := Parse(`<pre><code>123</code></pre>`, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Profile.WordCount != 1 || result.Profile.Breakdown.NumberWords != 1 || result.Profile.Breakdown.Digits != 3 {
		t.Fatalf("profile = %+v, want one numeric code word", result.Profile)
	}
}

func TestTextProfileUsesVisibleAttributeFallbacks(t *testing.T) {
	result, err := Parse(`<p text="Hello"/><p><span title="world"/></p><p><a title="tooltip">Click here</a></p><img href="https://example.com/image.png" caption="图"/>`, FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	profile := result.Profile
	if profile.WordCount != 5 || profile.CharCount != 20 {
		t.Fatalf("profile = %+v, want word_count=5 char_count=20", profile)
	}
	if profile.Breakdown.EnglishWords != 4 || profile.Breakdown.HanChars != 1 {
		t.Fatalf("breakdown = %+v", profile.Breakdown)
	}
}

func TestListMarkersUseMarkerSegments(t *testing.T) {
	nodes, err := parseXML(`<ul><li>one</li></ul><ol><li>two</li></ol>`)
	if err != nil {
		t.Fatalf("parseXML() error = %v", err)
	}
	markers := map[string]segmentKind{}
	for _, segment := range extractSegments(nodes) {
		if segment.text == "•" || segment.text == "1." {
			markers[segment.text] = segment.kind
		}
	}
	for _, marker := range []string{"•", "1."} {
		if markers[marker] != segmentMarker {
			t.Fatalf("marker %q kind = %v, want segmentMarker", marker, markers[marker])
		}
	}
}

func TestTextProfileHandlesLongASCIIWord(t *testing.T) {
	word := strings.Repeat("a", 100_000)
	result, err := Parse("<p>"+word+"</p>", FormatXML)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Profile.WordCount != 1 || result.Profile.CharCount != len(word) {
		t.Fatalf("profile = %+v", result.Profile)
	}
}

var (
	kindUnsupportedTestBlock  = gast.NewNodeKind("UnsupportedTestBlock")
	kindUnsupportedTestInline = gast.NewNodeKind("UnsupportedTestInline")
)

type unsupportedTestBlock struct{ gast.BaseBlock }

func (n *unsupportedTestBlock) Kind() gast.NodeKind { return kindUnsupportedTestBlock }
func (n *unsupportedTestBlock) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

type unsupportedTestInline struct{ gast.BaseInline }

func (n *unsupportedTestInline) Kind() gast.NodeKind { return kindUnsupportedTestInline }
func (n *unsupportedTestInline) Dump(source []byte, level int) {
	gast.DumpHelper(n, source, level, nil, nil)
}

func TestMarkdownRendererRejectsUnsupportedNodes(t *testing.T) {
	if _, err := renderBlockNode(&unsupportedTestBlock{}, nil); err == nil || !strings.Contains(err.Error(), "UnsupportedTestBlock") {
		t.Fatalf("renderBlockNode() error = %v", err)
	}
	if _, err := renderInlineNode(&unsupportedTestInline{}, nil); err == nil || !strings.Contains(err.Error(), "UnsupportedTestInline") {
		t.Fatalf("renderInlineNode() error = %v", err)
	}
}

func TestParseRejectsUnsafeXMLDeclarations(t *testing.T) {
	_, err := Parse(`<!DOCTYPE foo [<!ENTITY x "value">]><p>&x;</p>`, FormatXML)
	if err == nil || !strings.Contains(err.Error(), "DOCTYPE or ENTITY") {
		t.Fatalf("Parse() error = %v, want unsafe declaration rejection", err)
	}
}

func TestParseRejectsInvalidUTF8(t *testing.T) {
	_, err := Parse(string([]byte{'<', 'p', '>', 0xff, '<', '/', 'p', '>'}), FormatXML)
	if err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("Parse() error = %v, want UTF-8 rejection", err)
	}
}

func TestParseRejectsExcessiveNesting(t *testing.T) {
	source := strings.Repeat("<span>", MaxNestingDepth+1)
	_, err := Parse(source, FormatXML)
	if err == nil || !strings.Contains(err.Error(), "nesting exceeds") {
		t.Fatalf("Parse() error = %v, want nesting limit rejection", err)
	}
}

func TestParseXMLRejectsNestedInvalidTagStarts(t *testing.T) {
	if _, err := Parse(`<<<<p>text</p>`, FormatXML); err == nil {
		t.Fatal("Parse() succeeded, want invalid XML token error")
	}
}

func blockCountForTest(blocks []BlockShare, typ string) int {
	for _, block := range blocks {
		if block.Type == typ {
			return block.Count
		}
	}
	return 0
}
