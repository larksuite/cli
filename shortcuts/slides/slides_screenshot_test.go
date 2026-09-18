// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestSlidesScreenshotCompatibilityAliases(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantURL     string
		wantIDs     []string
		wantNumbers []int
	}{
		{
			name:    "presentation-id",
			args:    []string{"--presentation-id", "pres_alias", "--slide-id", "slide_1"},
			wantURL: "/open-apis/slides_ai/v1/xml_presentations/pres_alias/slide_images",
			wantIDs: []string{"slide_1"},
		},
		{name: "slide-id-aliases-merge", args: []string{"--presentation", "pres_abc", "--slide-ids", "s1", "--slides", "s2"}, wantIDs: []string{"s1", "s2"}},
		{name: "slides", args: []string{"--presentation", "pres_abc", "--slides", "s1,s2"}, wantIDs: []string{"s1", "s2"}},
		{name: "slide-numbers", args: []string{"--presentation", "pres_abc", "--slide-numbers", "1,2"}, wantNumbers: []int{1, 2}},
		{name: "slide-routes-id", args: []string{"--presentation", "pres_abc", "--slide", "pII"}, wantIDs: []string{"pII"}},
		{name: "slide-routes-nonnumeric-id", args: []string{"--presentation", "pres_abc", "--slide", "sld_123"}, wantIDs: []string{"sld_123"}},
		{name: "slide-routes-number", args: []string{"--presentation", "pres_abc", "--slide", "7"}, wantNumbers: []int{7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			args := append([]string{"+screenshot"}, tt.args...)
			args = append(args, "--dry-run", "--as", "user")
			if err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := decodeSlidesScreenshotDryRunRequest(t, stdout)
			wantURL := tt.wantURL
			if wantURL == "" {
				wantURL = "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images"
			}
			if got.URL != wantURL {
				t.Fatalf("url = %q, want %q", got.URL, wantURL)
			}
			if !reflect.DeepEqual(got.Body.SlideIDs, tt.wantIDs) {
				t.Fatalf("slide_ids = %#v, want %#v", got.Body.SlideIDs, tt.wantIDs)
			}
			if !reflect.DeepEqual(got.Body.SlideNumbers, tt.wantNumbers) {
				t.Fatalf("slide_numbers = %#v, want %#v", got.Body.SlideNumbers, tt.wantNumbers)
			}
		})
	}
}

func TestSlidesScreenshotCompatibilityAliasesUseCanonicalFlags(t *testing.T) {
	wantAliases := map[string][]string{
		"presentation": {"presentation-id", "presentation-token", "token", "presentation_id", "xml-presentation-id", "url"},
		"slide-id":     {"slide-ids", "slides"},
		"slide-number": {"slide-numbers"},
	}
	for _, flag := range SlidesScreenshot.Flags {
		want, ok := wantAliases[flag.Name]
		if !ok {
			if flag.Name == "presentation-id" || flag.Name == "slide-ids" || flag.Name == "slides" || flag.Name == "slide-numbers" {
				t.Errorf("--%s registered independently, want a canonical flag alias", flag.Name)
			}
			continue
		}
		if !reflect.DeepEqual(flag.Aliases, want) {
			t.Errorf("--%s aliases = %#v, want %#v", flag.Name, flag.Aliases, want)
		}
		delete(wantAliases, flag.Name)
	}
	if len(wantAliases) != 0 {
		t.Fatalf("missing canonical alias declarations: %#v", wantAliases)
	}
	for _, flag := range SlidesScreenshot.Flags {
		if flag.Name == "slide" && !flag.Hidden {
			t.Fatal("--slide Hidden = false, want true")
		}
	}
}

func TestSlidesScreenshotSameTypeSelectorsMergeAndDeduplicate(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantIDs     []string
		wantNumbers []int
	}{
		{
			name:    "ids",
			args:    []string{"--slide-id", "canonical_id", "--slide-ids", "alias_id", "--slides", "canonical_id,alias_id_2", "--slide", "pII"},
			wantIDs: []string{"canonical_id", "alias_id", "alias_id_2", "pII"},
		},
		{
			name:        "numbers",
			args:        []string{"--slide-number", "8", "--slide-numbers", "9,8", "--slide", "10"},
			wantNumbers: []int{8, 9, 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			args := append([]string{"+screenshot", "--presentation", "pres_abc"}, tt.args...)
			args = append(args, "--dry-run", "--as", "user")
			if err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := decodeSlidesScreenshotDryRunRequest(t, stdout)
			if !reflect.DeepEqual(got.Body.SlideIDs, tt.wantIDs) {
				t.Fatalf("slide_ids = %#v, want %#v", got.Body.SlideIDs, tt.wantIDs)
			}
			if !reflect.DeepEqual(got.Body.SlideNumbers, tt.wantNumbers) {
				t.Fatalf("slide_numbers = %#v, want %#v", got.Body.SlideNumbers, tt.wantNumbers)
			}
		})
	}
}

