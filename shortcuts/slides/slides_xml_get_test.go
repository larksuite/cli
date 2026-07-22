// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestSlidesXMLGetWritesContentToFileAndSuppressesXML(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	xml := `<presentation><slide id="s1"><shape id="a">hello</shape></slide></presentation>`
	// Golden value computed independently of prettyPrintXML (not derived by
	// calling it): a bug in prettyPrintXML itself must not be able to make
	// this assertion pass by construction.
	wantXML := "<presentation>\n  <slide id=\"s1\">\n    <shape id=\"a\">hello</shape>\n  </slide>\n</presentation>\n"
	var capturedQuery url.Values
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"presentation_id": "pres_abc",
					"revision_id":     7,
					"content":         xml,
				},
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--output", "readback.xml",
		"--revision-id", "7",
		"--remove-attr-id",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "readback.xml")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved XML: %v", err)
	}
	if string(got) != wantXML {
		t.Fatalf("saved XML = %q, want %q", got, wantXML)
	}
	if strings.Contains(stdout.String(), wantXML) {
		t.Fatalf("stdout leaked full XML content: %s", stdout.String())
	}
	if got := capturedQuery.Get("revision_id"); got != "7" {
		t.Fatalf("revision_id query = %q, want 7", got)
	}
	if got := capturedQuery.Get("remove_attr_id"); got != "true" {
		t.Fatalf("remove_attr_id query = %q, want true", got)
	}

	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc", data["xml_presentation_id"])
	}
	if data["revision_id"] != float64(7) {
		t.Fatalf("revision_id = %v, want 7", data["revision_id"])
	}
	if data["pretty_printed"] != true {
		t.Fatalf("pretty_printed = %v, want true", data["pretty_printed"])
	}
	if data["size"] != float64(len(wantXML)) {
		t.Fatalf("size = %v, want %d", data["size"], len(wantXML))
	}
	gotPath, _ := data["path"].(string)
	if !filepath.IsAbs(gotPath) {
		t.Fatalf("path = %v, want absolute path", gotPath)
	}
	if !strings.HasSuffix(gotPath, "readback.xml") {
		t.Fatalf("path = %v, want readback.xml suffix", gotPath)
	}
}

func TestSlidesXMLGetReturnsContentEnvelopeWhenOutputOmitted(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	// The JSON envelope carries the server content verbatim: no reindentation
	// and no parse/reserialize cycle. Reintroducing the in-repo formatter
	// would fail this by inserting indentation; the &#32; reference
	// additionally guards against a plain unmasked XML round trip, which
	// would decode it to a literal space.
	xml := `<presentation><slide id="s1"><shape id="a"><content><p><span>Hello</span>&#32;<strong>World</strong></p></content></shape></slide></presentation>`
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": xml,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	presentation := data["xml_presentation"].(map[string]interface{})
	if got := presentation["content"]; got != xml {
		t.Fatalf("content = %q, want the server content verbatim %q", got, xml)
	}
	if got := data["xml_presentation_id"]; got != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc", got)
	}
	if _, ok := data["pretty_printed"]; ok {
		t.Fatalf("pretty_printed should not appear in the envelope: %#v", data)
	}
	if strings.Contains(stdout.String(), "content_saved") {
		t.Fatalf("stdout should not contain file metadata: %s", stdout.String())
	}
}

func TestSlidesXMLGetJqFiltersContentEnvelopeWhenOutputOmitted(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	// --jq extracts fields from the envelope, and the envelope carries the
	// server content verbatim, so the filter yields the single-line original.
	xml := `<presentation><slide id="s1"><shape id="a">hello</shape></slide></presentation>`
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": xml,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--jq", ".data.xml_presentation.content",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != xml {
		t.Fatalf("stdout = %q, want the server content verbatim %q", got, xml)
	}
}

