// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestSlidesCreateBasic verifies that slides +create returns the presentation ID and title in user mode.
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
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode")
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

	slidesJSON := `["<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>","<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>"]`
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

	slidesJSON := `["<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>","<bad-xml>"]`
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Partial",
		"--slides", slidesJSON,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "pres_partial") {
		t.Fatalf("error should contain presentation ID, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "slide 2/2") {
		t.Fatalf("error should indicate slide 2/2 failed, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "1 slide(s) added") {
		t.Fatalf("error should report 1 slide added before failure, got: %s", errMsg)
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
	slidesJSON := `["<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>","<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data></data></slide>"]`
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

// TestSlidesCreateWithoutSlidesUnchanged verifies existing behavior when --slides is not passed.
func TestSlidesCreateWithoutSlidesUnchanged(t *testing.T) {
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
