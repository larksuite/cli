// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestSlidesCreateBasic verifies that slides +create returns the presentation ID, title, and URL in user mode.
func TestSlidesCreateBasic(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_abc123",
				"revision_id":         1,
				"url":                 "https://tenant.example.com/slides/pres_abc123",
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "项目汇报",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_abc123" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc123", data["xml_presentation_id"])
	}
	if data["title"] != "项目汇报" {
		t.Fatalf("title = %v, want 项目汇报", data["title"])
	}
	if data["url"] != "https://tenant.example.com/slides/pres_abc123" {
		t.Fatalf("url = %v, want https://tenant.example.com/slides/pres_abc123", data["url"])
	}
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode")
	}
}

func TestBuildPresentationXMLUsesCanonicalHTTPSNamespace(t *testing.T) {
	got := buildPresentationXML("Demo")
	want := `<presentation xmlns="https://www.larkoffice.com/sml/2.0" width="960" height="540"><title>Demo</title></presentation>`
	if got != want {
		t.Fatalf("buildPresentationXML() = %q, want %q", got, want)
	}
}

// TestSlidesCreateBotAutoGrant verifies that bot mode grants the current user full_access on the new presentation.
func TestSlidesCreateBotAutoGrant(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, "ou_current_user"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_bot",
				"revision_id":         1,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/pres_bot/members",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_current_user",
					"member_type": "openid",
					"perm":        "full_access",
				},
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Bot PPT",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if !strings.Contains(grant["message"].(string), "presentation") {
		t.Fatalf("permission_grant.message = %q, want 'presentation' mention", grant["message"])
	}
}

// TestSlidesCreateBotSkippedWithoutCurrentUser verifies that permission grant is skipped when no user open_id is configured.
func TestSlidesCreateBotSkippedWithoutCurrentUser(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_no_user",
				"revision_id":         1,
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "No User PPT",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if hint, ok := grant["hint"].(string); !ok ||
		!strings.Contains(hint, "auth login") {
		t.Fatalf("hint = %#v, want actionable default authorization recovery", grant["hint"])
	}
}

func TestSlidesCreateBotAutoGrantFailed(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, "ou_current_user"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_grant_fail",
				"revision_id":         1,
			},
		},
	})

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/pres_grant_fail/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Grant Fail PPT",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	if hint, ok := grant["hint"].(string); !ok || !strings.Contains(hint, "Retry later") {
		t.Fatalf("hint = %#v, want string containing 'Retry later'", grant["hint"])
	}
}

// TestSlidesCreateDryRunDefaultTitle verifies that dry-run also normalizes an empty title to "Untitled".
func TestSlidesCreateDryRunDefaultTitle(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Untitled") {
		t.Fatalf("dry-run should contain Untitled in XML payload, got: %s", out)
	}
	if !strings.Contains(out, "xml_presentations") {
		t.Fatalf("dry-run should show API path, got: %s", out)
	}
}

// TestSlidesCreateDefaultTitle verifies that omitting --title outputs "Untitled" (matching the actual resource).
func TestSlidesCreateDefaultTitle(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_default",
				"revision_id":         1,
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["title"] != "Untitled" {
		t.Fatalf("title = %v, want Untitled", data["title"])
	}
}

// TestSlidesCreateMissingPresentationID verifies the error when the API returns no xml_presentation_id.
func TestSlidesCreateMissingPresentationID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"revision_id": 1,
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Missing ID",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error when xml_presentation_id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "xml_presentation_id") {
		t.Fatalf("error = %q, want mention of xml_presentation_id", err.Error())
	}
}