func TestSlidesScreenshotRejectsMixedSelectorTypes(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-id", "pII",
		"--slide-number", "2",
		"--dry-run",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %v, want typed validation error", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("error = %v, want mixed selector guidance", err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	wantParams := []errs.InvalidParam{
		{Name: "--slide-id", Reason: "selects by slide ID; cannot be combined with slide-number selectors"},
		{Name: "--slide-number", Reason: "selects by slide number; cannot be combined with slide-ID selectors"},
	}
	if !reflect.DeepEqual(validationErr.Params, wantParams) {
		t.Fatalf("params = %#v, want %#v", validationErr.Params, wantParams)
	}
}

func TestSlidesScreenshotAttributesMixedSelectorAliasesToCallerInput(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantParams []errs.InvalidParam
	}{
		{
			name: "numeric slide alias",
			args: []string{"--slides", "pII", "--slide", "2"},
			wantParams: []errs.InvalidParam{
				{Name: "--slides", Reason: "selects by slide ID; cannot be combined with slide-number selectors"},
				{Name: "--slide", Reason: "selects by slide number; cannot be combined with slide-ID selectors"},
			},
		},
		{
			name: "ID slide alias",
			args: []string{"--slide", "pII", "--slide-numbers", "2"},
			wantParams: []errs.InvalidParam{
				{Name: "--slide", Reason: "selects by slide ID; cannot be combined with slide-number selectors"},
				{Name: "--slide-numbers", Reason: "selects by slide number; cannot be combined with slide-ID selectors"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			args := append([]string{"+screenshot", "--presentation", "pres_abc"}, tt.args...)
			args = append(args, "--dry-run", "--as", "user")
			err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, args)
			if err == nil {
				t.Fatal("expected validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if !reflect.DeepEqual(validationErr.Params, tt.wantParams) {
				t.Fatalf("params = %#v, want %#v", validationErr.Params, tt.wantParams)
			}
		})
	}
}

func TestSlidesScreenshotValidatesPresentationBeforeSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "tmp/wiki/invalid",
		"--slide-id", "pII",
		"--slide-number", "2",
		"--dry-run",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "--presentation" {
		t.Fatalf("param = %q, want --presentation", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "unsupported --presentation input") {
		t.Fatalf("error = %v, want presentation validation before selector conflict", err)
	}
}

func TestSlidesScreenshotSlideAliasRejectsInvalidNumbers(t *testing.T) {
	for _, value := range []string{"0", "999999999999999999999999999999"} {
		t.Run(value, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
				"+screenshot",
				"--presentation", "pres_abc",
				"--slide", value,
				"--dry-run",
				"--as", "user",
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %v, want typed validation error", err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != "--slide" {
				t.Fatalf("param = %q, want --slide", validationErr.Param)
			}
		})
	}
}

func decodeSlidesScreenshotDryRunRequest(t *testing.T, stdout *bytes.Buffer) struct {
	URL  string `json:"url"`
	Body struct {
		SlideIDs     []string `json:"slide_ids"`
		SlideNumbers []int    `json:"slide_numbers"`
	} `json:"body"`
} {
	t.Helper()
	var envelope struct {
		Data struct {
			API []struct {
				URL  string `json:"url"`
				Body struct {
					SlideIDs     []string `json:"slide_ids"`
					SlideNumbers []int    `json:"slide_numbers"`
				} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\nraw=%s", err, stdout.String())
	}
	if len(envelope.Data.API) != 1 {
		t.Fatalf("api calls = %d, want 1\nraw=%s", len(envelope.Data.API), stdout.String())
	}
	return envelope.Data.API[0]
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

func TestSlidesScreenshotOutputWritesRequestedSingleListPath(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	imageBytes := []byte("jpeg-bytes")
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
						"format":       2,
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
		"--output", "shots/cover.jpeg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "shots", "cover.jpeg")
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read requested output: %v", err)
	}
	if string(gotBytes) != string(imageBytes) {
		t.Fatalf("written bytes = %q, want %q", gotBytes, imageBytes)
	}

	data := decodeShortcutData(t, stdout)
	items, ok := data["screenshots"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("screenshots = %#v, want one item", data["screenshots"])
	}
	item, _ := items[0].(map[string]interface{})
	gotPath := item["path"].(string)
	if !filepath.IsAbs(gotPath) || !strings.HasSuffix(gotPath, filepath.Join("shots", "cover.jpeg")) {
		t.Fatalf("path = %q, want absolute path ending in shots/cover.jpeg", gotPath)
	}
	if got := data["output"]; got != gotPath {
		t.Fatalf("output = %v, want actual path %q", got, gotPath)
	}
	if _, ok := data["requested_output"]; ok {
		t.Fatalf("requested_output must be omitted when the requested path is unchanged: %#v", data)
	}
	if _, ok := data["output_adjusted"]; ok {
		t.Fatalf("output_adjusted must be omitted when the requested path is unchanged: %#v", data)
	}
	if _, ok := data["output_dir"]; ok {
		t.Fatalf("output_dir must be omitted when --output is used: %#v", data)
	}
}

func TestSlidesScreenshotOutputRejectsMultipleSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1,2",
		"--output", "shots/cover.jpg",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--output" {
		t.Fatalf("error = %#v, want --output validation error", err)
	}
	if !strings.Contains(err.Error(), "exactly one slide") {
		t.Fatalf("error = %v, want single-slide guidance", err)
	}
}

