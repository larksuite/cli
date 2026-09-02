// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"regexp"
	"strings"
	"testing"
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

func TestParseCompatibleXMLRepairsMalformedXMLForProfile(t *testing.T) {
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
			profile, err := ParseCompatibleXML(tt.source)
			if err != nil {
				t.Fatalf("ParseCompatibleXML() error = %v", err)
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

func TestParseCompatibleXMLRejectsMarkdown(t *testing.T) {
	if _, err := ParseCompatibleXML("# Heading\n\n- item"); err == nil || !strings.Contains(err.Error(), "must begin with '<'") {
		t.Fatalf("ParseCompatibleXML() error = %v, want XML input rejection", err)
	}
}

func TestCompatibleXMLNormalizationDoesNotRewriteProtectedText(t *testing.T) {
	source := `<p><![CDATA[<block_id="8,9">]]></p><!-- <block_id="10"> --><?note <block_id="11">?>`
	if got := normalizeCompatibleXMLInput(source); got != source {
		t.Fatalf("normalizeCompatibleXMLInput() = %q, want protected source unchanged %q", got, source)
	}
	profile, err := ParseCompatibleXML(source)
	if err != nil {
		t.Fatalf("ParseCompatibleXML() error = %v", err)
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

func TestParseCompatibleXMLAcceptsLocalImagePath(t *testing.T) {
	profile, err := ParseCompatibleXML(`<title>Local image</title><img path="@diagram.png" caption="diagram"/>`)
	if err != nil {
		t.Fatalf("ParseCompatibleXML() error = %v", err)
	}
	if profile.BlockCount != 2 || blockCountForTest(profile.Blocks, "img") != 1 {
		t.Fatalf("profile = %+v, want one title and one img block", profile)
	}
}

func TestParseCompatibleXMLDoesNotSupportLegacyQAImage(t *testing.T) {
	profile, err := ParseCompatibleXML(`<qa_image><image_key=img_v3_abc w=320 h=200></qa_image>`)
	if err != nil {
		t.Fatalf("ParseCompatibleXML() error = %v", err)
	}
	if blockCountForTest(profile.Blocks, "img") != 0 {
		t.Fatalf("profile = %+v, legacy qa_image must not be converted to img", profile)
	}
}

func TestParseCompatibleXMLCompatibilityKeepsGlobalSafetyErrorsFatal(t *testing.T) {
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
			if _, err := ParseCompatibleXML(tt.source); err == nil {
				t.Fatalf("ParseCompatibleXML(%q) succeeded, want safety error", tt.source)
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

func TestParseCompatibleXMLDoesNotCountRawWhiteboardTagsAsBlocks(t *testing.T) {
	profile, err := ParseCompatibleXML(`<whiteboard type="svg"><svg><image href="x"/><text>raw</text></svg></whiteboard><p>visible</p>`)
	if err != nil {
		t.Fatalf("ParseCompatibleXML() error = %v", err)
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

func TestParseCompatibleXMLCompatibilityAcceptsBareAmpersandsInAttributes(t *testing.T) {
	source := `<block_insert><parameter><block_id>-1</block_id><content><img href="https://picsum.photos/320/200?seed=lark-cli&raw=1"/></content></parameter></block_insert>`
	profile, err := ParseCompatibleXML(source)
	if err != nil {
		t.Fatalf("ParseCompatibleXML() error = %v", err)
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