// TestSlidesCreateWithSlides verifies that slides +create with --slides creates the presentation and adds slides.
func TestSlidesCreateWithSlides(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_with_slides",
				"revision_id":         1,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_with_slides/slide",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"slide_id":    "slide_001",
				"revision_id": 2,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_with_slides/slide",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"slide_id":    "slide_002",
				"revision_id": 3,
			},
		},
	})

	slidesJSON := `["<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>","<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "With Slides",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_with_slides" {
		t.Fatalf("xml_presentation_id = %v, want pres_with_slides", data["xml_presentation_id"])
	}
	slideIDs, ok := data["slide_ids"].([]interface{})
	if !ok || len(slideIDs) != 2 {
		t.Fatalf("slide_ids = %v, want 2 elements", data["slide_ids"])
	}
	if slideIDs[0] != "slide_001" || slideIDs[1] != "slide_002" {
		t.Fatalf("slide_ids = %v, want [slide_001, slide_002]", slideIDs)
	}
	if data["slides_added"] != float64(2) {
		t.Fatalf("slides_added = %v, want 2", data["slides_added"])
	}
}

func TestSlidesCreatePreservesSchemaIssues(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_issues",
				"issues":              "presentation schema issue",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_issues/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"slide_id": "slide_001",
				"issues":   "slide schema issue",
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--slides", `["<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data/></slide>"]`,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["issues"] != "presentation schema issue" {
		t.Fatalf("issues = %v, want presentation schema issue", data["issues"])
	}
	slideIssues, ok := data["slide_issues"].([]interface{})
	if !ok || len(slideIssues) != 1 {
		t.Fatalf("slide_issues = %#v, want one entry", data["slide_issues"])
	}
	issue, _ := slideIssues[0].(map[string]interface{})
	if issue["slide_index"] != float64(1) || issue["slide_id"] != "slide_001" || issue["issues"] != "slide schema issue" {
		t.Fatalf("slide_issues[0] = %#v", issue)
	}
}

// TestSlidesCreateWithSlidesPartialFailure verifies error reporting when a slide fails to create.
func TestSlidesCreateWithSlidesPartialFailure(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_partial",
				"revision_id":         1,
			},
		},
	})
	// First slide succeeds
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_partial/slide",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"slide_id":    "slide_ok",
				"revision_id": 2,
			},
		},
	})
	// Second slide fails
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_partial/slide",
		Body: map[string]interface{}{
			"code": 400,
			"msg":  "invalid xml",
		},
	})

	// Page 2 is a structurally valid <slide>, so it reaches the API and is
	// rejected there — the case this test is about. A locally malformed page is
	// now caught before the presentation is created at all, which is a different
	// path with its own test.
	slidesJSON := `["<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>","<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data><shape type=\"text\" height=\"-6\"/></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Partial",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error, got %v", err)
	}
	// Attaching the progress hint must not rewrite what failed: the caller still
	// sees the API's own category, code and message rather than an internal error.
	if p.Category != errs.CategoryAPI {
		t.Fatalf("category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Code != 400 || p.Message != "invalid xml" {
		t.Fatalf("api failure not preserved: code=%d message=%q", p.Code, p.Message)
	}
	// The presentation was created but a slide add failed; the recovery hint
	// carries the partial-progress context (which presentation exists, how many
	// slides landed) so the caller can resume without recreating.
	if !strings.Contains(p.Hint, "pres_partial") {
		t.Fatalf("hint should contain presentation ID, got: %s", p.Hint)
	}
	if !strings.Contains(p.Hint, "slide 2/2") {
		t.Fatalf("hint should indicate slide 2/2 failed, got: %s", p.Hint)
	}
	if !strings.Contains(p.Hint, "1 slide(s) added") {
		t.Fatalf("hint should report 1 slide added before failure, got: %s", p.Hint)
	}
}

// TestSlidesCreateWithSlidesInvalidJSON verifies validation rejects non-JSON slides input.
func TestSlidesCreateWithSlidesInvalidJSON(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Bad JSON",
		"--slides", "not json",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "--slides invalid JSON") {
		t.Fatalf("error = %q, want --slides invalid JSON mention", err.Error())
	}
}

// TestSlidesCreateWithSlidesExceedsMax verifies validation rejects arrays exceeding the limit.
func TestSlidesCreateWithSlidesExceedsMax(t *testing.T) {
	t.Parallel()

	// Build a JSON array with 11 elements (exceeds maxSlidesPerCreate = 10)
	elems := make([]string, 11)
	for i := range elems {
		elems[i] = `"<slide/>"` //nolint:goconst
	}
	slidesJSON := "[" + strings.Join(elems, ",") + "]"

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Too Many",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error for exceeding max, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("error = %q, want 'exceeds maximum' mention", err.Error())
	}
}