func TestSlidesScreenshotOutputRejectsConflictingNamingFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "output-dir",
			args: []string{"--presentation", "pres_abc", "--slide-number", "1", "--output", "cover.jpg", "--output-dir", "shots"},
		},
		{
			name: "output-name",
			args: []string{"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`, "--output", "cover.png", "--output-name", "preview"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			args := append([]string{"+screenshot"}, tt.args...)
			args = append(args, "--as", "user")
			err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, args)
			if err == nil {
				t.Fatal("expected validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != "--output" {
				t.Fatalf("error = %#v, want --output validation error", err)
			}
			if !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("error = %v, want naming flag conflict", err)
			}
		})
	}
}

func TestSlidesScreenshotOutputRejectsInvalidPath(t *testing.T) {
	tests := []struct {
		name            string
		output          string
		createDirectory bool
		wantDirHint     bool
	}{
		{name: "unsupported extension", output: "shots/cover.gif"},
		{name: "path escapes every allowed root", output: "../../../../../../../../../../../../cover.jpg"},
		{name: "directory suffix", output: "shots/", wantDirHint: true},
		{name: "existing directory", output: "shots", createDirectory: true, wantDirHint: true},
		{name: "leading whitespace", output: " shots/cover.png"},
		{name: "trailing whitespace", output: "shots/cover.png "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			withSlidesTestWorkingDir(t, dir)
			if tt.createDirectory {
				if err := os.MkdirAll(filepath.Join(dir, tt.output), 0o755); err != nil {
					t.Fatalf("create output directory: %v", err)
				}
			}
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
				"+screenshot",
				"--presentation", "pres_abc",
				"--slide-number", "1",
				"--output", tt.output,
				"--as", "user",
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != "--output" {
				t.Fatalf("error = %#v, want --output validation error", err)
			}
			if tt.wantDirHint && !strings.Contains(validationErr.Hint, "--output-dir") {
				t.Fatalf("hint = %q, want --output-dir guidance", validationErr.Hint)
			}
		})
	}
}

func TestSlidesScreenshotOutputAdjustsExtensionToResponseFormat(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_number": 1,
						"format":       2,
						"data":         base64.StdEncoding.EncodeToString([]byte("jpeg-bytes")),
					},
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output", "shots/cover.png",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	actualPath := filepath.Join(dir, "shots", "cover.jpg")
	if got, readErr := os.ReadFile(actualPath); readErr != nil || string(got) != "jpeg-bytes" {
		t.Fatalf("adjusted output = %q, err=%v", got, readErr)
	}
	actualPath = canonicalSlidesScreenshotTestPath(t, actualPath)
	if _, statErr := os.Stat(filepath.Join(dir, "shots", "cover.png")); !os.IsNotExist(statErr) {
		t.Fatalf("requested .png path unexpectedly exists, stat error = %v", statErr)
	}
	data := decodeShortcutData(t, stdout)
	if got := data["requested_output"]; got != "shots/cover.png" {
		t.Fatalf("requested_output = %v, want shots/cover.png", got)
	}
	if got := data["output"]; got != actualPath {
		t.Fatalf("output = %v, want %q", got, actualPath)
	}
	if got := data["output_adjusted"]; got != true {
		t.Fatalf("output_adjusted = %v, want true", got)
	}
}

