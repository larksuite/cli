// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── V2 (OpenAPI) tests ──

func TestDocsCreateV2RemoteImageDryRunDownloadsAfterDocumentCreation(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", `<title>Remote image</title><img href="https://93.184.216.34/photo.png"/>`,
		"--dry-run",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("execute docs +create dry-run: %v", err)
	}
	var envelope struct {
		Data struct {
			API []common.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout)
	}
	if len(envelope.Data.API) < 3 {
		t.Fatalf("dry-run API calls = %d, want create, download, and upload: %#v", len(envelope.Data.API), envelope.Data.API)
	}
	if got := envelope.Data.API[0].URL; got != "/open-apis/docs_ai/v1/documents" {
		t.Fatalf("first API URL = %q, want document creation", got)
	}
	if got := envelope.Data.API[1].URL; got != "https://93.184.216.34/photo.png" {
		t.Fatalf("second API URL = %q, want remote image download", got)
	}
	if got := envelope.Data.API[2].URL; got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("third API URL = %q, want image upload", got)
	}
}

func TestDocsCreateV2RejectsBlockedRemoteImageBeforeDocumentCreation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(createStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", `<title>Blocked image</title><img href="http://127.0.0.1/image.png"/>`,
		"--as", "bot",
	})
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "href")
	if len(createStub.CapturedBodies) != 0 {
		t.Fatalf("document creation was called before remote image validation: %s", createStub.CapturedBody)
	}
}

func TestDocsCreateV2BotAutoGrantSuccess(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://example.feishu.cn/docx/doxcn_new_doc",
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcn_new_doc/members",
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
	}
	reg.Register(permStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>项目计划</title><h1>目标</h1>",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantGranted {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantGranted)
	}
	if grant["user_open_id"] != "ou_current_user" {
		t.Fatalf("permission_grant.user_open_id = %#v, want %q", grant["user_open_id"], "ou_current_user")
	}
	if grant["message"] != "Granted the current CLI user full_access on the new document." {
		t.Fatalf("permission_grant.message = %#v", grant["message"])
	}

	var body map[string]interface{}
	if err := json.Unmarshal(permStub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to parse permission request body: %v", err)
	}
	if body["member_type"] != "openid" || body["member_id"] != "ou_current_user" || body["perm"] != "full_access" || body["type"] != "user" {
		t.Fatalf("unexpected permission request body: %#v", body)
	}
}

func TestDocsCreateV2BotAutoGrantSkippedWithoutCurrentUser(t *testing.T) {
	t.Parallel()

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://example.feishu.cn/docx/doxcn_new_doc",
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>内容</title><p>正文</p>",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantSkipped {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantSkipped)
	}
	if _, ok := grant["user_open_id"]; ok {
		t.Fatalf("did not expect user_open_id when current user is missing: %#v", grant)
	}
	if !strings.Contains(stderr.String(), "auto-grant was skipped") {
		t.Fatalf("stderr missing auto-grant skipped warning; got:\n%s", stderr.String())
	}
}

func TestDocsCreateV2UserSkipsPermissionGrantAugmentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://example.feishu.cn/docx/doxcn_new_doc",
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>内容</title><p>正文</p>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	if _, ok := data["permission_grant"]; ok {
		t.Fatalf("did not expect permission_grant in user mode output: %#v", data)
	}
}

