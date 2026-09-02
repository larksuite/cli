// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestReplaceSlideDeclaredScopes(t *testing.T) {
	if got := SlidesReplaceSlide.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"slides:presentation:update", "slides:presentation:write_only"}) {
		t.Fatalf("user preflight scopes = %#v, want slides update/write_only only", got)
	}
	if got := SlidesReplaceSlide.ScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"slides:presentation:update", "slides:presentation:write_only"}) {
		t.Fatalf("bot preflight scopes = %#v, want slides update/write_only only", got)
	}

	got := SlidesReplaceSlide.DeclaredScopesForIdentity("user")
	want := []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declared scopes = %#v, want %#v", got, want)
	}
}

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
		// An actually-empty payload keeps the non-empty wording; only a wrong
		// field name gets rerouted to the unknown-field error.
		{"block_replace empty replacement", `[{"action":"block_replace","block_id":"bUn","replacement":""}]`, "requires non-empty replacement"},
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
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, ok = %v, want CategoryValidation/SubtypeInvalidArgument", problem, ok)
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != "--parts" {
				t.Fatalf("Param = %q, want %q", ve.Param, "--parts")
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
		{
			"payload alias is not a string",
			`[{"action":"replace","target_id":"bUn","content":{"type":"shape"}}]`,
			"replacement must be a string",
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
			assertValidationProblem(t, err, "--parts", nil)
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

// TestReplaceSlidePassesThroughIssues covers the field pointing the other way
// from failed_reason: the parts were applied and committed, and the backend
// still had something to say about the page they produced. A finding serious
// enough to refuse the write leaves as an error carrying the same report, so
// what arrives here always describes a page that is already stored.
func TestReplaceSlidePassesThroughIssues(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"revision_id": 4,
				"issues":      "[issue=text_overflows_container id=bgD level=warning]",
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
	if issues, _ := data["issues"].(string); issues != "[issue=text_overflows_container id=bgD level=warning]" {
		t.Fatalf("issues = %#v, want the backend report passed through verbatim", data["issues"])
	}
	if _, ok := data["failed_reason"]; ok {
		t.Fatalf("failed_reason should stay absent on a write that succeeded: %#v", data)
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
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected a typed errs.* error, got %v", err)
			}
			if p.Code != 3350001 {
				t.Fatalf("expected code 3350001, got %d", p.Code)
			}
			if !strings.Contains(p.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", p.Hint, tt.wantHint)
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
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %v", err)
	}
	if p.Code != 99991672 {
		t.Fatalf("expected code 99991672, got %d", p.Code)
	}
	// Non-3350001 errors must not have the slides-specific hint attached.
	// Assert the actual hint is not our 3350001 checklist, rather than a
	// string the hint never emits.
	if strings.Contains(p.Hint, "common causes") {
		t.Fatalf("non-3350001 error should not get slides-specific hint, got %q", p.Hint)
	}
}

// TestReplaceSlideValidationParam locks the structured Param on every
// +replace-slide validation error, so callers route on the typed field
// instead of parsing the message. Guards against a regression where the flag
// tag is dropped from any of the --slide-id / --parts validation branches.
func TestReplaceSlideValidationParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		slideID   string
		parts     string
		wantParam string
	}{
		{"slide-id empty", "", `[{"action":"block_insert","insertion":"<shape/>"}]`, "--slide-id"},
		{"parts whitespace-only", "s", "   ", "--parts"},
		{"parts invalid JSON", "s", "not-json", "--parts"},
		{"parts non-string field", "s", `[{"action":123}]`, "--parts"},
		{"parts empty array", "s", `[]`, "--parts"},
		{"parts missing required field", "s", `[{"action":"block_insert"}]`, "--parts"},
		{"parts str_replace rejected", "s", `[{"action":"str_replace","pattern":"a","replacement":"b"}]`, "--parts"},
		{"parts unknown action", "s", `[{"action":"nuke","block_id":"b"}]`, "--parts"},
		{"parts replacement without root", "s", `[{"action":"block_replace","block_id":"b","replacement":"plain text"}]`, "--parts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
				"+replace-slide",
				"--presentation", "pres_abc",
				"--slide-id", tt.slideID,
				"--parts", tt.parts,
				"--as", "user",
			})

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != tt.wantParam {
				t.Fatalf("Param = %q, want %q", ve.Param, tt.wantParam)
			}
		})
	}
}

