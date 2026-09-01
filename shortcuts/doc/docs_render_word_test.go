// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const wordRenderTestDownloadURL = "https://93.184.216.34/render.pdf"

func TestDocsRenderWordDryRunPlansCreatePollAndDownload(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	if err := os.WriteFile("report.docx", []byte("docx"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-dry-run"))
	if err := mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word", "--file", "report.docx", "--output", "report.pdf", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	var envelope struct {
		Data struct {
			API   []common.DryRunAPICall `json:"api"`
			Files []struct {
				Name     string `json:"name"`
				IfExists string `json:"if_exists"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if len(envelope.Data.API) != 3 {
		t.Fatalf("API calls = %#v", envelope.Data.API)
	}
	if envelope.Data.API[0].Method != http.MethodPost || envelope.Data.API[0].URL != wordRenderCreatePath ||
		envelope.Data.API[1].Method != http.MethodGet || envelope.Data.API[1].URL != wordRenderStatusPath ||
		envelope.Data.API[2].URL != "https://presigned.invalid/render.pdf" {
		t.Fatalf("API calls = %#v", envelope.Data.API)
	}
	if len(envelope.Data.Files) != 1 || envelope.Data.Files[0].Name != "report.pdf" || envelope.Data.Files[0].IfExists != wordRenderIfExistsError {
		t.Fatalf("files = %#v", envelope.Data.Files)
	}
}

func TestDocsRenderWordStatusDryRunPlansOptionalDownload(t *testing.T) {
	factory, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-status-dry-run"))
	if err := mountAndRunDocs(t, DocsRenderWordStatus, []string{
		"+render-word-status", "--task-id", "render_test", "--output", "report.pdf", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	var envelope struct {
		Data struct {
			API []common.DryRunAPICall `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(envelope.Data.API) != 2 || envelope.Data.API[0].URL != "/open-apis/docs_ai/v1/word_render_tasks/render_test" {
		t.Fatalf("API calls = %#v", envelope.Data.API)
	}
}

func TestDocsRenderWordFileValidationBoundaries(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	writeSizedDocTestFile(t, "limit.docx", wordRenderMaxDOCXSizeBytes)
	writeSizedDocTestFile(t, "too-large.docx", wordRenderMaxDOCXSizeBytes+1)

	factory, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-size"))
	if err := mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word", "--file", "limit.docx", "--output", "limit.pdf", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("20 MiB boundary rejected: %v", err)
	}
	err := mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word", "--file", "too-large.docx", "--output", "large.pdf", "--dry-run", "--as", "user",
	}, factory, stdout)
	assertWordRenderProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--file")

	if err := os.WriteFile("empty.docx", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err = mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word", "--file", "empty.docx", "--output", "empty.pdf", "--dry-run", "--as", "user",
	}, factory, stdout)
	assertWordRenderProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--file")
}

func TestDocsRenderWordProcessingToSucceededDownloadsPDF(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	docxBytes := []byte("DOCX-BYTES")
	if err := os.WriteFile("report.docx", docxBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	pdfBytes := []byte("%PDF-1.7\nrendered")

	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-e2e"))
	createStub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    wordRenderCreatePath,
		Body: wordRenderAPIResponse(map[string]interface{}{
			"task_id":       "render_test",
			"status":        "processing",
			"stage":         "converting",
			"poll_after_ms": 1,
		}),
	}
	registry.Register(createStub)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/docs_ai/v1/word_render_tasks/render_test",
		Body: wordRenderAPIResponse(map[string]interface{}{
			"task_id":       "render_test",
			"status":        "processing",
			"stage":         "mapping",
			"poll_after_ms": 1,
		}),
	})
	succeededTask := wordRenderSucceededTask(int64(len(pdfBytes)))
	succeededTask["task_id"] = "render_test"
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/docs_ai/v1/word_render_tasks/render_test",
		Body:   wordRenderAPIResponse(succeededTask),
	})
	downloadStub := &httpmock.Stub{
		Method:      http.MethodGet,
		URL:         wordRenderTestDownloadURL,
		RawBody:     pdfBytes,
		ContentType: "application/pdf",
	}
	registry.Register(downloadStub)

	if err := mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word",
		"--file", "report.docx",
		"--output", "report.pdf",
		"--idempotency-key", "idem-explicit",
		"--wait-timeout-seconds", "5",
		"--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	written, err := os.ReadFile("report.pdf")
	if err != nil {
		t.Fatalf("read PDF: %v", err)
	}
	if !bytes.Equal(written, pdfBytes) {
		t.Fatalf("PDF bytes = %q", written)
	}

	multipartBody := decodeWordRenderMultipart(t, createStub)
	if multipartBody.fields["idempotency_key"] != "idem-explicit" ||
		multipartBody.fileName != "report.docx" || !bytes.Equal(multipartBody.file, docxBytes) {
		t.Fatalf("multipart = %#v", multipartBody)
	}
	if got := downloadStub.CapturedHeaders.Get("Authorization"); got != "" {
		t.Fatalf("presigned download leaked Authorization header: %q", got)
	}

	data := decodeWordRenderEnvelope(t, stdout)
	if data["task_id"] != "render_test" || data["status"] != "succeeded" || data["downloaded_size"].(float64) != float64(len(pdfBytes)) {
		t.Fatalf("output = %#v", data)
	}
	if path, _ := data["output_path"].(string); !filepath.IsAbs(path) || !strings.HasSuffix(path, "report.pdf") {
		t.Fatalf("output_path = %#v", data["output_path"])
	}
	if headings, _ := data["headings"].([]interface{}); len(headings) != 1 {
		t.Fatalf("headings = %#v", data["headings"])
	}
	if warnings, _ := data["warnings"].([]interface{}); len(warnings) != 1 {
		t.Fatalf("warnings = %#v", data["warnings"])
	}
}