func TestDocsCreateV2BotAutoGrantFailureDoesNotFailCreate(t *testing.T) {
	t.Parallel()

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, "ou_current_user"))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://example.feishu.cn/docx/doxcn_new_doc",
		},
	})

	permStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxcn_new_doc/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	}
	reg.Register(permStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>内容</title><p>正文</p>",
		"--as", "bot",
	})
	if err != nil {
		t.Fatalf("document creation should still succeed when auto-grant fails, got: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	grant, _ := data["permission_grant"].(map[string]interface{})
	if grant["status"] != common.PermissionGrantFailed {
		t.Fatalf("permission_grant.status = %#v, want %q", grant["status"], common.PermissionGrantFailed)
	}
	wantMessage := "Resource was created, but granting current user full_access failed: no permission. You can retry later or continue using bot identity."
	if grant["message"] != wantMessage {
		t.Fatalf("permission_grant.message = %q, want %q", grant["message"], wantMessage)
	}
	if !strings.Contains(stderr.String(), "auto-grant failed") {
		t.Fatalf("stderr missing auto-grant failed warning; got:\n%s", stderr.String())
	}
}

func TestDocsCreateV2FallbackURLWhenBackendOmitsIt(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			// "url" deliberately omitted to exercise the fallback.
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>内容</title><p>正文</p>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		t.Fatalf("missing document in envelope: %#v", data)
	}
	if got, want := doc["url"], "https://www.feishu.cn/docx/doxcn_new_doc"; got != want {
		t.Fatalf("document.url = %#v, want %q (brand-standard fallback)", got, want)
	}
}

func TestDocsCreateV2PreservesBackendURL(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://tenant.larkoffice.com/docx/doxcn_new_doc",
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", "<title>内容</title><p>正文</p>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocsCreateEnvelope(t, stdout)
	doc, _ := data["document"].(map[string]interface{})
	if got, want := doc["url"], "https://tenant.larkoffice.com/docx/doxcn_new_doc"; got != want {
		t.Fatalf("document.url = %#v, want backend tenant URL %q (fallback must not overwrite)", got, want)
	}
}

func TestDocsCreateV2LargeXMLDryRunPlansCreateThenAppend(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	content := "<title>Large</title>\n" + strings.Repeat("<p>x</p>\n", 5_000)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--content", content,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("execute docs +create dry-run: %v", err)
	}
	var envelope struct {
		Data struct {
			API []common.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout)
	}
	if len(envelope.Data.API) != 3 {
		t.Fatalf("dry-run API calls = %d, want create + 2 appends", len(envelope.Data.API))
	}
	if envelope.Data.API[0].Method != "POST" || envelope.Data.API[0].URL != "/open-apis/docs_ai/v1/documents" {
		t.Fatalf("create call = %#v", envelope.Data.API[0])
	}
	if envelope.Data.API[1].Method != "PUT" || envelope.Data.API[1].URL != "/open-apis/docs_ai/v1/documents/<created_document_id>" {
		t.Fatalf("append call = %#v", envelope.Data.API[1])
	}
	createBody, _ := envelope.Data.API[0].Body.(map[string]interface{})
	appendBody, _ := envelope.Data.API[1].Body.(map[string]interface{})
	lastAppendBody, _ := envelope.Data.API[2].Body.(map[string]interface{})
	if got := strings.Count(common.GetString(createBody, "content"), "<p>x</p>"); got != 1_999 {
		t.Fatalf("create paragraph count = %d, want 1999", got)
	}
	if got := strings.Count(common.GetString(appendBody, "content"), "<p>x</p>"); got != 2_000 {
		t.Fatalf("append paragraph count = %d, want 2000", got)
	}
	if got := strings.Count(common.GetString(lastAppendBody, "content"), "<p>x</p>"); got != 1_001 {
		t.Fatalf("last append paragraph count = %d, want 1001", got)
	}
	if appendBody["command"] != "block_insert_after" || appendBody["block_id"] != "-1" || appendBody["revision_id"] != float64(-1) && appendBody["revision_id"] != -1 {
		t.Fatalf("append body = %#v", appendBody)
	}
}

func TestDocsCreateV2LargeXMLExecutesSerialAppendBatches(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_batched",
			"revision_id": 1,
			"new_blocks":  []interface{}{map[string]interface{}{"block_id": "create-block"}},
		},
	})
	appendStub := registerDocsAIStub(reg, "PUT", "/open-apis/docs_ai/v1/documents/doxcn_batched", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_batched",
			"revision_id": 3,
			"new_blocks":  []interface{}{map[string]interface{}{"block_id": "append-block"}},
		},
	})
	appendStub.Reusable = true
	content := "<title>Large</title>" + strings.Repeat("<p>x</p>", 10_000)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", content, "--as", "user",
	})
	if err != nil {
		t.Fatalf("execute docs +create: %v", err)
	}
	if len(createStub.CapturedBodies) != 1 || len(appendStub.CapturedBodies) != 5 {
		t.Fatalf("request counts: create=%d append=%d", len(createStub.CapturedBodies), len(appendStub.CapturedBodies))
	}
	data := decodeDocsCreateEnvelope(t, stdout)
	doc := common.GetMap(data, "document")
	if got := len(common.GetSlice(doc, "new_blocks")); got != 6 {
		t.Fatalf("aggregated new_blocks = %d, want 6", got)
	}
	if got := doc["revision_id"]; got != float64(3) {
		t.Fatalf("revision_id = %#v, want 3", got)
	}
}