// TestReplaceSlideUnknownPartField covers the field-name hallucinations seen in
// the wild: XML written into "content" (and friends) instead of the action's own
// payload field. Each case must name the wrong field and point at the right one,
// so the caller fixes the key instead of rewriting the value.
func TestReplaceSlideUnknownPartField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parts    string
		want     []string
		notWant  string
		wantHint string
	}{
		{
			name:  "xml alias",
			parts: `[{"action":"block_replace","block_id":"bUn","xml":"<shape/>"}]`,
			want:  []string{`unknown field "xml"`, `did you mean "replacement"?`},
		},
		{
			name:  "new_xml alias",
			parts: `[{"action":"block_replace","block_id":"bUn","new_xml":"<shape/>"}]`,
			want:  []string{`unknown field "new_xml"`, `did you mean "replacement"?`},
		},
		{
			name:  "block_xml alias",
			parts: `[{"action":"block_replace","block_id":"bUn","block_xml":"<shape/>"}]`,
			want:  []string{`unknown field "block_xml"`, `did you mean "replacement"?`},
		},
		{
			name:  "data alias",
			parts: `[{"action":"block_replace","block_id":"bUn","data":{"x":1}}]`,
			want:  []string{`unknown field "data"`, `did you mean "replacement"?`},
		},
		{
			// A shape attribute is not a payload alias: "recolor this block" is a
			// different intent from "here is my XML", so no field is guessed.
			name:    "shape attribute is not treated as a payload alias",
			parts:   `[{"action":"block_replace","block_id":"bUn","fill":"#fff"}]`,
			want:    []string{`unknown field "fill"`, "Valid fields for block_replace: action, block_id, replacement"},
			notWant: "did you mean",
		},
		{
			name:  "field of the other action",
			parts: `[{"action":"block_replace","block_id":"bUn","insertion":"<shape/>"}]`,
			want:  []string{`unknown field "insertion"`, "it belongs to block_insert"},
		},
		{
			name:  "block_id under block_insert",
			parts: `[{"action":"block_insert","insertion":"<shape/>","block_id":"bUn"}]`,
			want:  []string{`unknown field "block_id"`, "it belongs to block_replace"},
		},
		{
			// Casing and separator variants: models write the casing of whatever
			// language they're thinking in, so these must still resolve.
			name:  "capitalized alias",
			parts: `[{"action":"block_replace","block_id":"bUn","Content":"<shape/>"}]`,
			want:  []string{`unknown field "Content"`, `did you mean "replacement"?`},
		},
		{
			name:  "camelCase alias",
			parts: `[{"action":"block_replace","block_id":"bUn","newXml":"<shape/>"}]`,
			want:  []string{`unknown field "newXml"`, `did you mean "replacement"?`},
		},
		{
			name:  "right field wrong casing",
			parts: `[{"action":"block_replace","block_id":"bUn","Replacement":"<shape/>"}]`,
			want:  []string{`unknown field "Replacement"`, `did you mean "replacement"?`},
		},
		{
			name:  "right field camelCased",
			parts: `[{"action":"block_replace","blockId":"bUn","replacement":"<shape/>"}]`,
			want:  []string{`unknown field "blockId"`, `did you mean "block_id"?`},
		},
		{
			// The CLI's own flags are kebab-case (--slide-id), so callers copy
			// that separator into part fields too.
			name:  "right field kebab-cased",
			parts: `[{"action":"block_replace","block-id":"bUn","replacement":"<shape/>"}]`,
			want:  []string{`unknown field "block-id"`, `did you mean "block_id"?`},
		},
		{
			// "action" picks the schema, so a misspelling there would otherwise
			// skip the field check and surface only as "action is required".
			name:  "action itself misspelled",
			parts: `[{"Action":"block_replace","block_id":"bUn","replacement":"<shape/>"}]`,
			want:  []string{`unknown field "Action"`, `did you mean "action"?`},
		},
		{
			name:  "action misspelled with a separator",
			parts: `[{"action-":"block_replace","block_id":"bUn","replacement":"<shape/>"}]`,
			want:  []string{`unknown field "action-"`, `did you mean "action"?`},
		},
		{
			name:  "camelCased field of the other action",
			parts: `[{"action":"block_replace","block_id":"bUn","insertBeforeBlockId":"bab","replacement":"<shape/>"}]`,
			want:  []string{`unknown field "insertBeforeBlockId"`, "it belongs to block_insert"},
		},
		{
			// Not in the alias table: still rejected, but without a misleading
			// "did you mean" pointing at an unrelated field.
			name:    "unlisted typo falls back to the field list",
			parts:   `[{"action":"block_replace","block_id":"bUn","replacment":"<shape/>"}]`,
			want:    []string{`unknown field "replacment"`, "Valid fields for block_replace: action, block_id, replacement"},
			notWant: "did you mean",
		},
		{
			// Two bad fields: the reported one is sorted-first, so the message
			// doesn't flip between runs on Go's randomized map iteration.
			// TestReplaceSlideUnknownFieldIsDeterministic re-parses to prove it.
			name:  "several bad fields report deterministically",
			parts: `[{"action":"block_replace","block_id":"bUn","zzz":"1","content":"<shape/>"}]`,
			want:  []string{`unknown field "zzz"`},
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
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want substring %q", err, want)
				}
			}
			if tt.notWant != "" && strings.Contains(err.Error(), tt.notWant) {
				t.Fatalf("err = %v, should not contain %q", err, tt.notWant)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, ok = %v, want CategoryValidation/SubtypeInvalidArgument", problem, ok)
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != "--parts" {
				t.Fatalf("Param = %q, want %q", ve.Param, "--parts")
			}
			if ve.Hint == "" {
				t.Fatalf("hint is empty; the correct-shape example should be attached")
			}
			if tt.wantHint != "" && !strings.Contains(ve.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", ve.Hint, tt.wantHint)
			}
		})
	}
}