// TestSlidesCreateValidationParam locks Param=="--slides" on the pure
// validation rejections, so callers route on the typed field rather than the
// message.
func TestSlidesCreateValidationParam(t *testing.T) {
	t.Parallel()

	elems := make([]string, 11)
	for i := range elems {
		elems[i] = `"<slide/>"`
	}
	exceedsMax := "[" + strings.Join(elems, ",") + "]"

	tests := []struct {
		name   string
		slides string
	}{
		{"invalid JSON", "not json"},
		{"exceeds max", exceedsMax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesCreateShortcut(t, f, stdout, []string{
				"+create",
				"--slides", tt.slides,
				"--as", "user",
			})

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != "--slides" {
				t.Fatalf("Param = %q, want --slides", ve.Param)
			}
		})
	}
}

// TestSlidesCreatePlaceholderMissingParam guards the create.go caller wiring:
// a missing @-placeholder file must surface a --slides-tagged validation error
// through the shared slidesInputStatError helper.
func TestSlidesCreatePlaceholderMissingParam(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slidesJSON := `["<slide><data><img src=\"@./missing.png\"/></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--slides", slidesJSON,
		"--as", "user",
	})

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *errs.ValidationError", err)
	}
	if ve.Param != "--slides" {
		t.Fatalf("Param = %q, want --slides", ve.Param)
	}
}

// TestSlidesCreateWithSlidesEmptyArray verifies that --slides '[]' behaves like no --slides.
func TestSlidesCreateWithSlidesEmptyArray(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_empty_slides",
				"revision_id":         1,
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Empty Slides",
		"--slides", "[]",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_empty_slides" {
		t.Fatalf("xml_presentation_id = %v, want pres_empty_slides", data["xml_presentation_id"])
	}
	if _, ok := data["slide_ids"]; ok {
		t.Fatalf("did not expect slide_ids for empty slides array")
	}
	if _, ok := data["slides_added"]; ok {
		t.Fatalf("did not expect slides_added for empty slides array")
	}
}

// TestSlidesCreateWithSlidesDryRun verifies dry-run output shows multi-step labels.
func TestSlidesCreateWithSlidesDryRun(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slidesJSON := `["<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>","<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "DryRun Slides",
		"--slides", slidesJSON,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "[1/3]") {
		t.Fatalf("dry-run should contain [1/3] step label, got: %s", out)
	}
	if !strings.Contains(out, "[2/3]") {
		t.Fatalf("dry-run should contain [2/3] step label, got: %s", out)
	}
	if !strings.Contains(out, "[3/3]") {
		t.Fatalf("dry-run should contain [3/3] step label, got: %s", out)
	}
	if !strings.Contains(out, "xml_presentation_id") {
		t.Fatalf("dry-run should contain placeholder xml_presentation_id, got: %s", out)
	}
}

// TestSlidesCreateWithoutSlidesReturnsNotificationMessage verifies that an empty
// presentation includes a user-facing kickoff-notification reminder.
func TestSlidesCreateWithoutSlidesReturnsNotificationMessage(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_no_slides",
				"revision_id":         1,
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "No Slides",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_no_slides" {
		t.Fatalf("xml_presentation_id = %v, want pres_no_slides", data["xml_presentation_id"])
	}
	if data["title"] != "No Slides" {
		t.Fatalf("title = %v, want No Slides", data["title"])
	}
	if data["message"] != "成功创建空白幻灯片，url：https://www.feishu.cn/slides/pres_no_slides，请给用户推送开工通知。" {
		t.Fatalf("message = %v, want empty-presentation notification reminder", data["message"])
	}
	if _, ok := data["slide_ids"]; ok {
		t.Fatalf("did not expect slide_ids when --slides not passed")
	}
	if _, ok := data["slides_added"]; ok {
		t.Fatalf("did not expect slides_added when --slides not passed")
	}
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode")
	}
}

// TestSlidesCreateURLFallsBackToLocalBuild verifies the presentation URL is
// constructed locally from the token when presentation.create omits url — no
// drive metas/batch_query call is made, so creation works for users who only
// authorized slides scopes. The httpmock registry has no batch_query stub
// registered; if the shortcut tried to call it, the request would fail the test.
func TestSlidesCreateURLFallsBackToLocalBuild(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_local_url",
				"revision_id":         1,
				"url":                 "",
			},
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Local URL",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_local_url" {
		t.Fatalf("xml_presentation_id = %v, want pres_local_url", data["xml_presentation_id"])
	}
	if data["url"] != "https://www.feishu.cn/slides/pres_local_url" {
		t.Fatalf("url = %v, want https://www.feishu.cn/slides/pres_local_url", data["url"])
	}
}