func TestDocsCreateV2LargeXMLAppendFailureReturnsPartialResult(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_partial_batch",
			"revision_id": 1,
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/doxcn_partial_batch",
		Body: map[string]interface{}{
			"code": 12345,
			"msg":  "append failed",
		},
	})
	content := "<title>Large</title>" + strings.Repeat("<p>x</p>", 5_000)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", content, "--as", "user",
	})
	var partial *output.PartialFailureError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %T %v, want PartialFailureError", err, err)
	}
	var envelope map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode stdout: %v\n%s", decodeErr, stdout)
	}
	if envelope["ok"] != false {
		t.Fatalf("partial result ok = %#v, want false", envelope["ok"])
	}
	data := common.GetMap(envelope, "data")
	batch := common.GetMap(data, "create_batches")
	if batch["total_batches"] != float64(3) || batch["completed_batches"] != float64(1) || batch["failed_batch"] != float64(2) {
		t.Fatalf("create_batches = %#v", batch)
	}
	failure := common.GetMap(batch, "error")
	if failure["code"] != float64(12345) || !strings.Contains(common.GetString(failure, "message"), "append failed") {
		t.Fatalf("create_batches.error = %#v, want typed append failure", failure)
	}
	if got := common.GetString(common.GetMap(data, "document"), "document_id"); got != "doxcn_partial_batch" {
		t.Fatalf("document_id = %q", got)
	}
}

func TestDocsCreateV2MarkdownUsesClientBatching(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	content := strings.Repeat("paragraph\n\n", 6_000)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--doc-format", "markdown", "--content", content, "--dry-run", "--as", "user",
	})
	if err != nil {
		t.Fatalf("execute markdown dry-run: %v", err)
	}
	var envelope struct {
		Data struct {
			API []common.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v", err)
	}
	if len(envelope.Data.API) != 4 || envelope.Data.API[0].Method != "POST" || envelope.Data.API[1].Method != "PUT" || envelope.Data.API[2].Method != "PUT" || envelope.Data.API[3].Method != "PUT" {
		t.Fatalf("markdown dry-run calls = %#v", envelope.Data.API)
	}
}

func TestDocsCreateV2MarkdownTitleFlagUsesClientBatchingAt5001(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	content := strings.Repeat("paragraph\n\n", 5_000)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--title", "Boundary", "--doc-format", "markdown", "--content", content, "--dry-run", "--as", "user",
	})
	if err != nil {
		t.Fatalf("execute markdown dry-run: %v", err)
	}
	var envelope struct {
		Data struct {
			API []common.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v", err)
	}
	if len(envelope.Data.API) != 3 || envelope.Data.API[0].Method != "POST" || envelope.Data.API[1].Method != "PUT" || envelope.Data.API[2].Method != "PUT" {
		t.Fatalf("markdown title dry-run calls = %#v", envelope.Data.API)
	}
}

func TestDocsCreateV2RejectsTotalBlockLimitBeforeCreate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(createStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", strings.Repeat("<p>x</p>", 40_000), "--as", "user",
	})
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	if len(createStub.CapturedBodies) != 0 {
		t.Fatalf("create was called for oversized content")
	}
}

func TestDocsCreateV2RejectsCompatibleXMLTotalBlockLimitBeforeCreate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(createStub)

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", "<callout>" + strings.Repeat("<p>x</p>", 40_000), "--as", "user",
	})
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	if len(createStub.CapturedBodies) != 0 {
		t.Fatalf("create was called for oversized compatibility XML")
	}
}

