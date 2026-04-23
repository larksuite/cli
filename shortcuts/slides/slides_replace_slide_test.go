// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// TestReplaceSlideBlockReplaceInjectsID is the core regression: users write
// <shape>…</shape> as replacement and the CLI must stitch id="<block_id>"
// onto the root before sending. The backend returns 3350001 otherwise.
func TestReplaceSlideBlockReplaceInjectsID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"revision_id": 42},
		},
	}
	reg.Register(stub)

	parts := `[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\" width=\"100\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_xyz",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []struct {
			Action      string `json:"action"`
			BlockID     string `json:"block_id"`
			Replacement string `json:"replacement"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v\nraw=%s", err, stub.CapturedBody)
	}
	if len(body.Parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(body.Parts))
	}
	got := body.Parts[0]
	if got.Action != "block_replace" || got.BlockID != "bUn" {
		t.Fatalf("part = %+v", got)
	}
	// The replacement must have id="bUn" injected into the <shape> root.
	if !strings.Contains(got.Replacement, `id="bUn"`) {
		t.Fatalf("replacement missing id=\"bUn\": %q", got.Replacement)
	}
	if !strings.Contains(got.Replacement, `type="rect"`) {
		t.Fatalf("replacement dropped existing attr: %q", got.Replacement)
	}
	// Input was self-closing <shape ... />; the content-injection pass should
	// have expanded it to <shape ...><content/></shape>. Asserting both
	// branches here guards against a future reorder between ensureXMLRootID
	// and ensureShapeHasContent silently regressing the combined path.
	if !strings.Contains(got.Replacement, "<content/>") || !strings.Contains(got.Replacement, "</shape>") {
		t.Fatalf("self-closing shape should have been expanded with <content/>: %q", got.Replacement)
	}

	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v", data["xml_presentation_id"])
	}
	if data["slide_id"] != "slide_xyz" {
		t.Fatalf("slide_id = %v", data["slide_id"])
	}
	if data["revision_id"] != float64(42) {
		t.Fatalf("revision_id = %v, want 42", data["revision_id"])
	}
}

// TestReplaceSlideBlockReplacePreservesMatchingID verifies that if the user
// already wrote id="<block_id>" in their XML, the CLI leaves the value alone.
func TestReplaceSlideBlockReplacePreservesMatchingID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 7}},
	}
	reg.Register(stub)

	parts := `[{"action":"block_replace","block_id":"bab","replacement":"<shape id=\"bab\" type=\"text\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_xyz",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []struct {
			Replacement string `json:"replacement"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Parts[0].Replacement != `<shape id="bab" type="text"><content/></shape>` {
		t.Fatalf("replacement = %q, want <content/> auto-injected", body.Parts[0].Replacement)
	}
}

// TestReplaceSlideBlockReplaceOverridesMismatchedID verifies that if the user
// wrote the wrong id in their XML, the CLI rewrites it to match block_id.
func TestReplaceSlideBlockReplaceOverridesMismatchedID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 7}},
	}
	reg.Register(stub)

	parts := `[{"action":"block_replace","block_id":"bUn","replacement":"<shape id=\"wrong\" type=\"rect\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_xyz",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []struct {
			Replacement string `json:"replacement"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(body.Parts[0].Replacement, `id="bUn"`) ||
		strings.Contains(body.Parts[0].Replacement, `id="wrong"`) {
		t.Fatalf("replacement = %q, want id=\"bUn\" override", body.Parts[0].Replacement)
	}
}

// TestReplaceSlideBlockInsertPassthrough verifies block_insert parts are sent
// as-is (no id injection, since there is no block_id to inject).
func TestReplaceSlideBlockInsertPassthrough(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 5}},
	}
	reg.Register(stub)

	parts := `[{"action":"block_insert","insertion":"<shape type=\"rect\"/>","insert_before_block_id":"baa"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_xyz",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []map[string]interface{} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got := body.Parts[0]
	if got["action"] != "block_insert" {
		t.Fatalf("action = %v", got["action"])
	}
	if got["insertion"] != `<shape type="rect"><content/></shape>` {
		t.Fatalf("insertion mutated: %v", got["insertion"])
	}
	if got["insert_before_block_id"] != "baa" {
		t.Fatalf("insert_before_block_id = %v", got["insert_before_block_id"])
	}
	if _, hasID := got["block_id"]; hasID {
		t.Fatalf("block_insert should not carry block_id, got %v", got)
	}
}

// TestReplaceSlideRejectsStrReplace verifies str_replace is blocked at the
// CLI even though the backend supports it (product decision).
func TestReplaceSlideRejectsStrReplace(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	parts := `[{"action":"str_replace","pattern":"old","replacement":"new"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", parts,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for str_replace action")
	}
	if !strings.Contains(err.Error(), "str_replace") || !strings.Contains(err.Error(), "block_replace") {
		t.Fatalf("err = %v, want mention of both str_replace and block_replace", err)
	}
}