// TestReplaceSlideAllKnownPartFieldsAccepted guards the field whitelist against
// typos: every legitimate field of both actions must survive a mixed batch.
func TestReplaceSlideAllKnownPartFieldsAccepted(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	parts := `[` +
		`{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\"/>"},` +
		`{"action":"block_insert","insertion":"<shape type=\"rect\"/>","insert_before_block_id":"bUn"}` +
		`]`
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
	if out := stdout.String(); !strings.Contains(out, "insert_before_block_id") {
		t.Fatalf("dry-run body should keep insert_before_block_id: %s", out)
	}
}

// TestReplaceSlideUnknownFieldIsDeterministic re-parses one payload carrying
// several rejected fields. A single parse would pass by luck even without the
// key sort, since Go's randomized map iteration can still land on the same
// field; repeating it makes the sort the only way every message matches.
func TestReplaceSlideUnknownFieldIsDeterministic(t *testing.T) {
	t.Parallel()

	// "content" is normalized first; "data" and "zzz" remain rejected, and sorted
	// order reports "data".
	parts := `[{"action":"block_replace","block_id":"bUn","zzz":"1","data":{"x":1},"content":"<shape/>"}]`
	const want = `--parts[0] unknown field "data"; did you mean "replacement"? Valid fields for block_replace: action, block_id, replacement`

	for i := 0; i < 50; i++ {
		_, err := parseReplaceParts(parts)
		if err == nil {
			t.Fatalf("parse %d: expected an error", i)
		}
		if err.Error() != want {
			t.Fatalf("parse %d: err = %q, want %q", i, err.Error(), want)
		}
	}
}

func TestReplaceSlideNormalizesCompatibleParts(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 9}},
	}
	reg.Register(stub)

	parts := `[` +
		`{"action":"replace","target_id":"bUn","content":"<shape type=\"text\"/>"},` +
		`{"action":"insert","element":"<shape type=\"rect\"/>"}` +
		`]`
	if err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
		"+replace-slide", "--presentation", "pres_abc", "--slide-id", "s",
		"--parts", parts, "--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []map[string]interface{} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got := body.Parts[0]["action"]; got != "block_replace" {
		t.Fatalf("replace action = %v, want block_replace", got)
	}
	if got := body.Parts[0]["block_id"]; got != "bUn" {
		t.Fatalf("target_id = %v, want block_id bUn", got)
	}
	if got := body.Parts[0]["replacement"]; got != `<shape type="text" id="bUn"><content/></shape>` {
		t.Fatalf("replacement = %v, want alias payload with only id and content injected", got)
	}
	if _, ok := body.Parts[0]["content"]; ok {
		t.Fatalf("wire part must not retain content alias: %#v", body.Parts[0])
	}
	if got := body.Parts[1]["action"]; got != "block_insert" {
		t.Fatalf("insert action = %v, want block_insert", got)
	}
	if got := body.Parts[1]["insertion"]; got != `<shape type="rect"><content/></shape>` {
		t.Fatalf("insertion = %v, want alias payload with only content injected", got)
	}
	if _, ok := body.Parts[1]["element"]; ok {
		t.Fatalf("wire part must not retain element alias: %#v", body.Parts[1])
	}

	data := decodeShortcutData(t, stdout)
	normalizations, ok := data["normalizations"].([]interface{})
	if !ok || len(normalizations) != 5 {
		t.Fatalf("normalizations = %#v, want five action/field conversions", data["normalizations"])
	}
}