func TestSlidesXMLGetPrintsFormattedContentWithoutEnvelopeWhenRaw(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	xml := `<presentation><slide id="s1"><shape id="a">hello</shape></slide></presentation>`
	// Golden value computed independently of prettyPrintXML; see the comment
	// in TestSlidesXMLGetWritesContentToFileAndSuppressesXML.
	wantXML := "<presentation>\n  <slide id=\"s1\">\n    <shape id=\"a\">hello</shape>\n  </slide>\n</presentation>\n"
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": xml,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--raw",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := stdout.String(); got != wantXML {
		t.Fatalf("stdout = %q, want formatted XML %q", got, wantXML)
	}
}

func TestSlidesXMLGetRawFlagDocumentsFormattedOutput(t *testing.T) {
	for _, flag := range SlidesXMLGet.Flags {
		if flag.Name != "raw" {
			continue
		}
		if !strings.Contains(flag.Desc, "formatted XML") || strings.Contains(flag.Desc, "raw XML") {
			t.Fatalf("--raw description = %q, want formatted XML without a raw-payload claim", flag.Desc)
		}
		return
	}
	t.Fatal("--raw flag not found")
}

func TestSlidesXMLGetFetchesSingleSlideByIDToFile(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	xml := `<slide id="slide_1"><data><shape id="a"/></data></slide>`
	// Golden value computed independently of prettyPrintXML; see the comment
	// in TestSlidesXMLGetWritesContentToFileAndSuppressesXML.
	wantXML := "<slide id=\"slide_1\">\n  <data>\n    <shape id=\"a\"/>\n  </data>\n</slide>\n"
	var capturedQuery url.Values
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide": map[string]interface{}{
					"slide_id": "slide_1",
					"content":  xml,
				},
				"revision_id": 8,
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1",
		"--output", "slide_1.xml",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := capturedQuery.Get("slide_id"); got != "slide_1" {
		t.Fatalf("slide_id query = %q, want slide_1", got)
	}
	if got := capturedQuery.Get("revision_id"); got != "-1" {
		t.Fatalf("revision_id query = %q, want -1", got)
	}
	path := filepath.Join(dir, "slide_1.xml")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved slide XML: %v", err)
	}
	if string(got) != wantXML {
		t.Fatalf("saved XML = %q, want %q", got, wantXML)
	}
	data := decodeShortcutData(t, stdout)
	if data["scope"] != "slide" {
		t.Fatalf("scope = %v, want slide", data["scope"])
	}
	if data["slide_id"] != "slide_1" {
		t.Fatalf("slide_id = %v, want slide_1", data["slide_id"])
	}
	if data["content_saved"] != true {
		t.Fatalf("content_saved = %v, want true", data["content_saved"])
	}
}

func TestSlidesXMLGetFetchesSingleSlideByNumberEnvelope(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	// The slide envelope carries the server content verbatim, like the
	// presentation envelope.
	xml := `<slide id="slide_2"><data><shape id="b"/></data></slide>`
	var capturedQuery url.Values
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide": map[string]interface{}{
					"slide_id": "slide_2",
					"content":  xml,
				},
				"revision_id": 9,
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.Query()
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-number", "2",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := capturedQuery.Get("slide_number"); got != "2" {
		t.Fatalf("slide_number query = %q, want 2", got)
	}
	data := decodeShortcutData(t, stdout)
	if data["scope"] != "slide" {
		t.Fatalf("scope = %v, want slide", data["scope"])
	}
	if data["slide_number"] != float64(2) {
		t.Fatalf("slide_number = %v, want 2", data["slide_number"])
	}
	slide := data["slide"].(map[string]interface{})
	if slide["content"] != xml {
		t.Fatalf("content = %q, want the server content verbatim %q", slide["content"], xml)
	}
	if slide["slide_id"] != "slide_2" {
		t.Fatalf("slide.slide_id = %v, want slide_2", slide["slide_id"])
	}
	if _, ok := data["pretty_printed"]; ok {
		t.Fatalf("pretty_printed should not appear in the envelope: %#v", data)
	}
}