func TestSlidesScreenshotOutputAppendsResponseExtensionWhenMissing(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{{
					"slide_number": 1,
					"format":       1,
					"data":         base64.StdEncoding.EncodeToString([]byte("png-bytes")),
				}},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output", "shots/cover",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	actualPath := filepath.Join(dir, "shots", "cover.png")
	if got, readErr := os.ReadFile(actualPath); readErr != nil || string(got) != "png-bytes" {
		t.Fatalf("output = %q, err=%v", got, readErr)
	}
	actualPath = canonicalSlidesScreenshotTestPath(t, actualPath)
	data := decodeShortcutData(t, stdout)
	if data["requested_output"] != "shots/cover" || data["output"] != actualPath || data["output_adjusted"] != true {
		t.Fatalf("adjusted output metadata = %#v", data)
	}
}

func TestSlidesScreenshotOutputAvoidsExistingPath(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "shots"), 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	existingPath := filepath.Join(dir, "shots", "cover.jpg")
	if err := os.WriteFile(existingPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{
						"slide_number": 1,
						"format":       2,
						"data":         base64.StdEncoding.EncodeToString([]byte("new-jpeg")),
					},
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output", "shots/cover.jpg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, err := os.ReadFile(existingPath); err != nil || string(got) != "existing" {
		t.Fatalf("existing output = %q, err=%v", got, err)
	}
	actualPath := filepath.Join(dir, "shots", "cover_2.jpg")
	if got, readErr := os.ReadFile(actualPath); readErr != nil || string(got) != "new-jpeg" {
		t.Fatalf("deduplicated output = %q, err=%v", got, readErr)
	}
	actualPath = canonicalSlidesScreenshotTestPath(t, actualPath)
	data := decodeShortcutData(t, stdout)
	if data["requested_output"] != "shots/cover.jpg" || data["output"] != actualPath || data["output_adjusted"] != true {
		t.Fatalf("adjusted output metadata = %#v", data)
	}
}

func TestSlidesScreenshotOutputAdjustsFormatBeforeAvoidingExistingPath(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "shots"), 0o755); err != nil {
		t.Fatalf("create output dir: %v", err)
	}
	existingPath := filepath.Join(dir, "shots", "cover.png")
	if err := os.WriteFile(existingPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{{
					"slide_number": 1,
					"format":       1,
					"data":         base64.StdEncoding.EncodeToString([]byte("new-png")),
				}},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output", "shots/cover.jpg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, err := os.ReadFile(existingPath); err != nil || string(got) != "existing" {
		t.Fatalf("existing output = %q, err=%v", got, err)
	}
	actualPath := filepath.Join(dir, "shots", "cover_2.png")
	if got, readErr := os.ReadFile(actualPath); readErr != nil || string(got) != "new-png" {
		t.Fatalf("deduplicated output = %q, err=%v", got, readErr)
	}
	actualPath = canonicalSlidesScreenshotTestPath(t, actualPath)
	data := decodeShortcutData(t, stdout)
	if data["requested_output"] != "shots/cover.jpg" || data["output"] != actualPath || data["output_adjusted"] != true {
		t.Fatalf("adjusted output metadata = %#v", data)
	}
}

func TestSetSlidesScreenshotResultOutputUsesPreResolvedRequestedPath(t *testing.T) {
	result := map[string]interface{}{}
	target := slidesScreenshotOutputTarget{
		requested:         "shots/cover.png",
		requestedResolved: filepath.Join(string(filepath.Separator), "work", "shots", "cover.png"),
	}
	actualPath := filepath.Join(string(filepath.Separator), "work", "shots", "cover.jpg")
	setSlidesScreenshotResultOutput(result, target, []map[string]interface{}{{"path": actualPath}})

	if result["output"] != actualPath || result["requested_output"] != target.requested || result["output_adjusted"] != true {
		t.Fatalf("result = %#v, want adjustment metadata from pre-resolved requested path", result)
	}
}

func canonicalSlidesScreenshotTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", path, err)
	}
	return canonical
}

func TestSlidesScreenshotOutputNameRejectsListMode(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output-name", "cover",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--output-name" {
		t.Fatalf("error = %#v, want --output-name validation error", err)
	}
	if !strings.Contains(err.Error(), "--output") {
		t.Fatalf("error = %v, want migration guidance", err)
	}
}

