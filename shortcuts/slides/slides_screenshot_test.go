// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestSlidesScreenshotDeclaredScopes(t *testing.T) {
	base := []string{"slides:presentation:screenshot"}
	if got := SlidesScreenshot.ScopesForIdentity("user"); !reflect.DeepEqual(got, base) {
		t.Fatalf("user preflight scopes = %#v, want %#v", got, base)
	}
	if got := SlidesScreenshot.ScopesForIdentity("bot"); !reflect.DeepEqual(got, base) {
		t.Fatalf("bot preflight scopes = %#v, want %#v", got, base)
	}

	got := SlidesScreenshot.DeclaredScopesForIdentity("user")
	want := []string{"slides:presentation:screenshot", "wiki:node:read"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declared scopes = %#v, want %#v", got, want)
	}
}

func TestSlidesScreenshotWritesFilesAndSuppressesBase64(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	imageBytes := []byte("png-bytes")
	jpegBytes := []byte("jpeg-bytes")
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_id": "slide_1",
						"format":   1,
						"data":     base64.StdEncoding.EncodeToString(imageBytes),
					},
					{
						"slide_id":     "slide_2",
						"slide_number": 2,
						"format":       2,
						"data":         base64.StdEncoding.EncodeToString(jpegBytes),
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1",
		"--output-dir", "shots",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "shots", "pres_abc_slide_1.png")
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read screenshot: %v", err)
	}
	if string(gotBytes) != string(imageBytes) {
		t.Fatalf("written bytes = %q, want %q", gotBytes, imageBytes)
	}
	jpegPath := filepath.Join(dir, "shots", "pres_abc_p002_slide_2.jpg")
	gotJPEGBytes, err := os.ReadFile(jpegPath)
	if err != nil {
		t.Fatalf("read jpeg screenshot: %v", err)
	}
	if string(gotJPEGBytes) != string(jpegBytes) {
		t.Fatalf("written jpeg bytes = %q, want %q", gotJPEGBytes, jpegBytes)
	}
	if strings.Contains(stdout.String(), base64.StdEncoding.EncodeToString(imageBytes)) {
		t.Fatalf("stdout leaked base64 image data: %s", stdout.String())
	}

	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v", data["xml_presentation_id"])
	}
	items, ok := data["screenshots"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("screenshots = %#v, want two items", data["screenshots"])
	}
	item, _ := items[0].(map[string]interface{})
	if item["slide_id"] != "slide_1" {
		t.Fatalf("slide_id = %v, want slide_1", item["slide_id"])
	}
	gotPath := item["path"].(string)
	if !filepath.IsAbs(gotPath) {
		t.Fatalf("path = %v, want absolute path", gotPath)
	}
	if !strings.HasSuffix(gotPath, filepath.Join("shots", "pres_abc_slide_1.png")) {
		t.Fatalf("path = %v, want shots/pres_abc_slide_1.png suffix", item["path"])
	}
	item2, _ := items[1].(map[string]interface{})
	if item2["format"] != "jpeg" {
		t.Fatalf("format = %v, want jpeg", item2["format"])
	}
	gotPath2 := item2["path"].(string)
	if !filepath.IsAbs(gotPath2) {
		t.Fatalf("path = %v, want absolute path", gotPath2)
	}
	if !strings.HasSuffix(gotPath2, filepath.Join("shots", "pres_abc_p002_slide_2.jpg")) {
		t.Fatalf("path = %v, want shots/pres_abc_p002_slide_2.jpg suffix", item2["path"])
	}

	var body struct {
		SlideIDs []string `json:"slide_ids"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.SlideIDs) != 1 || body.SlideIDs[0] != "slide_1" {
		t.Fatalf("slide_ids = %#v, want [slide_1]", body.SlideIDs)
	}
}

func TestSlidesScreenshotListBySlideNumber(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_number": 2,
						"format":       1,
						"data":         base64.StdEncoding.EncodeToString([]byte("png-bytes")),
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "2",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		SlideNumbers []int `json:"slide_numbers"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.SlideNumbers) != 1 || body.SlideNumbers[0] != 2 {
		t.Fatalf("slide_numbers = %#v, want [2]", body.SlideNumbers)
	}
	path := filepath.Join(dir, defaultSlidesScreenshotDir, "pres_abc_p002.png")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("read screenshot without slide_id: %v", err)
	}
}