// TestXmlEscape verifies that XML special characters are properly escaped.
func TestXmlEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"a&b", "a&amp;b"},
		{"<script>", "&lt;script&gt;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"it's", "it&apos;s"},
	}
	for _, tt := range tests {
		got := xmlEscape(tt.input)
		if got != tt.want {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// createStubPresentation registers the presentation.create stub and returns the
// registry-backed slide stubs for a deck of n pages, in call order.
func createStubPresentation(t *testing.T, reg *httpmock.Registry, presentationID string, n int) []*httpmock.Stub {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"xml_presentation_id": presentationID, "revision_id": 1},
		},
	})
	stubs := make([]*httpmock.Stub, 0, n)
	for i := 0; i < n; i++ {
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/slides_ai/v1/xml_presentations/" + presentationID + "/slide",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"slide_id": fmt.Sprintf("s_%d", i+1), "revision_id": i + 2},
			},
		}
		reg.Register(stub)
		stubs = append(stubs, stub)
	}
	return stubs
}

// capturedSlideContent pulls slide.content out of a captured request body.
func capturedSlideContent(t *testing.T, stub *httpmock.Stub) string {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	slide, _ := body["slide"].(map[string]interface{})
	content, _ := slide["content"].(string)
	return content
}

// TestSlidesCreateAssemblesRepeatedSlideFiles is the reason --slide exists:
// building the --slides JSON array in the shell needs a JSON encoder for the
// quotes and newlines inside each page, so callers reached for jq — and every
// environment without jq turned that into an empty argument. Repeating
// --slide @page.xml lets the CLI do the encoding.
func TestSlidesCreateAssemblesRepeatedSlideFiles(t *testing.T) {
	dir := t.TempDir()
	// Multi-line XML with quotes: exactly what shell escaping mangles.
	page1 := "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\">\n  <data>\n    <shape type=\"text\"><content>Page \"one\"</content></shape>\n  </data>\n</slide>"
	page2 := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>`
	for name, content := range map[string]string{"slide-01.xml": page1, "slide-02.xml": page2} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stubs := createStubPresentation(t, reg, "pres_repeat", 2)

	if err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Assembled",
		"--slide", "@./slide-01.xml",
		"--slide", "@./slide-02.xml",
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Flag order is page order, and the file bytes arrive verbatim.
	if got := capturedSlideContent(t, stubs[0]); got != page1 {
		t.Fatalf("page 1 content = %q, want the file verbatim %q", got, page1)
	}
	if got := capturedSlideContent(t, stubs[1]); got != page2 {
		t.Fatalf("page 2 content = %q, want %q", got, page2)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["slides_added"] != float64(2) {
		t.Fatalf("slides_added = %v, want 2", data["slides_added"])
	}
}

// TestSlidesCreateReadsSlidesArrayFromFile covers the other half: callers who
// already have the JSON array no longer have to inline it into an argument.
func TestSlidesCreateReadsSlidesArrayFromFile(t *testing.T) {
	dir := t.TempDir()
	deck := `["<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data/></slide>"]`
	if err := os.WriteFile(filepath.Join(dir, "deck.json"), []byte(deck), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stubs := createStubPresentation(t, reg, "pres_json", 1)

	if err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "From File",
		"--slides", "@./deck.json",
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>`
	if got := capturedSlideContent(t, stubs[0]); got != want {
		t.Fatalf("slide content = %q, want %q", got, want)
	}
}