func TestSlidesScreenshotOutputRejectsMultipleResponseImagesBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_images": []map[string]interface{}{
					{"slide_number": 1, "format": 2, "data": base64.StdEncoding.EncodeToString([]byte("one"))},
					{"slide_number": 2, "format": 2, "data": base64.StdEncoding.EncodeToString([]byte("two"))},
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--output", "shots/cover.jpg",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected invalid response error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, want api/invalid_response", problem)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "shots", "cover.jpg")); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after multi-image response, stat error = %v", statErr)
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
	tests := []struct {
		name        string
		args        []string
		wantMessage string
		wantHint    string
		wantParam   string
	}{
		{
			name:        "omitted",
			args:        nil,
			wantMessage: "--slide-id or --slide-number is required",
			wantHint:    "specify up to 10 slides with --slide-id <slide_id> or --slide-number <number>; repeat the flag or use comma-separated values for multiple slides",
		},
		{
			name:        "empty slide ID",
			args:        []string{"--slide-id", ""},
			wantMessage: "--slide-id cannot be empty",
			wantHint:    "provide a non-empty slide ID or use --slide-number <number>",
			wantParam:   "--slide-id",
		},
		{
			name:        "empty slide ID with slide number",
			args:        []string{"--slide-id", "", "--slide-number", "1"},
			wantMessage: "--slide-id cannot be empty",
			wantHint:    "provide a non-empty slide ID or use --slide-number <number>",
			wantParam:   "--slide-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			args := append([]string{"+screenshot", "--presentation", "pres_abc"}, tt.args...)
			args = append(args, "--as", "user")

			err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, args)
			if err == nil {
				t.Fatal("expected error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed validation error", err, err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
			}
			if problem.Message != tt.wantMessage {
				t.Fatalf("message = %q, want %q", problem.Message, tt.wantMessage)
			}
			if problem.Hint != tt.wantHint {
				t.Fatalf("hint = %q, want %q", problem.Hint, tt.wantHint)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != tt.wantParam {
				t.Fatalf("param = %q, want %q", validationErr.Param, tt.wantParam)
			}
		})
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

	content := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`
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

func TestSlidesScreenshotOutputWritesRequestedRenderPath(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	imageBytes := []byte("rendered-png")
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/slide_image/render",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_image": map[string]interface{}{
					"format": 1,
					"data":   base64.StdEncoding.EncodeToString(imageBytes),
				},
			},
		},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--output", "shots/preview.jpg",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	actualPath := filepath.Join(dir, "shots", "preview.png")
	if got, err := os.ReadFile(actualPath); err != nil || string(got) != string(imageBytes) {
		t.Fatalf("render output = %q, err=%v", got, err)
	}
	actualPath = canonicalSlidesScreenshotTestPath(t, actualPath)
	data := decodeShortcutData(t, stdout)
	if data["requested_output"] != "shots/preview.jpg" || data["output"] != actualPath || data["output_adjusted"] != true {
		t.Fatalf("adjusted render output metadata = %#v", data)
	}
	if _, ok := data["output_dir"]; ok {
		t.Fatalf("output_dir must be omitted with --output: %#v", data)
	}
}

func TestSlidesScreenshotRenderRejectsSlideSelectors(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide-id", "slide_1",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--content cannot be used with slide selectors") {
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
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide-number", "0",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--content cannot be used with slide selectors") {
		t.Fatalf("error = %v, want content/slide selector conflict", err)
	}
}

func TestSlidesScreenshotRenderAttributesSlideAliasConflictToCallerInput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide", "pII",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	wantParams := []errs.InvalidParam{
		{Name: "--content", Reason: "cannot be combined with slide selectors"},
		{Name: "--slide", Reason: "cannot be combined with --content"},
	}
	if !reflect.DeepEqual(validationErr.Params, wantParams) {
		t.Fatalf("params = %#v, want %#v", validationErr.Params, wantParams)
	}
}

func TestSlidesScreenshotRenderIgnoresEmptySlideID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
		"--slide-id", "",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/slides_ai/v1/slide_image/render") {
		t.Fatalf("dry-run missing render endpoint: %s", stdout.String())
	}
}

func TestSlidesScreenshotRenderRejectsListOnlyFlags(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
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
			"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
			"--output", "shots/preview.png",
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
		if !strings.Contains(out, `"output": "shots/preview.png"`) {
			t.Fatalf("dry-run missing requested output: %s", out)
		}
		if strings.Contains(out, `"output_dir"`) {
			t.Fatalf("dry-run output_dir must be omitted with --output: %s", out)
		}
	})
}

func TestSlidesScreenshotDryRunReportsRequestedOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "2",
		"--output", "shots/cover.jpg",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"output": "shots/cover.jpg"`) {
		t.Fatalf("dry-run output = %s, want requested output path", stdout.String())
	}
	if strings.Contains(stdout.String(), `"output_dir"`) {
		t.Fatalf("dry-run output = %s, output_dir must be omitted with --output", stdout.String())
	}
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