func TestSlidesScreenshotListBySlideIDCSV(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_id": "slide_1",
						"format":   1,
						"data":     base64.StdEncoding.EncodeToString([]byte("png-bytes-1")),
					},
					{
						"slide_id": "slide_2",
						"format":   1,
						"data":     base64.StdEncoding.EncodeToString([]byte("png-bytes-2")),
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1,slide_2",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		SlideIDs []string `json:"slide_ids"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.SlideIDs) != 2 || body.SlideIDs[0] != "slide_1" || body.SlideIDs[1] != "slide_2" {
		t.Fatalf("slide_ids = %#v, want [slide_1 slide_2]", body.SlideIDs)
	}

	path1 := filepath.Join(dir, defaultSlidesScreenshotDir, "pres_abc_slide_1.png")
	if _, err := os.ReadFile(path1); err != nil {
		t.Fatalf("read first CSV slide screenshot: %v", err)
	}
	path2 := filepath.Join(dir, defaultSlidesScreenshotDir, "pres_abc_slide_2.png")
	if _, err := os.ReadFile(path2); err != nil {
		t.Fatalf("read second CSV slide screenshot: %v", err)
	}
}

func TestSlidesScreenshotListBySlideIDCSVDeduplicatesAndTrims(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_id": "slide_1",
						"format":   1,
						"data":     base64.StdEncoding.EncodeToString([]byte("png-bytes-1")),
					},
					{
						"slide_id": "slide_2",
						"format":   1,
						"data":     base64.StdEncoding.EncodeToString([]byte("png-bytes-2")),
					},
				},
			},
		},
	}
	reg.Register(stub)

	// CSV with a duplicate and blank segments should normalize the same way
	// normalizeSlideIDs already does for repeated --slide-id flags.
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1, slide_2,slide_1,",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		SlideIDs []string `json:"slide_ids"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.SlideIDs) != 2 || body.SlideIDs[0] != "slide_1" || body.SlideIDs[1] != "slide_2" {
		t.Fatalf("slide_ids = %#v, want deduplicated [slide_1 slide_2]", body.SlideIDs)
	}
}

func TestSlidesScreenshotListRejectsMoreThanTenSlideIDsCSV(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "s1,s2,s3,s4,s5,s6,s7,s8,s9,s10,s11",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %v, want typed validation error", err)
	}
	if problem.Hint != "request at most 10 pages at a time" {
		t.Fatalf("hint = %q, want max 10 pages guidance", problem.Hint)
	}
}

func TestSlidesScreenshotAvoidsOverwritingExistingFile(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	outputDir := filepath.Join(dir, "shots")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	existingPath := filepath.Join(outputDir, "pres_abc_p002.png")
	if err := os.WriteFile(existingPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing screenshot: %v", err)
	}

	imageBytes := []byte("new-png")
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_number": 2,
						"format":       1,
						"data":         base64.StdEncoding.EncodeToString(imageBytes),
					},
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "2",
		"--output-dir", "shots",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotExisting, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("read existing screenshot: %v", err)
	}
	if string(gotExisting) != "existing" {
		t.Fatalf("existing screenshot = %q, want unchanged", gotExisting)
	}
	newPath := filepath.Join(outputDir, "pres_abc_p002_2.png")
	gotNew, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read deduplicated screenshot: %v", err)
	}
	if string(gotNew) != string(imageBytes) {
		t.Fatalf("deduplicated screenshot = %q, want %q", gotNew, imageBytes)
	}
	data := decodeShortcutData(t, stdout)
	items, ok := data["screenshots"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("screenshots = %#v, want one item", data["screenshots"])
	}
	item, _ := items[0].(map[string]interface{})
	if !strings.HasSuffix(item["path"].(string), filepath.Join("shots", "pres_abc_p002_2.png")) {
		t.Fatalf("path = %v, want shots/pres_abc_p002_2.png suffix", item["path"])
	}
}

func TestSlidesScreenshotListRequiresSelector(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--slide-id or --slide-number is required") {
		t.Fatalf("error = %v, want missing selector error", err)
	}
}

func TestSlidesScreenshotListRejectsMoreThanTenSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--slide-number", "2",
		"--slide-number", "3",
		"--slide-number", "4",
		"--slide-number", "5",
		"--slide-number", "6",
		"--slide-number", "7",
		"--slide-number", "8",
		"--slide-number", "9",
		"--slide-number", "10",
		"--slide-number", "11",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %v, want typed validation error", err)
	}
	if problem.Hint != "request at most 10 pages at a time" {
		t.Fatalf("hint = %q, want max 10 pages guidance", problem.Hint)
	}
}