// TestSlidesCreateStripsBOMFromSlideFile keeps the two file forms behaving the
// same. The framework strips a leading BOM from every Input flag, so
// --slides @deck.json already tolerated editors that add one; --slide resolves
// @path itself, and without the same normalization the BOM read as text outside
// the root element and rejected a file the other form accepted.
func TestSlidesCreateStripsBOMFromSlideFile(t *testing.T) {
	dir := t.TempDir()
	page := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>`
	if err := os.WriteFile(filepath.Join(dir, "slide-01.xml"), []byte("\uFEFF"+page), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stubs := createStubPresentation(t, reg, "pres_bom", 1)

	if err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "BOM",
		"--slide", "@./slide-01.xml",
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The BOM is dropped rather than forwarded: the backend would reject it too.
	if got := capturedSlideContent(t, stubs[0]); got != page {
		t.Fatalf("slide content = %q, want the page without the BOM %q", got, page)
	}
}

// TestSlidesCreateRejectsBothSlideForms keeps page order unambiguous: merging
// the two forms would make it depend on flag-ordering rules.
func TestSlidesCreateRejectsBothSlideForms(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Both",
		"--slides", `["<slide/>"]`,
		"--slide", "<slide/>",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error when both forms are used")
	}
	assertValidationProblem(t, err, "--slide", nil)
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %q, want a combined-forms rejection", err.Error())
	}
}

// TestSlidesCreateRejectsMalformedSlideBeforeCreating locks the ordering that
// makes local validation worth having: a bad page must not leave an empty
// presentation behind. The registry has no stubs, so any API call fails loudly.
func TestSlidesCreateRejectsMalformedSlideBeforeCreating(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Malformed",
		"--slide", `<slide xmlns="x"><data/></slide>`,
		"--slide", `<presentation xmlns="x"><slide/></presentation>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a <presentation> root")
	}
	assertValidationProblem(t, err, "--slide", errAnyCause)
	// The index is what makes a 10-page deck debuggable.
	if !strings.Contains(err.Error(), "page 2") || !strings.Contains(err.Error(), "want <slide>") {
		t.Fatalf("error = %q, want the offending page index and the structural reason", err.Error())
	}
}

// TestSlidesCreateRejectsXMLDeclarationBeforeCreating covers the one malformed
// page the parser alone will not catch: `<?xml ...?>` is well-formed XML, so
// without an explicit check the deck gets created and only the slide request
// fails, which is the orphaned-empty-deck outcome this validation exists to
// prevent. No stubs, so a presentation create would fail loudly.
func TestSlidesCreateRejectsXMLDeclarationBeforeCreating(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Prolog",
		"--slide", "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data/></slide>",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a leading XML declaration")
	}
	assertValidationProblem(t, err, "--slide", errAnyCause)
	if !strings.Contains(err.Error(), "page 1") || !strings.Contains(err.Error(), "<?xml ...?>") {
		t.Fatalf("error = %q, want the page index and the declaration named", err.Error())
	}
}

// TestSlidesCreateSlideFileNotFound checks the missing-file path reports the
// flag, not a bare os error.
func TestSlidesCreateSlideFileNotFound(t *testing.T) {
	withSlidesTestWorkingDir(t, t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Missing",
		"--slide", "@./nope.xml",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a missing file")
	}
	assertValidationProblem(t, err, "--slide", errAnyCause)
}

// TestSlidesCreateSlideRejectsStdin documents the one thing a repeatable flag
// cannot do: a process has a single stdin, so "-" has no per-occurrence
// meaning. The hint has to name the two forms that do work.
func TestSlidesCreateSlideRejectsStdin(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Stdin",
		"--slide", "-",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for --slide -")
	}
	ve := assertValidationProblem(t, err, "--slide", nil)
	if !strings.Contains(ve.Hint, "@page.xml") || !strings.Contains(ve.Hint, "--slides -") {
		t.Fatalf("hint = %q, want both working alternatives", ve.Hint)
	}
}

