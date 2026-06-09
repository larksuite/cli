// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// writeTestFile creates a file at name (relative to cwd) with size bytes of
// ASCII data and returns the relative path it wrote.
func writeTestFile(t *testing.T, name string, size int) string {
	t.Helper()
	if err := os.WriteFile(name, bytes.Repeat([]byte("a"), size), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", name, err)
	}
	return name
}

// writeSparseTestFile produces a sparse file of the requested size without
// allocating real disk space, useful for exercising the 50MB validation path.
func writeSparseTestFile(t *testing.T, name string, size int64) string {
	t.Helper()
	fh, err := os.Create(name)
	if err != nil {
		t.Fatalf("Create(%q) error: %v", name, err)
	}
	if err := fh.Truncate(size); err != nil {
		t.Fatalf("Truncate(%q, %d) error: %v", name, size, err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close(%q) error: %v", name, err)
	}
	return name
}

func TestUploadAttachmentTask_Success(t *testing.T) {
	for _, tt := range []struct {
		name     string
		format   string
		contains []string
	}{
		{
			name:   "pretty format",
			format: "pretty",
			contains: []string{
				"✅ Attachment uploaded successfully!",
				"Attachment GUID: att-guid-1",
			},
		},
		{
			name:   "json format",
			format: "json",
			contains: []string{
				`"guid": "att-guid-1"`,
				`"name": "note.txt"`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, stderr, reg := taskShortcutTestFactory(t)
			warmTenantToken(t, f, reg)

			dir := t.TempDir()
			cmdutil.TestChdir(t, dir)

			filePath := writeTestFile(t, "note.txt", 12)

			uploadStub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/task/v2/attachments/upload",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{
								"guid": "att-guid-1",
								"name": "note.txt",
								"size": 12,
							},
						},
					},
				},
			}
			reg.Register(uploadStub)

			args := []string{
				"+upload-attachment",
				"--resource-id", "task-guid-123",
				"--file", filePath,
				"--as", "bot",
				"--format", tt.format,
			}
			if err := runMountedTaskShortcut(t, UploadAttachmentTask, args, f, stdout); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			out := stdout.String()
			// Normalize JSON whitespace so that both compact and indented forms match.
			outNorm := strings.ReplaceAll(out, `":"`, `": "`)
			for _, want := range tt.contains {
				if !strings.Contains(outNorm, want) && !strings.Contains(out, want) {
					t.Errorf("stdout missing %q; got:\n%s", want, out)
				}
			}

			// Verify multipart body structure.
			body := decodeTaskAttachmentMultipart(t, uploadStub)
			if got := body.Fields["resource_type"]; got != "task" {
				t.Errorf("resource_type = %q, want %q", got, "task")
			}
			if got := body.Fields["resource_id"]; got != "task-guid-123" {
				t.Errorf("resource_id = %q, want %q", got, "task-guid-123")
			}
			if got, ok := body.Files["file"]; !ok {
				t.Errorf("multipart missing file part")
			} else if len(got) != 12 {
				t.Errorf("file size = %d, want 12", len(got))
			}
			if got := body.FileNames["file"]; got != "note.txt" {
				t.Errorf("multipart file filename = %q, want %q", got, "note.txt")
			}

			// Verify key observability logs on stderr.
			errOut := stderr.String()
			for _, log := range []string{
				"input parsed",
				"http call: POST /open-apis/task/v2/attachments/upload",
				"http response",
				"att-guid-1",
			} {
				if !strings.Contains(errOut, log) {
					t.Errorf("stderr missing log %q; got:\n%s", log, errOut)
				}
			}
		})
	}
}

func TestUploadAttachmentTask_ExplicitResourceTypePassthrough(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	filePath := writeTestFile(t, "note.txt", 5)

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/attachments/upload",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"guid": "att-guid-2"}},
			},
		},
	}
	reg.Register(uploadStub)

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--resource-type", "custom_type",
		"--file", filePath,
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeTaskAttachmentMultipart(t, uploadStub)
	if got := body.Fields["resource_type"]; got != "custom_type" {
		t.Fatalf("resource_type = %q, want custom_type", got)
	}
}

func TestUploadAttachmentTask_ResourceIDFromApplink(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	filePath := writeTestFile(t, "note.txt", 5)

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/attachments/upload",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"guid": "att-guid-3"}},
			},
		},
	}
	reg.Register(uploadStub)

	applink := "https://applink.feishu.cn/client/todo/task?guid=task-from-url"
	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", applink,
		"--file", filePath,
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeTaskAttachmentMultipart(t, uploadStub)
	if got := body.Fields["resource_id"]; got != "task-from-url" {
		t.Fatalf("resource_id = %q, want task-from-url", got)
	}
}

func TestUploadAttachmentTask_SizeLimit(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	// 50MB + 1 byte; no HTTP stub registered — we must fail before any call.
	filePath := writeSparseTestFile(t, "big.bin", 50*1024*1024+1)

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", filePath,
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "50MB") {
		t.Fatalf("error message should mention 50MB limit, got: %v", err)
	}
}

func TestUploadAttachmentTask_FileMissing(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", "does-not-exist.bin",
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if ve.Param != "--file" {
		t.Fatalf("param = %q, want %q", ve.Param, "--file")
	}
}

func TestUploadAttachmentTask_NotRegularFile(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	// A directory stats fine but is not a regular file; we must reject it as
	// invalid --file input before any HTTP call (no stub registered).
	if err := os.Mkdir("a-dir", 0o755); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", "a-dir",
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "--file" {
		t.Fatalf("param = %q, want %q", ve.Param, "--file")
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error message should mention regular file, got: %v", err)
	}
}