func TestSlidesScreenshotRenderContentWritesFile(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	content := `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`
	if err := os.WriteFile(filepath.Join(dir, "slide.xml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write input xml: %v", err)
	}
	imageBytes := []byte("rendered-png")
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/slide_image/render",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_image": map[string]interface{}{
					"slide_id":     "render_slide",
					"slide_number": 1,
					"format":       1,
					"data":         base64.StdEncoding.EncodeToString(imageBytes),
				},
			},
		},
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", "@slide.xml",
		"--output-dir", "shots",
		"--output-name", "preview",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "shots", "preview.png")
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered screenshot: %v", err)
	}
	if string(gotBytes) != string(imageBytes) {
		t.Fatalf("written bytes = %q, want %q", gotBytes, imageBytes)
	}
	if strings.Contains(stdout.String(), base64.StdEncoding.EncodeToString(imageBytes)) {
		t.Fatalf("stdout leaked base64 image data: %s", stdout.String())
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body.Content != content {
		t.Fatalf("content = %q, want input XML", body.Content)
	}

	data := decodeShortcutData(t, stdout)
	items, ok := data["screenshots"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("screenshots = %#v, want one item", data["screenshots"])
	}
	item, _ := items[0].(map[string]interface{})
	if !strings.HasSuffix(item["path"].(string), filepath.Join("shots", "preview.png")) {
		t.Fatalf("path = %v, want shots/preview.png suffix", item["path"])
	}
}

func TestSlidesScreenshotRenderRejectsSlideSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide-id", "slide_1",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--content cannot be used with --slide-id or --slide-number") {
		t.Fatalf("error = %v, want content/slide selector conflict", err)
	}
}

func TestSlidesScreenshotRenderRejectsSlideNumberSelector(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	// Exercises the --slide-number-only side of the --content conflict check
	// (TestSlidesScreenshotRenderRejectsSlideSelectors above only covers the
	// --slide-id side of that same `||` condition).
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide-number", "1",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--content cannot be used with --slide-id or --slide-number") {
		t.Fatalf("error = %v, want content/slide selector conflict", err)
	}
}

func TestSlidesScreenshotRenderRejectsListOnlyFlags(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--presentation", "pres_abc",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--presentation cannot be used with --content") {
		t.Fatalf("error = %v, want presentation/content conflict", err)
	}
}

func TestSlidesScreenshotDryRunSelectsListOrRenderAPI(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
			"+screenshot",
			"--presentation", "pres_abc",
			"--slide-number", "2",
			"--dry-run",
			"--as", "user",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "/xml_presentations/pres_abc/slide_images") {
			t.Fatalf("dry-run missing list endpoint: %s", out)
		}
		if !strings.Contains(out, "slide_numbers") {
			t.Fatalf("dry-run missing slide_numbers body: %s", out)
		}
	})

	t.Run("render", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
		err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
			"+screenshot",
			"--content", `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`,
			"--dry-run",
			"--as", "user",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "/slide_image/render") {
			t.Fatalf("dry-run missing render endpoint: %s", out)
		}
		if !strings.Contains(out, "base64_output") {
			t.Fatalf("dry-run missing base64 suppression note: %s", out)
		}
	})
}

func TestSlidesScreenshotRejectsBadOutputDir(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "slide_1",
		"--output-dir", "../outside",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for unsafe output dir")
	}
	if !strings.Contains(err.Error(), "--output-dir invalid") {
		t.Fatalf("error = %v, want output-dir validation", err)
	}
}

func TestSlidesScreenshotNoImagesErrorIncludesRawDataAndLogID(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Tt-Logid":   {"log-123"},
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"unexpected": "shape",
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "pJJ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error type = %T, want typed problem", err)
	}
	if p.LogID != "log-123" {
		t.Fatalf("log_id = %v, want log-123", p.LogID)
	}
	if !strings.Contains(p.Message, "unexpected:shape") {
		t.Fatalf("message = %q, want raw_data summary", p.Message)
	}
}

func TestSlidesScreenshotSlideNumberAPIErrorAddsHint(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
			"X-Tt-Logid":   {"log-slide-number"},
		},
		Body: map[string]interface{}{
			"code": 99992402,
			"msg":  "field validation failed",
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "25",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error type = %T, want typed problem", err)
	}
	if p.LogID != "log-slide-number" {
		t.Fatalf("log_id = %v, want log-slide-number", p.LogID)
	}
	if !strings.Contains(p.Hint, "--slide-id") {
		t.Fatalf("hint = %q, want --slide-id guidance", p.Hint)
	}
}
