// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"os"
	"strings"
	"testing"
)

// The pure-function contract tests for prettyPrintXML (golden strings,
// whitespace character references, leaf whitespace, CDATA, idempotency,
// malformed rejection) live in slides_xml_get_test.go, unchanged from the
// original etree-based implementation. This file adds engine-level cases
// specific to the offset-slicing implementation.

func TestPrettyPrintXMLGoldenPresentation(t *testing.T) {
	input := `<presentation><slide id="s1"><shape id="a">hello</shape></slide></presentation>`
	want := "<presentation>\n  <slide id=\"s1\">\n    <shape id=\"a\">hello</shape>\n  </slide>\n</presentation>\n"
	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if got != want {
		t.Fatalf("prettyPrintXML(%q) = %q, want %q", input, got, want)
	}
}

func TestPrettyPrintXMLGoldenSlide(t *testing.T) {
	input := `<slide id="slide_1"><data><shape id="a"/></data></slide>`
	want := "<slide id=\"slide_1\">\n  <data>\n    <shape id=\"a\"/>\n  </data>\n</slide>\n"
	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if got != want {
		t.Fatalf("prettyPrintXML(%q) = %q, want %q", input, got, want)
	}
}

// TestPrettyPrintXMLRejectsMalformedInputTable pins that the whole document
// is decoded before anything is emitted: even a late syntax error yields no
// partial output, only the error the fallback path reports.
func TestPrettyPrintXMLRejectsMalformedInputTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"mismatched close tag", `<presentation><slide></presentation>`},
		{"unclosed slide from fallback test", `<slide><data></slide>`},
		{"invalid control character", "<presentation><title>\x0b</title><slide/></presentation>"},
		{"unclosed root", `<presentation><slide/>`},
		{"undefined entity", `<presentation><title>&nbsp;</title></presentation>`},
		{"bare close tag", `</presentation>`},
		{"unescaped cdata terminator in text", `<presentation><title>a]]>b</title></presentation>`},
		{"late error after valid prefix", `<presentation><slide/><slide/><slide id=></presentation>`},
		{"empty input", ``},
		{"whitespace-only input", `   `},
		{"plain text without markup", `hello`},
		{"comment-only document", `<!-- only a comment -->`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prettyPrintXML(tt.input)
			if err == nil {
				t.Fatalf("prettyPrintXML(%q) = %q, want error", tt.input, got)
			}
			if got != "" {
				t.Fatalf("prettyPrintXML(%q) returned partial output %q alongside error %v", tt.input, got, err)
			}
		})
	}
}