func TestDocsRenderWordGeneratesIdempotencyKeyAndTimesOutCleanly(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	if err := os.WriteFile("report.docx", []byte("docx"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-timeout"))
	createStub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    wordRenderCreatePath,
		Body: wordRenderAPIResponse(map[string]interface{}{
			"task_id": "render_timeout", "status": "processing", "stage": "converting",
		}),
	}
	registry.Register(createStub)
	if err := mountAndRunDocs(t, DocsRenderWord, []string{
		"+render-word", "--file", "report.docx", "--output", "report.pdf",
		"--wait-timeout-seconds", "0", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	multipartBody := decodeWordRenderMultipart(t, createStub)
	if key := multipartBody.fields["idempotency_key"]; !strings.HasPrefix(key, "lark-cli-") || len(key) <= len("lark-cli-") {
		t.Fatalf("generated idempotency key = %q", key)
	}
	data := decodeWordRenderEnvelope(t, stdout)
	if data["timed_out"] != true || !strings.Contains(data["next_command"].(string), "+render-word-status") {
		t.Fatalf("timeout output = %#v", data)
	}
	if _, err := os.Stat("report.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("timeout unexpectedly wrote output, stat error = %v", err)
	}
}

func TestDocsRenderWordStatusDownloadsSucceededTask(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	pdfBytes := []byte("%PDF-status")
	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-status"))
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/docs_ai/v1/word_render_tasks/render_status",
		Body:   wordRenderAPIResponse(wordRenderSucceededTask(int64(len(pdfBytes)))),
	})
	registry.Register(&httpmock.Stub{Method: http.MethodGet, URL: wordRenderTestDownloadURL, RawBody: pdfBytes, ContentType: "application/pdf"})
	if err := mountAndRunDocs(t, DocsRenderWordStatus, []string{
		"+render-word-status", "--task-id", "render_status", "--output", "status.pdf", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if got, err := os.ReadFile("status.pdf"); err != nil || !bytes.Equal(got, pdfBytes) {
		t.Fatalf("downloaded PDF = %q, err=%v", got, err)
	}
}

func TestDocsRenderWordStatusFailedAndExpiredAreTyped(t *testing.T) {
	tests := []struct {
		name    string
		task    map[string]interface{}
		subtype errs.Subtype
	}{
		{
			name: "failed",
			task: map[string]interface{}{
				"task_id": "render_failed", "status": "failed",
				"failure": map[string]interface{}{"code": "drive_preview_failed", "message": "conversion failed"},
			},
			subtype: errs.SubtypeServerError,
		},
		{
			name:    "expired",
			task:    map[string]interface{}{"task_id": "render_expired", "status": "expired"},
			subtype: errs.SubtypeNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-"+test.name))
			taskID := test.task["task_id"].(string)
			registry.Register(&httpmock.Stub{
				Method: http.MethodGet,
				URL:    "/open-apis/docs_ai/v1/word_render_tasks/" + taskID,
				Body:   wordRenderAPIResponse(test.task),
			})
			err := mountAndRunDocs(t, DocsRenderWordStatus, []string{
				"+render-word-status", "--task-id", taskID, "--as", "user",
			}, factory, stdout)
			assertWordRenderProblem(t, err, errs.CategoryAPI, test.subtype, "")
		})
	}
}

func TestDownloadWordRenderPDFRejectsBlockedAndNonPDFResponses(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	factory, _, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-download-security"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+render-word-status"},
		docsTestConfigWithAppID("word-render-download-security"), factory, core.AsUser)

	blockedTask := wordRenderTask{TaskID: "render_blocked", Status: "succeeded", PDF: &wordRenderPDF{DownloadURL: "http://127.0.0.1/pdf"}}
	runtime.Format = "json"
	_, _, err := downloadWordRenderPDF(context.Background(), runtime, blockedTask, "blocked.pdf", wordRenderIfExistsError)
	assertWordRenderProblem(t, err, errs.CategoryPolicy, errs.SubtypeAccessDenied, "")
	var policyErr *errs.SecurityPolicyError
	if !errors.As(err, &policyErr) {
		t.Fatalf("blocked download error type = %T, want *errs.SecurityPolicyError", err)
	}
	if policyErr.DownloadURL != blockedTask.PDF.DownloadURL {
		t.Fatalf("download_url = %q, want %q", policyErr.DownloadURL, blockedTask.PDF.DownloadURL)
	}

	runtime.Format = "pretty"
	_, _, err = downloadWordRenderPDF(context.Background(), runtime, blockedTask, "blocked-pretty.pdf", wordRenderIfExistsError)
	if !errors.As(err, &policyErr) {
		t.Fatalf("pretty blocked download error type = %T, want *errs.SecurityPolicyError", err)
	}
	if policyErr.DownloadURL != "" {
		t.Fatalf("pretty download_url = %q, want omitted", policyErr.DownloadURL)
	}

	registry.Register(&httpmock.Stub{Method: http.MethodGet, URL: wordRenderTestDownloadURL, RawBody: []byte("hello world"), ContentType: "text/plain"})
	nonPDFTask := wordRenderTask{TaskID: "render_non_pdf", Status: "succeeded", PDF: &wordRenderPDF{DownloadURL: wordRenderTestDownloadURL}}
	_, _, err = downloadWordRenderPDF(context.Background(), runtime, nonPDFTask, "non-pdf.pdf", wordRenderIfExistsError)
	assertWordRenderProblem(t, err, errs.CategoryNetwork, errs.SubtypeNetworkProtocol, "")
	if _, err := os.Stat("non-pdf.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-PDF response wrote a file, stat error = %v", err)
	}
}