// TestSlidesCreateSlideExceedsMax covers the page cap from the repeatable form.
// The cap is enforced on the assembled array, so it applies to both forms, but
// only this pins that the error names the flag the caller actually typed --
// the backend never sees a page count at all (one call per page), so a wrong
// flag name here is the caller's only signal about what to change.
func TestSlidesCreateSlideExceedsMax(t *testing.T) {
	t.Parallel()

	// Every page is structurally valid, so the count is the only possible
	// reason to fail.
	args := []string{"+create", "--title", "Too Many"}
	for i := 0; i <= maxSlidesPerCreate; i++ {
		args = append(args, "--slide", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>`)
	}
	args = append(args, "--as", "user")

	// No stubs registered: reaching the API would fail loudly, which is how
	// this also shows no empty presentation is left behind.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, args)
	if err == nil {
		t.Fatalf("expected a validation error for %d pages", maxSlidesPerCreate+1)
	}
	assertValidationProblem(t, err, "--slide", nil)
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("error = %q, want 'exceeds maximum' mention", err.Error())
	}
	// "--slides" contains "--slide", so Contains cannot tell the forms apart;
	// the absence of the plural is what proves the right flag was blamed.
	if strings.Contains(err.Error(), "--slides") {
		t.Fatalf("error = %q, blames --slides but the caller used --slide", err.Error())
	}
	// The way out has to be in the message: there is no larger value to pass.
	if !strings.Contains(err.Error(), "+add-slide") {
		t.Fatalf("error = %q, want the two-step alternative", err.Error())
	}
}

// TestSlidesCreateRejectsEmptySlidesValue pins the one input that must not be
// read as "no pages given". `--slides "$(...)"` collapses to an empty value
// whenever the substitution fails, and treating that as absent is what turns a
// broken array into a blank deck reported as success.
func TestSlidesCreateRejectsEmptySlidesValue(t *testing.T) {
	t.Parallel()

	// No stubs: a presentation create would fail loudly, so passing this test
	// also means nothing was created.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Empty Value",
		"--slides", "",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for an empty --slides value")
	}
	assertValidationProblem(t, err, "--slides", nil)
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("error = %q, want the JSON parse failure named", err.Error())
	}
	// An empty value is rejected while "[]" still creates a blank deck
	// (TestSlidesCreateWithSlidesEmptyArray): "" is not valid JSON, "[]" is a
	// deliberate zero pages.
}

// TestSlidesCreateRejectsNullSlidesValue closes the gap the empty-value check
// leaves open: null IS valid JSON for a slice, so it parses without error and
// leaves the array nil, which reads as "no pages given" and produces the same
// blank deck reported as success. Again no stubs, so nothing may be created.
func TestSlidesCreateRejectsNullSlidesValue(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Null Value",
		"--slides", "null",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a null --slides value")
	}
	assertValidationProblem(t, err, "--slides", nil)
	if !strings.Contains(err.Error(), "must be an array") {
		t.Fatalf("error = %q, want the array requirement named", err.Error())
	}
}

// TestSlidesCreateRejectsBothSlideFormsWithEmptySlides is the same combination
// as TestSlidesCreateRejectsBothSlideForms, except --slides carries no value.
// Detecting the form by value rather than by "was it typed" let this one slip
// through and silently run as if only --slide had been passed.
func TestSlidesCreateRejectsBothSlideFormsWithEmptySlides(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Both",
		"--slides", "",
		"--slide", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data/></slide>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for both forms")
	}
	assertValidationProblem(t, err, "--slide", nil)
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %q, want the mutual-exclusion message", err.Error())
	}
}

// TestUploadSlidesPlaceholdersReportsSourceFlag pins that the uploader blames
// the flag its caller was given: +create reads pages from --slides, +add-slide
// from --slide, and the name used to be hardcoded to the former. Driving the
// helper directly is deliberate -- both shortcuts check placeholder files in
// their Validate stage, so these branches are only reachable if a file stops
// being readable between validation and upload, and the flag name is the part
// that has to stay right when they are.
func TestUploadSlidesPlaceholdersReportsSourceFlag(t *testing.T) {
	for _, param := range []string{"--slide", "--slides"} {
		t.Run(param, func(t *testing.T) {
			dir := t.TempDir()
			withSlidesTestWorkingDir(t, dir)

			f, _, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			runtime := &common.RuntimeContext{Factory: f}

			// A directory, not a missing file: it proves the check reached the
			// "must be a regular file" branch rather than failing to stat.
			if err := os.Mkdir(filepath.Join(dir, "notafile.png"), 0o755); err != nil {
				t.Fatal(err)
			}

			tokens, uploaded, err := uploadSlidesPlaceholders(runtime, "pres_x", []string{"./notafile.png"}, param)
			if err == nil {
				t.Fatal("expected a validation error for a directory placeholder")
			}
			assertValidationProblem(t, err, param, nil)
			if uploaded != 0 || len(tokens) != 0 {
				t.Fatalf("uploaded = %d, tokens = %v, want nothing uploaded", uploaded, tokens)
			}
		})
	}
}

// slidesTestConfig returns a CliConfig for testing with the given user open ID.
func slidesTestConfig(t *testing.T, userOpenID string) *core.CliConfig {
	t.Helper()
	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "test-slides-create-" + suffix,
		AppSecret:  "secret-slides-create-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
}

// runSlidesCreateShortcut mounts and executes the slides +create shortcut with the given args.
func runSlidesCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "slides"}
	SlidesCreate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// decodeSlidesCreateEnvelope parses the JSON output and returns the data map.
func decodeSlidesCreateEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}

// TestSlidesCreateWithImagePlaceholders verifies @path placeholders are uploaded
// once each (with dedup) and replaced with file_tokens before slide.create runs.
//
// Not parallel: uses os.Chdir to pin local file paths to a temp dir.
func TestSlidesCreateWithImagePlaceholders(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("a.png", []byte("aa"), 0o644); err != nil {
		t.Fatalf("write a.png: %v", err)
	}
	if err := os.WriteFile("b.png", []byte("bb"), 0o644); err != nil {
		t.Fatalf("write b.png: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_img",
				"revision_id":         1,
			},
		},
	})

	// Two distinct images → two upload calls. a.png is referenced twice but
	// must be uploaded only once.
	uploadStubA := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok_a"}},
	}
	uploadStubB := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok_b"}},
	}
	reg.Register(uploadStubA)
	reg.Register(uploadStubB)

	// Slide stubs: capture the rewritten slide content to assert tokens were
	// actually substituted into the XML.
	slideStub1 := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "s1", "revision_id": 2}},
	}
	slideStub2 := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "s2", "revision_id": 3}},
	}
	reg.Register(slideStub1)
	reg.Register(slideStub2)

	slidesJSON := `[
	  "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data><img src=\"@a.png\" topLeftX=\"10\"/><img src=\"@b.png\" topLeftX=\"20\"/></data></slide>",
	  "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data><img src=\"@a.png\" topLeftX=\"30\"/></data></slide>"
	]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Img test",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["images_uploaded"] != float64(2) {
		t.Fatalf("images_uploaded = %v, want 2 (a.png deduped)", data["images_uploaded"])
	}
	if data["slides_added"] != float64(2) {
		t.Fatalf("slides_added = %v, want 2", data["slides_added"])
	}

	// Assert each slide.create body uses tokens (not @path placeholders), and
	// that both upload tokens reach at least one slide so a buggy mapping
	// where `@b.png` got rewritten to `tok_a` would still fail.
	hasTokB := false
	for _, stub := range []*httpmock.Stub{slideStub1, slideStub2} {
		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("decode slide body: %v", err)
		}
		slide, _ := body["slide"].(map[string]interface{})
		content, _ := slide["content"].(string)
		if strings.Contains(content, "@a.png") || strings.Contains(content, "@b.png") {
			t.Fatalf("slide content still contains placeholder: %s", content)
		}
		if !strings.Contains(content, "tok_a") {
			t.Fatalf("slide content missing tok_a: %s", content)
		}
		if strings.Contains(content, "tok_b") {
			hasTokB = true
		}
	}
	if !hasTokB {
		t.Fatal("expected at least one slide body to contain tok_b")
	}
}