// TestPrettyPrintXMLIgnoresMaskingEraPlaceholderText pins that user content
// resembling the previous implementation's masking placeholders
// (LARKCLI_XML_WHITESPACE_REFERENCE_<n>_) flows through untouched now that
// no masking exists at all.
func TestPrettyPrintXMLIgnoresMaskingEraPlaceholderText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "placeholder-shaped text in p",
			input: `<content><p>LARKCLI_XML_WHITESPACE_REFERENCE_0_&#32;end</p></content>`,
			want:  "<content>\n  <p>LARKCLI_XML_WHITESPACE_REFERENCE_0_&#32;end</p>\n</content>\n",
		},
		{
			name:  "placeholder-shaped text in leaf",
			input: `<presentation><title>LARKCLI_XML_WHITESPACE_REFERENCE_1_</title><slide/></presentation>`,
			want:  "<presentation>\n  <title>LARKCLI_XML_WHITESPACE_REFERENCE_1_</title>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "placeholder-shaped attribute value",
			input: `<presentation><slide note="LARKCLI_XML_WHITESPACE_REFERENCE_0_"><shape/></slide></presentation>`,
			want:  "<presentation>\n  <slide note=\"LARKCLI_XML_WHITESPACE_REFERENCE_0_\">\n    <shape/>\n  </slide>\n</presentation>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prettyPrintXML(tt.input)
			if err != nil {
				t.Fatalf("prettyPrintXML(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("prettyPrintXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestPrettyPrintXMLStructuralTable covers comments, processing
// instructions, prolog/DOCTYPE, mixed text between structural children,
// CRLF pre-formatting, and multi-byte UTF-8 around offset boundaries.
// Expected outputs were verified byte-identical against the previous
// etree-based implementation via a differential probe.
func TestPrettyPrintXMLStructuralTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		// wantSecond is the expected output of formatting the output again.
		// Usually equal to want (idempotent); the mixed-content rows pin the
		// one known non-idempotent shape, where kept text merges with the
		// inserted indent on reparse — byte-identical to the previous
		// implementation's behavior on the same inputs. Real SML structural
		// elements carry no mixed text, so the contract's idempotency
		// guarantee is unaffected.
		wantSecond string
	}{
		{
			name:  "comment child is indented like an element",
			input: `<presentation><!-- deck notes --><slide/></presentation>`,
			want:  "<presentation>\n  <!-- deck notes -->\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "processing instruction child is indented like an element",
			input: `<presentation><?pi data?><slide/></presentation>`,
			want:  "<presentation>\n  <?pi data?>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "xml declaration prolog stays glued to the root",
			input: `<?xml version="1.0" encoding="UTF-8"?><presentation><slide/></presentation>`,
			want:  "<?xml version=\"1.0\" encoding=\"UTF-8\"?><presentation>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "prolog with doctype and trailing newline preserved verbatim",
			input: "<?xml version=\"1.0\"?>\n<!DOCTYPE presentation>\n<presentation><slide/></presentation>\n",
			want:  "<?xml version=\"1.0\"?>\n<!DOCTYPE presentation>\n<presentation>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "document-level trailing comment preserved verbatim",
			input: "<presentation><slide/></presentation><!-- tail -->",
			want:  "<presentation>\n  <slide/>\n</presentation><!-- tail -->\n",
		},
		{
			name:       "kept mixed text glues to previous sibling and close tag",
			input:      `<data>x<child/>y</data>`,
			want:       "<data>x\n  <child/>y</data>\n",
			wantSecond: "<data>x\n  \n  <child/>y</data>\n",
		},
		{
			name:       "kept mixed text does not suppress indent of next element",
			input:      `<data>x<child/>y<child/></data>`,
			want:       "<data>x\n  <child/>y\n  <child/>\n</data>\n",
			wantSecond: "<data>x\n  \n  <child/>y\n  \n  <child/>\n</data>\n",
		},
		{
			name:  "pre-existing CRLF formatting is dropped and rebuilt",
			input: "<presentation>\r\n\t<slide/>\r\n</presentation>",
			want:  "<presentation>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "multi-byte UTF-8 text and attributes keep exact bytes",
			input: `<presentation><title>原生图表 📊 Chart</title><slide 备注="中文värde"><shape/></slide></presentation>`,
			want:  "<presentation>\n  <title>原生图表 📊 Chart</title>\n  <slide 备注=\"中文värde\">\n    <shape/>\n  </slide>\n</presentation>\n",
		},
		{
			name:  "namespace-prefixed p is still text-bearing",
			input: `<content xmlns:sml="urn:x"><sml:p><span>a</span>&#32;<span>b</span></sml:p></content>`,
			want:  "<content xmlns:sml=\"urn:x\">\n  <sml:p><span>a</span>&#32;<span>b</span></sml:p>\n</content>\n",
		},
		{
			name:  "already formatted input is preserved",
			input: "<presentation>\n  <slide id=\"s1\">\n    <shape id=\"a\">hello</shape>\n  </slide>\n</presentation>\n",
			want:  "<presentation>\n  <slide id=\"s1\">\n    <shape id=\"a\">hello</shape>\n  </slide>\n</presentation>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prettyPrintXML(tt.input)
			if err != nil {
				t.Fatalf("prettyPrintXML(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("prettyPrintXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
			wantSecond := tt.wantSecond
			if wantSecond == "" {
				wantSecond = tt.want
			}
			again, err := prettyPrintXML(got)
			if err != nil {
				t.Fatalf("prettyPrintXML(second pass, %q): %v", got, err)
			}
			if again != wantSecond {
				t.Fatalf("second pass:\nonce:  %q\ntwice: %q\nwant:  %q", got, again, wantSecond)
			}
		})
	}
}

// TestPrettyPrintXMLPreservesLexicalFormsEtreeChanged pins the cases where
// slicing original bytes intentionally differs from the previous
// etree-based parse-and-reserialize implementation. Each case preserves the
// input MORE faithfully than before; none is covered by the original
// contract tests. The etree field records the old output for the record.
func TestPrettyPrintXMLPreservesLexicalFormsEtreeChanged(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // current behavior: original bytes preserved
		etree string // what the etree-based implementation produced
	}{
		{
			name:  "whitespace-only CDATA between structural children is kept",
			input: `<data><![CDATA[ ]]><child/></data>`,
			want:  "<data><![CDATA[ ]]>\n  <child/>\n</data>\n",
			etree: "<data>\n  <child/>\n</data>\n",
		},
		{
			name:  "empty element with explicit close tag is not collapsed",
			input: `<slide><data></data><shape/></slide>`,
			want:  "<slide>\n  <data></data>\n  <shape/>\n</slide>\n",
			etree: "<slide>\n  <data/>\n  <shape/>\n</slide>\n",
		},
		{
			name:  "non-whitespace character reference keeps its lexical form",
			input: `<presentation><title>&#65;&amp;&#x4E2D;</title><slide/></presentation>`,
			want:  "<presentation>\n  <title>&#65;&amp;&#x4E2D;</title>\n  <slide/>\n</presentation>\n",
			etree: "<presentation>\n  <title>A&amp;中</title>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "single-quoted attributes keep their quoting",
			input: `<presentation><slide id='s1'><shape/></slide></presentation>`,
			want:  "<presentation>\n  <slide id='s1'>\n    <shape/>\n  </slide>\n</presentation>\n",
			etree: "<presentation>\n  <slide id=\"s1\">\n    <shape/>\n  </slide>\n</presentation>\n",
		},
		{
			name:  "in-tag whitespace is preserved verbatim",
			input: "<presentation><slide  id=\"s1\" ><shape/></slide ></presentation>",
			want:  "<presentation>\n  <slide  id=\"s1\" >\n    <shape/>\n  </slide >\n</presentation>\n",
			etree: "<presentation>\n  <slide id=\"s1\">\n    <shape/>\n  </slide>\n</presentation>\n",
		},
		{
			name:  "literal > in leaf text is not re-escaped",
			input: `<presentation><title>a>b</title><slide/></presentation>`,
			want:  "<presentation>\n  <title>a>b</title>\n  <slide/>\n</presentation>\n",
			etree: "<presentation>\n  <title>a&gt;b</title>\n  <slide/>\n</presentation>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prettyPrintXML(tt.input)
			if err != nil {
				t.Fatalf("prettyPrintXML(%q): %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("prettyPrintXML(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if tt.want == tt.etree {
				t.Fatalf("case is not a divergence: want == etree == %q", tt.want)
			}
			again, err := prettyPrintXML(got)
			if err != nil {
				t.Fatalf("prettyPrintXML(second pass, %q): %v", got, err)
			}
			if again != got {
				t.Fatalf("not idempotent:\nonce:  %q\ntwice: %q", got, again)
			}
		})
	}
}

// loadChartDemo reads the real-world chart demo shipped with the
// lark-slides skill (~60KB, pretty-printed): the closest in-repo stand-in
// for a full presentation read.
func loadChartDemo(t testing.TB) string {
	t.Helper()
	data, err := os.ReadFile("../../skills/lark-slides/references/slides_chart_demo.xml")
	if err != nil {
		t.Fatalf("read chart demo fixture: %v", err)
	}
	return string(data)
}

// minifyXML strips whitespace-only text children of structural (non
// text-bearing, element-bearing) elements — the exact text nodes
// prettyPrintXML treats as disposable formatting — producing the
// single-line element shape the slides server actually returns.
// Document-level tokens (prolog, trailing newline) pass through verbatim,
// because the formatter preserves them verbatim too.
func minifyXML(t testing.TB, input string) string {
	t.Helper()
	tokens, err := tokenize(input)
	if err != nil {
		t.Fatalf("tokenize for minify: %v", err)
	}
	var out strings.Builder
	var emitElement func(startIndex int)
	emitElement = func(startIndex int) {
		start := tokens[startIndex]
		end := tokens[start.match]
		if textBearingTags[start.local] || !hasElementChild(tokens, startIndex) {
			out.WriteString(input[start.start:end.end])
			return
		}
		out.WriteString(input[start.start:start.end])
		for i := startIndex + 1; i < start.match; {
			child := tokens[i]
			switch child.kind {
			case tokenCharData:
				if !isAllWhitespace(input[child.start:child.end]) {
					out.WriteString(input[child.start:child.end])
				}
				i++
			case tokenStartElement:
				emitElement(i)
				i = child.match + 1
			default:
				out.WriteString(input[child.start:child.end])
				i++
			}
		}
		out.WriteString(input[end.start:end.end])
	}
	for i := 0; i < len(tokens); {
		token := tokens[i]
		if token.kind == tokenStartElement {
			emitElement(i)
			i = token.match + 1
			continue
		}
		out.WriteString(input[token.start:token.end])
		i++
	}
	return out.String()
}

// TestPrettyPrintXMLChartDemoFixture formats the real chart demo both as
// shipped (pretty-printed) and minified to the single-line shape the server
// returns; both must converge on the same idempotent output.
func TestPrettyPrintXMLChartDemoFixture(t *testing.T) {
	original := loadChartDemo(t)

	formattedOriginal, err := prettyPrintXML(original)
	if err != nil {
		t.Fatalf("prettyPrintXML(original): %v", err)
	}
	twice, err := prettyPrintXML(formattedOriginal)
	if err != nil {
		t.Fatalf("prettyPrintXML(second pass): %v", err)
	}
	if twice != formattedOriginal {
		t.Fatal("prettyPrintXML is not idempotent on the chart demo fixture")
	}

	minified := minifyXML(t, original)
	if strings.Contains(minified, ">\n  <") {
		t.Fatalf("minified fixture still contains structural indentation: %q", minified[:200])
	}
	// Only the doc-level newline after the XML declaration and the trailing
	// newline may remain; the whole element tree must be one line.
	if got := strings.Count(minified, "\n"); got > 2 {
		t.Fatalf("minified fixture has %d newlines, want <= 2", got)
	}
	formattedMinified, err := prettyPrintXML(minified)
	if err != nil {
		t.Fatalf("prettyPrintXML(minified): %v", err)
	}
	// Formatting drops exactly the whitespace minification dropped, so both
	// paths must converge on the same output.
	if formattedMinified != formattedOriginal {
		t.Fatal("format(minified) != format(original) for the chart demo fixture")
	}
	if !strings.Contains(formattedMinified, "\n  <slide>") {
		t.Fatal("formatted chart demo lacks expected slide indentation")
	}
}

func BenchmarkPrettyPrintXMLChartDemoMinified(b *testing.B) {
	minified := minifyXML(b, loadChartDemo(b))
	b.SetBytes(int64(len(minified)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := prettyPrintXML(minified); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrettyPrintXMLChartDemoPreformatted(b *testing.B) {
	original := loadChartDemo(b)
	b.SetBytes(int64(len(original)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := prettyPrintXML(original); err != nil {
			b.Fatal(err)
		}
	}
}
