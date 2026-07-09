// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"errors"
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