// TestSlidesCreatePlaceholderFileMissing verifies validation rejects a missing local file
// up front, before the presentation is created.
func TestSlidesCreatePlaceholderFileMissing(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	// No HTTP mocks registered — Validate must reject before any API call.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slidesJSON := `["<slide><data><img src=\"@./missing.png\"/></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "missing img",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected validation error for missing placeholder file")
	}
	if !strings.Contains(err.Error(), "missing.png") {
		t.Fatalf("err = %v, want mention of missing.png", err)
	}
}

// TestSlidesCreateWithPlaceholdersDryRun verifies dry-run lists upload steps
// with placeholder files counted into the total.
func TestSlidesCreateWithPlaceholdersDryRun(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)
	if err := os.WriteFile("p1.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile("p2.png", []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slidesJSON := `["<slide><data><img src=\"@p1.png\"/><img src=\"@p2.png\"/></data></slide>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "dry imgs",
		"--slides", slidesJSON,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	// Bookend step markers: [1/4] = create presentation, [4/4] = add slide 1.
	// Upload steps in between use the helper's own [N] labels (no /total).
	for _, marker := range []string{"[1/4]", "[4/4]"} {
		if !strings.Contains(out, marker) {
			t.Fatalf("dry-run missing %s, got: %s", marker, out)
		}
	}
	if strings.Count(out, "upload_all") != 2 {
		t.Fatalf("dry-run should contain 2 upload_all calls, got: %s", out)
	}
	if !strings.Contains(out, slidesMediaParentType) {
		t.Fatalf("dry-run missing parent_type %q, got: %s", slidesMediaParentType, out)
	}
	if !strings.Contains(out, "Create presentation + upload 2 image(s)") {
		t.Fatalf("dry-run header should describe upload count, got: %s", out)
	}
}