func TestDocsCreateV2RejectsContentLimitsBeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		content string
		actual  int
		limit   int
	}{
		{
			name:    "xml block characters",
			format:  "xml",
			content: `<title>Limit</title><p>` + strings.Repeat("x", 100_001) + `</p>`,
			actual:  100_001,
			limit:   100_000,
		},
		{
			name:    "markdown block characters",
			format:  "markdown",
			content: "# Limit\n\n" + strings.Repeat("x", 100_001),
			actual:  100_001,
			limit:   100_000,
		},
		{
			name:    "xml late block characters",
			format:  "xml",
			content: `<title>Limit</title>` + strings.Repeat(`<p>prefix</p>`, 1_999) + `<p>` + strings.Repeat("x", 100_001) + `</p>`,
			actual:  100_001,
			limit:   100_000,
		},
		{
			name:    "xml table cells",
			format:  "xml",
			content: `<title>Limit</title>` + docsCreateXMLTable(2_001, 1),
			actual:  2_001,
			limit:   2_000,
		},
		{
			name:    "xml table columns",
			format:  "xml",
			content: `<title>Limit</title>` + docsCreateXMLTable(1, 101),
			actual:  101,
			limit:   100,
		},
		{
			name:    "xml late table columns",
			format:  "xml",
			content: `<title>Limit</title>` + strings.Repeat(`<p>prefix</p>`, 1_899) + docsCreateXMLTable(1, 101),
			actual:  101,
			limit:   100,
		},
		{
			name:    "markdown gfm table columns",
			format:  "markdown",
			content: docsCreateMarkdownTable(2, 101),
			actual:  101,
			limit:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
			createStub := &httpmock.Stub{
				Method: "POST", URL: "/open-apis/docs_ai/v1/documents", Optional: true,
				Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
			}
			reg.Register(createStub)

			err := runDocsCreateShortcut(t, f, stdout, []string{
				"+create", "--doc-format", tt.format, "--content", tt.content, "--as", "user",
			})
			assertDocsCreateLimitError(t, err, tt.actual, tt.limit)
			if len(createStub.CapturedBodies) != 0 {
				t.Fatalf("create API was called for %s", tt.name)
			}
		})
	}
}

func TestDocsCreateV2ContentLimitRunsBeforeLocalResourcePreparation(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := &httpmock.Stub{
		Method: "POST", URL: "/open-apis/docs_ai/v1/documents", Optional: true,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(createStub)
	content := `<p>` + strings.Repeat("x", 100_001) + `</p><img path="@missing.png"/>`

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", content, "--as", "user",
	})

	assertDocsCreateLimitError(t, err, 100_001, 100_000)
	if len(createStub.CapturedBodies) != 0 {
		t.Fatal("create API was called before content/resource preflight")
	}
}

func TestDocsCreateV2ContentLimitAcceptsExactBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "block characters", content: `<title>Limit</title><p>` + strings.Repeat("x", 100_000) + `</p>`},
		{name: "table columns", content: `<title>Limit</title>` + docsCreateXMLTable(1, 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
			createStub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
				"document": map[string]interface{}{"document_id": "doxcn_boundary", "revision_id": 1},
			})

			err := runDocsCreateShortcut(t, f, stdout, []string{
				"+create", "--content", tt.content, "--as", "user",
			})
			if err != nil {
				t.Fatalf("exact boundary rejected: %v", err)
			}
			if len(createStub.CapturedBodies) != 1 {
				t.Fatalf("create requests = %d, want 1", len(createStub.CapturedBodies))
			}
		})
	}
}

func TestDocsCreateV2RejectsUnsplittableFirstContainerBeforeCreate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	createStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(createStub)
	content := "<callout>" + strings.Repeat("<p>x</p>", 4_999) + "</callout>"

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create", "--content", content, "--as", "user",
	})
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	if len(createStub.CapturedBodies) != 0 {
		t.Fatalf("create was called for an unsplittable initial container")
	}
}