func TestSlidesXMLGetResolvesWikiPresentation(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "pres_real",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_real",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": `<presentation/>`,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "https://example.feishu.cn/wiki/wikcn123",
		"--output", "wiki.xml",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_real" {
		t.Fatalf("xml_presentation_id = %v, want pres_real", data["xml_presentation_id"])
	}
}

func TestSlidesXMLGetRejectsUnsafeOutputPath(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--output", "../readback.xml",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected unsafe output path error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != "--output" {
		t.Fatalf("param = %q, want --output", validationErr.Param)
	}
}

func TestSlidesXMLGetRejectsRevisionIDBelowMinusOneBeforeAPICall(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry-run=%t", dryRun), func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

			args := []string{
				"+xml-get",
				"--presentation", "pres_abc",
				"--revision-id", "-2",
				"--as", "user",
			}
			if dryRun {
				args = append(args, "--dry-run")
			}
			err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, args)
			if err == nil {
				t.Fatal("expected invalid revision-id error, got nil")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T %v", err, err)
			}
			if problem.Category != errs.CategoryValidation {
				t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
			}
			if problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
			}
			if validationErr.Param != "--revision-id" {
				t.Fatalf("param = %q, want --revision-id", validationErr.Param)
			}
		})
	}
}

func TestSlidesXMLGetRejectsConflictingSlideSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1",
		"--slide-number", "1",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected selector conflict error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != "--slide-id" {
		t.Fatalf("param = %q, want --slide-id", validationErr.Param)
	}
}

func TestSlidesXMLGetRejectsEmptySlideID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-id", " ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected empty slide-id error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != "--slide-id" {
		t.Fatalf("param = %q, want --slide-id", validationErr.Param)
	}
}

func TestSlidesXMLGetRejectsRemoveAttrIDForSingleSlide(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--remove-attr-id",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected remove-attr-id validation error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != "--remove-attr-id" {
		t.Fatalf("param = %q, want --remove-attr-id", validationErr.Param)
	}
}

func TestPrettyPrintXML(t *testing.T) {
	input := `<presentation id="p1" xmlns="http://www.larkoffice.com/sml/2.0" width="960"><slide id="s1"><style><fill id="f1"><fillColor color="rgba(0,0,0,1)"/></fill></style><data/></slide></presentation>`

	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected reindented output with newlines, got %q", got)
	}
	if n := strings.Count(got, `xmlns="http://www.larkoffice.com/sml/2.0"`); n != 1 {
		t.Fatalf("expected the xmlns declaration to appear exactly once, got %d occurrences in %q", n, got)
	}
	if !strings.Contains(got, "<data/>") {
		t.Fatalf("expected empty <data/> to stay self-closing, got %q", got)
	}
	if !strings.Contains(got, `<fillColor color="rgba(0,0,0,1)"/>`) {
		t.Fatalf("expected attributes to be preserved on their element, got %q", got)
	}
}

func TestPrettyPrintXMLRejectsMalformedInput(t *testing.T) {
	if _, err := prettyPrintXML(`<presentation><slide></presentation>`); err == nil {
		t.Fatal("expected an error for malformed XML, got nil")
	}
}