// TestReplaceSlideRejectsUnknownAction verifies unknown actions are rejected
// with a helpful error listing supported actions.
func TestReplaceSlideRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	parts := `[{"action":"nuke","block_id":"bUn"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", parts,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("err = %v, want 'unknown action'", err)
	}
}

// TestReplaceSlideMissingRequiredField checks per-action required fields.
func TestReplaceSlideMissingRequiredField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parts   string
		wantErr string
	}{
		{"block_replace missing block_id", `[{"action":"block_replace","replacement":"<shape/>"}]`, "block_id"},
		{"block_replace missing replacement", `[{"action":"block_replace","block_id":"bUn"}]`, "replacement"},
		{"block_insert missing insertion", `[{"action":"block_insert"}]`, "insertion"},
		{"empty action", `[{"block_id":"bUn"}]`, "action is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
				"+replace-slide",
				"--presentation", "pres_abc",
				"--slide-id", "s",
				"--parts", tt.parts,
				"--as", "user",
			})
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestReplaceSlidePartsNonStringField covers the type-assertion guards in
// parseReplaceParts — each string field must reject non-string JSON values
// rather than silently coercing or panicking.
func TestReplaceSlidePartsNonStringField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		parts   string
		wantErr string
	}{
		{
			"action is not a string",
			`[{"action":123,"block_id":"bUn","replacement":"<shape type=\"text\"/>"}]`,
			"action must be a string",
		},
		{
			"replacement is not a string",
			`[{"action":"block_replace","block_id":"bUn","replacement":123}]`,
			"replacement must be a string",
		},
		{
			"block_id is not a string",
			`[{"action":"block_replace","block_id":123,"replacement":"<shape/>"}]`,
			"block_id must be a string",
		},
		{
			"insertion is not a string",
			`[{"action":"block_insert","insertion":{"foo":"bar"}}]`,
			"insertion must be a string",
		},
		{
			"insert_before_block_id is not a string",
			`[{"action":"block_insert","insertion":"<shape/>","insert_before_block_id":true}]`,
			"insert_before_block_id must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
				"+replace-slide",
				"--presentation", "pres_abc",
				"--slide-id", "s",
				"--parts", tt.parts,
				"--as", "user",
			})
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestReplaceSlideWhitespaceOnlyParts hits parseReplaceParts' pre-decode
// guard for a raw value that trims to empty. Distinct from `[]` which
// falls through to validateReplaceParts' "at least 1 item" error.
func TestReplaceSlideWhitespaceOnlyParts(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", "   ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only --parts")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("err = %v, want 'cannot be empty'", err)
	}
}

// TestReplaceSlideReplacementWithoutRootElement covers the ensureXMLRootID
// error branch inside injectBlockReplaceIDs: validateReplaceParts accepts
// any non-empty string for replacement, but a payload with no XML root
// (plain text / comment-only) fails at id-injection time and must surface
// as a clean validation error instead of reaching the backend.
func TestReplaceSlideReplacementWithoutRootElement(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", `[{"action":"block_replace","block_id":"bUn","replacement":"plain text, no root element"}]`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for replacement without root element")
	}
	if !strings.Contains(err.Error(), "no root element") {
		t.Fatalf("err = %v, want 'no root element'", err)
	}
}

// TestReplaceSlideEmptyParts verifies the 1..200 size bounds.
func TestReplaceSlideEmptyParts(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", `[]`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for empty parts")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Fatalf("err = %v, want 'at least 1'", err)
	}
}

func TestReplaceSlideTooManyParts(t *testing.T) {
	t.Parallel()

	// Build 201 valid block_insert parts.
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 201; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"action":"block_insert","insertion":"<shape type=\"rect\"/>"}`)
	}
	b.WriteString("]")

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", b.String(),
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for >200 parts")
	}
	if !strings.Contains(err.Error(), "200") {
		t.Fatalf("err = %v, want mention of 200", err)
	}
}

// TestReplaceSlideInvalidJSON verifies a clear error for malformed --parts.
func TestReplaceSlideInvalidJSON(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", `not-json`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("err = %v, want 'invalid JSON'", err)
	}
}