func TestResolveWordRenderOutputPolicies(t *testing.T) {
	dir := t.TempDir()
	withDocsWorkingDir(t, dir)
	if err := os.WriteFile("report.pdf", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("report (1).pdf", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("word-render-output"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+render-word"},
		docsTestConfigWithAppID("word-render-output"), factory, core.AsUser)

	if _, err := resolveWordRenderOutputPath(runtime, "report.pdf", wordRenderIfExistsError); err == nil {
		t.Fatal("error policy accepted an existing file")
	}
	if got, err := resolveWordRenderOutputPath(runtime, "report.pdf", wordRenderIfExistsOverwrite); err != nil || got != "report.pdf" {
		t.Fatalf("overwrite path = %q, err=%v", got, err)
	}
	if got, err := resolveWordRenderOutputPath(runtime, "report.pdf", wordRenderIfExistsRename); err != nil || got != "report (2).pdf" {
		t.Fatalf("rename path = %q, err=%v", got, err)
	}
}

func TestClampWordRenderPoll(t *testing.T) {
	for _, test := range []struct {
		input int64
		want  time.Duration
	}{
		{input: 0, want: 3 * time.Second},
		{input: 1, want: 500 * time.Millisecond},
		{input: 750, want: 750 * time.Millisecond},
		{input: 20_000, want: 10 * time.Second},
	} {
		if got := clampWordRenderPoll(test.input); got != test.want {
			t.Errorf("clampWordRenderPoll(%d) = %s, want %s", test.input, got, test.want)
		}
	}
}

func wordRenderAPIResponse(task map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{"task": task},
	}
}

func wordRenderSucceededTask(pdfSize int64) map[string]interface{} {
	pageIndex := 1
	pageNumber := 2
	return map[string]interface{}{
		"task_id": "render_status",
		"status":  "succeeded",
		"pdf": map[string]interface{}{
			"download_url":   wordRenderTestDownloadURL,
			"size":           pdfSize,
			"url_expires_at": 2_000_000_000,
		},
		"page_count": 2,
		"headings": []interface{}{
			map[string]interface{}{
				"heading_index": 0, "title": "Overview", "level": 1,
				"page_index": pageIndex, "page_number": pageNumber,
			},
		},
		"warnings": []interface{}{
			map[string]interface{}{"code": "heading_page_unresolved", "message": "one heading was unresolved", "heading_index": 1},
		},
	}
}

type capturedWordRenderMultipart struct {
	fields   map[string]string
	fileName string
	file     []byte
}

func decodeWordRenderMultipart(t *testing.T, stub *httpmock.Stub) capturedWordRenderMultipart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(stub.CapturedHeaders.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse multipart content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	result := capturedWordRenderMultipart{fields: make(map[string]string)}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart content: %v", err)
		}
		if part.FileName() != "" {
			result.fileName = part.FileName()
			result.file = content
		} else {
			result.fields[part.FormName()] = string(content)
		}
	}
	return result
}

func decodeWordRenderEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data: %#v", envelope)
	}
	return data
}

func assertWordRenderProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype, param string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != category || problem.Subtype != subtype {
		t.Fatalf("problem = %#v, error=%T %v", problem, err, err)
	}
	if param != "" {
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) || validationErr.Param != param {
			t.Fatalf("validation error = %#v, want param %q", validationErr, param)
		}
	}
}