func TestReplaceSlideNormalizesAllCompatiblePayloadAliases(t *testing.T) {
	t.Parallel()

	const payload = `<shape type="rect"/>`
	for _, action := range []string{"block_replace", "block_insert"} {
		for _, alias := range compatibleXMLPayloadAliases {
			t.Run(action+"/"+alias, func(t *testing.T) {
				t.Parallel()
				part := map[string]interface{}{
					"action": action,
					alias:    payload,
				}
				if action == "block_replace" {
					part["block_id"] = "bUn"
				}
				raw, err := json.Marshal([]map[string]interface{}{part})
				if err != nil {
					t.Fatal(err)
				}
				parsed, normalizations, err := parseReplacePartsWithNormalization(string(raw))
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(normalizations) != 1 {
					t.Fatalf("normalizations = %#v, want one", normalizations)
				}
				if action == "block_replace" && (parsed[0].Replacement == nil || *parsed[0].Replacement != payload) {
					t.Fatalf("replacement = %v, want exact alias payload %q", parsed[0].Replacement, payload)
				}
				if action == "block_insert" && (parsed[0].Insertion == nil || *parsed[0].Insertion != payload) {
					t.Fatalf("insertion = %v, want exact alias payload %q", parsed[0].Insertion, payload)
				}
			})
		}
	}
}

func TestReplaceSlideRejectsNormalizationConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts string
	}{
		{
			name:  "target id conflicts with block id",
			parts: `[{"action":"replace","block_id":"bOne","target_id":"bTwo","content":"<shape/>"}]`,
		},
		{
			name:  "payload alias conflicts with canonical field",
			parts: `[{"action":"insert","insertion":"<shape type=\"rect\"/>","element":"<img src=\"token\"/>"}]`,
		},
		{
			name:  "two payload aliases conflict",
			parts: `[{"action":"replace","target_id":"bUn","content":"<shape/>","element":"<img src=\"token\"/>"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseReplacePartsWithNormalization(tt.parts)
			if err == nil || !strings.Contains(err.Error(), "conflict") {
				t.Fatalf("err = %v, want conflict", err)
			}
			p, ok := errs.ProblemOf(err)
			var ve *errs.ValidationError
			if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument ||
				!errors.As(err, &ve) || ve.Param != "--parts" {
				t.Fatalf("problem = %#v, error = %v, want typed --parts validation error", p, err)
			}
		})
	}
}

func TestReplaceSlideAcceptsDuplicateAliasValues(t *testing.T) {
	t.Parallel()

	parts := `[{"action":"replace","block_id":"bUn","target_id":"bUn","replacement":"<shape/>","content":"<shape/>"}]`
	parsed, normalizations, err := parseReplacePartsWithNormalization(parts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed) != 1 || parsed[0].BlockID == nil || *parsed[0].BlockID != "bUn" {
		t.Fatalf("parsed = %#v", parsed)
	}
	if parsed[0].Replacement == nil || *parsed[0].Replacement != "<shape/>" {
		t.Fatalf("replacement = %v, want exact folded payload %q", parsed[0].Replacement, "<shape/>")
	}
	if len(normalizations) != 3 {
		t.Fatalf("normalizations = %#v, want action plus two folded fields", normalizations)
	}
}

func TestReplaceSlideRejectsSemanticallyDifferentActions(t *testing.T) {
	t.Parallel()

	for _, action := range []string{"str_replace", "replace_all", "page_replace", "slide_replace"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			parts := fmt.Sprintf(`[{"action":%q,"target_id":"bUn","content":"<shape/>"}]`, action)
			err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, []string{
				"+replace-slide", "--presentation", "pres_abc", "--slide-id", "s",
				"--parts", parts, "--as", "user",
			})
			if err == nil || !strings.Contains(err.Error(), action) {
				t.Fatalf("err = %v, want action %q named", err, action)
			}
			assertValidationProblem(t, err, "--parts", nil)
			if action == "page_replace" || action == "slide_replace" {
				if !strings.Contains(err.Error(), "slides +update-slide") {
					t.Fatalf("err = %v, want whole-page recovery command", err)
				}
			}
		})
	}
}

func TestReplaceSlideInvalidJSONSuggestsFileInput(t *testing.T) {
	t.Parallel()

	_, _, err := parseReplacePartsWithNormalization(`"'[{"action":"replace"}`)
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	ve := assertValidationProblem(t, err, "--parts", errAnyCause)
	if ve.Param != "--parts" || !strings.Contains(ve.Hint, "--parts @parts.json") ||
		!strings.Contains(ve.Hint, "--parts -") {
		t.Fatalf("validation error = %#v, want file and stdin recovery guidance", ve)
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("err = %v, want preserved *json.SyntaxError cause", err)
	}
}