// TestReplaceSlideWikiResolution verifies a wiki URL is resolved before the
// replace call, and the resolved token appears in the replace URL.
func TestReplaceSlideWikiResolution(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "real_pres",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/real_pres/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 1}},
	})

	parts := `[{"action":"block_insert","insertion":"<shape type=\"rect\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "https://x.feishu.cn/wiki/wikcn_abc",
		"--slide-id", "sid",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "real_pres" {
		t.Fatalf("xml_presentation_id = %v, want real_pres", data["xml_presentation_id"])
	}
}

// TestReplaceSlideDryRun verifies dry-run prints the URL with the slide_id
// query param and shows the id-injection result in the body.
func TestReplaceSlideDryRun(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	parts := `[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_xyz",
		"--parts", parts,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/slide/replace") {
		t.Fatalf("dry-run missing endpoint: %s", out)
	}
	if !strings.Contains(out, "slide_xyz") {
		t.Fatalf("dry-run missing slide_id: %s", out)
	}
	if !strings.Contains(out, `id=\"bUn\"`) && !strings.Contains(out, `id="bUn"`) {
		t.Fatalf("dry-run body should show injected id=\"bUn\": %s", out)
	}
}

// TestReplaceSlidePassThroughFailureFields verifies failed_part_index /
// failed_reason are returned when the server reports partial failure.
func TestReplaceSlidePassThroughFailureFields(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"revision_id":       3,
				"failed_part_index": 0,
				"failed_reason":     "block not found",
			},
		},
	})

	parts := `[{"action":"block_replace","block_id":"bxx","replacement":"<shape type=\"rect\"/>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", parts,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	if data["failed_part_index"] != float64(0) {
		t.Fatalf("failed_part_index = %v", data["failed_part_index"])
	}
	if data["failed_reason"] != "block not found" {
		t.Fatalf("failed_reason = %v", data["failed_reason"])
	}
}

func TestReplaceSlide3350001ErrorEnrichment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parts    string
		wantHint string
	}{
		{
			name:     "block_replace with non-existent block_id gets generic hint",
			parts:    `[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\" width=\"100\"/>"}]`,
			wantHint: "common causes",
		},
		{
			// Mixed block_replace+block_insert is supported by the backend
			// (empirically verified). A 3350001 in a mixed batch means something
			// else went wrong (bad block_id, invalid XML, etc.) — use generic hint.
			name:     "mixed actions gets generic hint",
			parts:    `[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\"><content/></shape>"},{"action":"block_insert","insertion":"<shape type=\"rect\"><content/></shape>"}]`,
			wantHint: "common causes",
		},
		{
			name:     "block_insert only gets generic hint",
			parts:    `[{"action":"block_insert","insertion":"<shape type=\"text\"><content/></shape>"}]`,
			wantHint: "common causes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/slide/replace",
				Body: map[string]interface{}{
					"code": 3350001,
					"msg":  "invalid param",
					"data": map[string]interface{}{},
				},
			})

			err := runSlidesShortcut(t, f, nil, SlidesReplaceSlide, []string{
				"+replace-slide",
				"--presentation", "pres_abc",
				"--slide-id", "s",
				"--parts", tt.parts,
				"--as", "user",
			})
			if err == nil {
				t.Fatal("expected error for 3350001")
			}
			var exitErr *output.ExitError
			if !errors.As(err, &exitErr) || exitErr.Detail == nil {
				t.Fatalf("expected ExitError with Detail, got %v", err)
			}
			if exitErr.Detail.Code != 3350001 {
				t.Fatalf("expected code 3350001, got %d", exitErr.Detail.Code)
			}
			if !strings.Contains(exitErr.Detail.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", exitErr.Detail.Hint, tt.wantHint)
			}
		})
	}
}

func TestReplaceSlideNon3350001ErrorNotEnriched(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled",
			"data": map[string]interface{}{},
		},
	})

	parts := `[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\"/>"}]`
	err := runSlidesShortcut(t, f, nil, SlidesReplaceSlide, []string{
		"+replace-slide",
		"--presentation", "pres_abc",
		"--slide-id", "s",
		"--parts", parts,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.Detail.Code != 99991672 {
		t.Fatalf("expected code 99991672, got %d", exitErr.Detail.Code)
	}
	// Non-3350001 errors must not have the slides-specific hint attached.
	// Assert the actual hint is not our 3350001 checklist, rather than a
	// string the hint never emits.
	if strings.Contains(exitErr.Detail.Hint, "common causes") {
		t.Fatalf("non-3350001 error should not get slides-specific hint, got %q", exitErr.Detail.Hint)
	}
}
