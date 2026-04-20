// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
			},
		},
	})
	registerBatchQueryStub(reg, "pres_abc123", "https://example.feishu.cn/slides/pres_abc123")

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
	if data["url"] != "https://example.feishu.cn/slides/pres_abc123" {
		t.Fatalf("url = %v, want https://example.feishu.cn/slides/pres_abc123", data["url"])
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
	registerBatchQueryStub(reg, "pres_bot", "https://example.feishu.cn/slides/pres_bot")
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
	registerBatchQueryStub(reg, "pres_no_user", "https://example.feishu.cn/slides/pres_no_user")

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
	registerBatchQueryStub(reg, "pres_default", "https://example.feishu.cn/slides/pres_default")

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
	registerBatchQueryStub(reg, "pres_with_slides", "https://example.feishu.cn/slides/pres_with_slides")
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
	registerBatchQueryStub(reg, "pres_empty_slides", "https://example.feishu.cn/slides/pres_empty_slides")

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
	registerBatchQueryStub(reg, "pres_no_slides", "https://example.feishu.cn/slides/pres_no_slides")

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

// TestSlidesCreateURLFetchBestEffort verifies that the shortcut succeeds even when batch_query fails.
func TestSlidesCreateURLFetchBestEffort(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"xml_presentation_id": "pres_no_url",
				"revision_id":         1,
			},
		},
	})
	// batch_query returns an error — URL fetch should be silently skipped
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99999,
			"msg":  "no permission",
		},
	})

	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "No URL",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeSlidesCreateEnvelope(t, stdout)
	if data["xml_presentation_id"] != "pres_no_url" {
		t.Fatalf("xml_presentation_id = %v, want pres_no_url", data["xml_presentation_id"])
	}
	if _, ok := data["url"]; ok {
		t.Fatalf("did not expect url when batch_query fails")
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

// registerBatchQueryStub registers a drive meta batch_query mock that returns the given URL.
func registerBatchQueryStub(reg *httpmock.Registry, token, url string) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{"doc_token": token, "doc_type": "slides", "title": "", "url": url},
				},
			},
		},
	})
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
	registerBatchQueryStub(reg, "pres_img", "https://x.feishu.cn/slides/pres_img")

	slidesJSON := `[
	  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><img src=\"@a.png\" topLeftX=\"10\"/><img src=\"@b.png\" topLeftX=\"20\"/></data></slide>",
	  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><data><img src=\"@a.png\" topLeftX=\"30\"/></data></slide>"
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