func TestMergeCreateBatchDataAggregatesResourceBlocksAndRevision(t *testing.T) {
	createData := map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": 1,
			"new_blocks":  []interface{}{map[string]interface{}{"block_id": "a", "block_token": "marker-a"}},
		},
	}
	batchData := map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": 2,
			"new_blocks":  []interface{}{map[string]interface{}{"block_id": "b", "block_token": "marker-b"}},
		},
	}

	mergeCreateBatchData(createData, batchData)

	doc := common.GetMap(createData, "document")
	if len(common.GetSlice(doc, "new_blocks")) != 2 || doc["revision_id"] != 2 {
		t.Fatalf("merged document = %#v", doc)
	}
}

func TestDocsCreateAPIVersionCompatFlagIsIgnored(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
			"url":         "https://example.feishu.cn/docx/doxcn_new_doc",
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "legacy",
		"--content", "<title>项目计划</title>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeDocsCreateEnvelope(t, stdout)
	doc, _ := data["document"].(map[string]interface{})
	if got, want := doc["document_id"], "doxcn_new_doc"; got != want {
		t.Fatalf("document.document_id = %#v, want %q", got, want)
	}
}

func TestDocsCreateRejectsLegacyV1Flags(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--markdown", "## 目标",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected legacy v1 flags to be rejected")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want typed problem", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %s/%s, want validation/invalid_argument", problem.Category, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	if got, want := validationErr.Param, "--markdown"; got != want {
		t.Fatalf("param = %q, want %q", got, want)
	}
	presented := problem.Message + "\n" + problem.Hint
	for _, want := range []string{
		"docs +create is v2-only",
		"the old v1 interface has been shut down",
		"legacy v1 flag(s) --markdown are no longer supported",
		"--markdown -> use --content with --doc-format markdown",
		"lark-cli docs +create --help",
	} {
		if !strings.Contains(presented, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func TestDocsCreateV2EmptyContentFileReportsPathAndRecovery(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("draft.xml", nil, 0o600); err != nil {
		t.Fatalf("write empty draft: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--doc-format", "xml",
		"--content", "@draft.xml",
		"--as", "user",
	})
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error does not expose a typed problem: %v", err)
	}
	if got, want := problem.Message, `--content file "draft.xml" is empty`; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if got, want := problem.Hint, `write non-empty XML or Markdown to this file; if the path was reserved by init-draft, use the exact data.draft_path returned by that command, then retry with --content "@./<data.draft_path>"`; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

// ── Helpers ──

func docsCreateTestConfig(t *testing.T, userOpenID string) *core.CliConfig {
	t.Helper()

	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:      "test-docs-create-" + suffix,
		AppSecret:  "secret-docs-create-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: userOpenID,
	}
}

func registerDocsCreateAPIStub(reg *httpmock.Registry, data map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		},
	})
}

func runDocsCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()

	return mountAndRunDocs(t, DocsCreate, args, f, stdout)
}

func decodeDocsCreateEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
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

func assertDocsCreateLimitError(t *testing.T, err error, actual, limit int) {
	t.Helper()
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--content")
	want := fmt.Sprintf("%d", actual)
	wantLimit := fmt.Sprintf("limit %d", limit)
	if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), wantLimit) {
		t.Fatalf("limit error = %q, want actual %d and limit %d", err, actual, limit)
	}
}

func docsCreateXMLTable(rows, columns int) string {
	var content strings.Builder
	content.WriteString("<table>")
	for row := 0; row < rows; row++ {
		content.WriteString("<tr>")
		for column := 0; column < columns; column++ {
			content.WriteString("<td><p>x</p></td>")
		}
		content.WriteString("</tr>")
	}
	content.WriteString("</table>")
	return content.String()
}

func docsCreateMarkdownTable(rows, columns int) string {
	var content strings.Builder
	writeRow := func(value string) {
		content.WriteByte('|')
		for column := 0; column < columns; column++ {
			content.WriteString(" " + value + " |")
		}
		content.WriteByte('\n')
	}
	writeRow("h")
	writeRow("---")
	for row := 1; row < rows; row++ {
		writeRow("x")
	}
	return content.String()
}