// TestPrettyPrintXMLPreservesEscapedWhitespaceReferences covers the schema's
// documented space/tab escape idiom (slides_xml_schema_definition.xml, <p>
// element docs) and CR/LF references whose lexical form is needed to avoid
// XML line-ending normalization on a later parse. etree normally decodes the
// references into literal whitespace. The formatter must preserve their
// lexical representation for safe read-modify-write workflows.
func TestPrettyPrintXMLPreservesEscapedWhitespaceReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"space in p", `<content><p>&#32;</p></content>`, "<content>\n  <p>&#32;</p>\n</content>\n"},
		{"tab in p", `<content><p>&#9;</p></content>`, "<content>\n  <p>&#9;</p>\n</content>\n"},
		{"space in nested span", `<content><p><span>&#32;</span></p></content>`, "<content>\n  <p><span>&#32;</span></p>\n</content>\n"},
		{"hex space", `<content><p>&#x20;</p></content>`, "<content>\n  <p>&#x20;</p>\n</content>\n"},
		{"zero-padded tab", `<content><p>&#0009;</p></content>`, "<content>\n  <p>&#0009;</p>\n</content>\n"},
		{"carriage return", `<content><p>A&#13;B</p></content>`, "<content>\n  <p>A&#13;B</p>\n</content>\n"},
		{"line feed", `<content><p>A&#10;B</p></content>`, "<content>\n  <p>A&#10;B</p>\n</content>\n"},
		{"hex carriage return", `<content><p>A&#xD;B</p></content>`, "<content>\n  <p>A&#xD;B</p>\n</content>\n"},
		{"hex line feed", `<content><p>A&#xA;B</p></content>`, "<content>\n  <p>A&#xA;B</p>\n</content>\n"},
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

func TestPrettyPrintXMLPreservesTextOnlyLeafWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "title literal space",
			input: `<presentation><title> </title><slide/></presentation>`,
			want:  "<presentation>\n  <title> </title>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "title escaped space",
			input: `<presentation><title>&#32;</title><slide/></presentation>`,
			want:  "<presentation>\n  <title>&#32;</title>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "title whitespace CDATA",
			input: `<presentation><title><![CDATA[   ]]></title><slide/></presentation>`,
			want:  "<presentation>\n  <title><![CDATA[   ]]></title>\n  <slide/>\n</presentation>\n",
		},
		{
			name:  "chart field literal space",
			input: `<chartData><chartField name="x"> </chartField></chartData>`,
			want:  "<chartData>\n  <chartField name=\"x\"> </chartField>\n</chartData>\n",
		},
		{
			name:  "title adjacent text and CDATA",
			input: `<presentation><title> <![CDATA[ ]]></title><slide/></presentation>`,
			want:  "<presentation>\n  <title> <![CDATA[ ]]></title>\n  <slide/>\n</presentation>\n",
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

// TestPrettyPrintXMLPreservesEscapedSpaceBetweenInlineSiblings is the
// critical case: &#32; sitting as a bare sibling text node directly between
// two inline elements, not wrapped in its own tag -- the literal reading of
// the schema's "标签之间...请使用&#32;" guidance, e.g. a plain-styled space
// between two differently formatted words at a pptx run boundary. A fix
// that only special-cases "element whose sole content is whitespace" (such
// as etree's PreserveLeafWhitespace) does not cover this: the whitespace
// here is one of several children of <p>, not the sole child of <span>.
func TestPrettyPrintXMLPreservesEscapedSpaceBetweenInlineSiblings(t *testing.T) {
	input := `<content><p><span>Hello</span>&#32;<strong>World</strong></p></content>`
	want := "<content>\n  <p><span>Hello</span>&#32;<strong>World</strong></p>\n</content>\n"
	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if got != want {
		t.Fatalf("prettyPrintXML(%q) = %q, want %q", input, got, want)
	}
}

func TestPrettyPrintXMLPreservesCDATA(t *testing.T) {
	input := `<content><p><![CDATA[a-->b & <c>]]></p></content>`
	want := "<content>\n  <p><![CDATA[a-->b & <c>]]></p>\n</content>\n"
	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if got != want {
		t.Fatalf("prettyPrintXML(%q) = %q, want %q", input, got, want)
	}
}

// TestPrettyPrintXMLSeparatesParagraphsWithoutTouchingTheirText is the
// feature's actual point: a shape with many paragraphs becomes navigable
// (each <p> on its own indented line), while every paragraph's own rich
// text -- including an inline formatting boundary -- stays byte-for-byte
// unchanged.
func TestPrettyPrintXMLSeparatesParagraphsWithoutTouchingTheirText(t *testing.T) {
	input := `<content><p>First paragraph.</p><p>Second <strong>paragraph</strong>.</p></content>`
	want := "<content>\n  <p>First paragraph.</p>\n  <p>Second <strong>paragraph</strong>.</p>\n</content>\n"
	got, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML: %v", err)
	}
	if got != want {
		t.Fatalf("prettyPrintXML(%q) = %q, want %q", input, got, want)
	}
}

