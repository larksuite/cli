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
	if string(got) != xml {
		t.Fatalf("saved XML = %q, want %q", got, xml)
	}
	if strings.Contains(stdout.String(), xml) {
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
	if data["size"] != float64(len(xml)) {
		t.Fatalf("size = %v, want %d", data["size"], len(xml))
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
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	presentation := data["xml_presentation"].(map[string]interface{})
	if got := presentation["content"]; got != xml {
		t.Fatalf("content = %q, want %q", got, xml)
	}
	if got := data["xml_presentation_id"]; got != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc", got)
	}
	if strings.Contains(stdout.String(), "content_saved") {
		t.Fatalf("stdout should not contain file metadata: %s", stdout.String())
	}
}

func TestSlidesXMLGetJqFiltersContentEnvelopeWhenOutputOmitted(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

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
		t.Fatalf("stdout = %q, want XML content %q", got, xml)
	}
}

func TestSlidesXMLGetPrintsRawContentWhenRaw(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

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
		"--raw",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := stdout.String(); got != xml {
		t.Fatalf("stdout = %q, want raw XML %q", got, xml)
	}
}

func TestSlidesXMLGetFetchesSingleSlideByIDToFile(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	xml := `<slide id="slide_1"><data><shape id="a"/></data></slide>`
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
	if string(got) != xml {
		t.Fatalf("saved XML = %q, want %q", got, xml)
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
		t.Fatalf("content = %q, want %q", slide["content"], xml)
	}
	if slide["slide_id"] != "slide_2" {
		t.Fatalf("slide.slide_id = %v, want slide_2", slide["slide_id"])
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