func TestUploadAttachmentTask_StatErrorMessage(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	// A missing path fails Stat → taskInputStatError surfaces "cannot access
	// file" (no longer "file not found"). No HTTP stub: must fail before any call.
	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", "missing.bin",
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "cannot access file") {
		t.Fatalf("error message should contain %q, got: %v", "cannot access file", err)
	}
}

func TestUploadAttachmentTask_MalformedResponse(t *testing.T) {
	f, stdout, stderr, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	filePath := writeTestFile(t, "note.txt", 4)

	// A 200 response whose body is not valid JSON: the parse failure must
	// surface a typed internal invalid_response error (exit 5), not a panic
	// or a silent success.
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/attachments/upload",
		RawBody: []byte("this is not json{"),
	})

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", filePath,
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Fatalf("exit code = %d, want %d", got, output.ExitInternal)
	}
	if !strings.Contains(err.Error(), "parse response") {
		t.Fatalf("error message should mention parse response, got: %v", err)
	}

	// The parse-error observability log should be emitted on stderr.
	if errOut := stderr.String(); !strings.Contains(errOut, "parse_error") {
		t.Errorf("stderr missing parse_error log; got:\n%s", errOut)
	}
}

func TestUploadAttachmentTask_APIError(t *testing.T) {
	f, stdout, stderr, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	filePath := writeTestFile(t, "note.txt", 3)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/task/v2/attachments/upload",
		Body: map[string]interface{}{
			"code": ErrCodeTaskPermissionDenied,
			"msg":  "no permission",
		},
	})

	err := runMountedTaskShortcut(t, UploadAttachmentTask, []string{
		"+upload-attachment",
		"--resource-id", "task-guid-123",
		"--file", filePath,
		"--as", "bot",
		"--format", "json",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf(err) = !ok, want typed errs.* error; err = %v", err)
	}
	if p.Subtype != errs.SubtypePermissionDenied {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypePermissionDenied)
	}
	if p.Code != ErrCodeTaskPermissionDenied {
		t.Fatalf("code = %d, want %d", p.Code, ErrCodeTaskPermissionDenied)
	}
	// permission_denied maps to CategoryAuthorization → exit 3 (was exit 1 under legacy).
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Fatalf("exit code = %d, want %d", got, output.ExitAuth)
	}

	// Key-path log should still be emitted on failure.
	errOut := stderr.String()
	for _, log := range []string{"input parsed", "http call", "http response"} {
		if !strings.Contains(errOut, log) {
			t.Errorf("stderr missing failure log %q; got:\n%s", log, errOut)
		}
	}
}

func TestUploadAttachmentTask_DryRun(t *testing.T) {
	for _, tt := range []struct {
		name             string
		extraArgs        []string
		wantResourceType string
	}{
		{
			name:             "default resource type",
			extraArgs:        nil,
			wantResourceType: "task",
		},
		{
			name:             "explicit resource type",
			extraArgs:        []string{"--resource-type", "custom_type"},
			wantResourceType: "custom_type",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := taskShortcutTestFactory(t)

			args := []string{
				"+upload-attachment",
				"--resource-id", "task-guid-123",
				"--file", "./some.pdf",
				"--as", "bot",
				"--format", "json",
				"--dry-run",
			}
			args = append(args, tt.extraArgs...)
			if err := runMountedTaskShortcut(t, UploadAttachmentTask, args, f, stdout); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			out := stdout.String()
			var dry map[string]interface{}
			if err := json.Unmarshal([]byte(out), &dry); err != nil {
				t.Fatalf("dry-run output is not JSON: %v\n%s", err, out)
			}
			calls, _ := dry["api"].([]interface{})
			if len(calls) != 1 {
				t.Fatalf("expected 1 api call in dry-run, got %d: %v", len(calls), calls)
			}
			call := calls[0].(map[string]interface{})
			if got := call["method"]; got != "POST" {
				t.Fatalf("method = %v, want POST", got)
			}
			if got := call["url"]; got != "/open-apis/task/v2/attachments/upload" {
				t.Fatalf("url = %v, want upload path", got)
			}
			params, _ := call["params"].(map[string]interface{})
			if got := params["user_id_type"]; got != "open_id" {
				t.Fatalf("params.user_id_type = %v, want open_id", got)
			}
			body := call["body"].(map[string]interface{})
			if got := body["resource_type"]; got != tt.wantResourceType {
				t.Fatalf("resource_type = %v, want %v", got, tt.wantResourceType)
			}
			if got := body["resource_id"]; got != "task-guid-123" {
				t.Fatalf("resource_id = %v, want task-guid-123", got)
			}
			fileDesc := body["file"].(map[string]interface{})
			if got := fileDesc["field"]; got != "file" {
				t.Fatalf("file.field = %v, want file", got)
			}
			if got := fileDesc["path"]; got != "./some.pdf" {
				t.Fatalf("file.path = %v, want ./some.pdf", got)
			}
			if got := fileDesc["name"]; got != "some.pdf" {
				t.Fatalf("file.name = %v, want some.pdf", got)
			}
		})
	}
}

// ── multipart body helper ──────────────────────────────────────────────────

type capturedAttachmentMultipart struct {
	Fields    map[string]string
	Files     map[string][]byte
	FileNames map[string]string
}

func decodeTaskAttachmentMultipart(t *testing.T, stub *httpmock.Stub) capturedAttachmentMultipart {
	t.Helper()
	contentType := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("content-type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	body := capturedAttachmentMultipart{
		Fields:    map[string]string{},
		Files:     map[string][]byte{},
		FileNames: map[string]string{},
	}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart data: %v", err)
		}
		if part.FileName() != "" {
			body.Files[part.FormName()] = data
			body.FileNames[part.FormName()] = part.FileName()
			continue
		}
		body.Fields[part.FormName()] = string(data)
	}
	return body
}