func TestPrettyPrintXMLIdempotent(t *testing.T) {
	input := `<presentation><slide id="s1"><shape id="a"><content><p>A&#32;&#32;B&#9;C&#13;D&#10;E</p></content><style/></shape></slide></presentation>`
	once, err := prettyPrintXML(input)
	if err != nil {
		t.Fatalf("prettyPrintXML (first pass): %v", err)
	}
	twice, err := prettyPrintXML(once)
	if err != nil {
		t.Fatalf("prettyPrintXML (second pass): %v", err)
	}
	if once != twice {
		t.Fatalf("not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

func TestSlidesXMLGetFallsBackToOriginalPresentationWhenReformatFails(t *testing.T) {
	content := "<presentation><title>\x0b</title><slide/></presentation>"
	f, stdout, stderr, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": content,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--raw",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := stdout.String(); got != content {
		t.Fatalf("stdout = %q, want original content %q", got, content)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: XML pretty-print skipped; returning original server content:") {
		t.Fatalf("stderr = %q, want explicit pretty-print fallback warning", got)
	}
}

// TestSlidesXMLGetEnvelopePassesThroughMalformedSlideContent pins the
// envelope contract: the content is never parsed, so even malformed XML
// flows through byte for byte with no fallback warning and no
// pretty_printed field.
func TestSlidesXMLGetEnvelopePassesThroughMalformedSlideContent(t *testing.T) {
	content := `<slide><data></slide>`
	f, stdout, stderr, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide": map[string]interface{}{
					"slide_id": "slide_1",
					"content":  content,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	slide, _ := data["slide"].(map[string]interface{})
	if slide == nil {
		t.Fatalf("missing slide: %#v", data)
	}
	if got, _ := slide["content"].(string); got != content {
		t.Fatalf("slide.content = %q, want the server content verbatim %q", got, content)
	}
	if _, ok := data["pretty_printed"]; ok {
		t.Fatalf("pretty_printed should not appear in the envelope: %#v", data)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty: the envelope path must not parse the content", got)
	}
}

// TestSlidesXMLGetEnvelopePassesThroughMalformedPresentationContent mirrors
// the slide-scope passthrough test for the presentation-scope fetch branch,
// which is a separate code path.
func TestSlidesXMLGetEnvelopePassesThroughMalformedPresentationContent(t *testing.T) {
	content := `<presentation><slide></presentation>`
	f, stdout, stderr, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": content,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	presentation, _ := data["xml_presentation"].(map[string]interface{})
	if presentation == nil {
		t.Fatalf("missing xml_presentation: %#v", data)
	}
	if got, _ := presentation["content"].(string); got != content {
		t.Fatalf("content = %q, want the server content verbatim %q", got, content)
	}
	if _, ok := data["pretty_printed"]; ok {
		t.Fatalf("pretty_printed should not appear in the envelope: %#v", data)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty: the envelope path must not parse the content", got)
	}
}

func TestSlidesXMLGetFileMetadataReportsPrettyPrintFallback(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	content := `<presentation><slide></presentation>`
	f, stdout, stderr, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation": map[string]interface{}{
					"content": content,
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--output", "fallback.xml",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fallback.xml"))
	if err != nil {
		t.Fatalf("read fallback XML: %v", err)
	}
	if string(got) != content {
		t.Fatalf("saved XML = %q, want original content %q", got, content)
	}
	data := decodeShortcutData(t, stdout)
	if data["pretty_printed"] != false {
		t.Fatalf("pretty_printed = %v, want false", data["pretty_printed"])
	}
	if got := stderr.String(); !strings.Contains(got, "warning: XML pretty-print skipped; returning original server content:") {
		t.Fatalf("stderr = %q, want explicit pretty-print fallback warning", got)
	}
}
