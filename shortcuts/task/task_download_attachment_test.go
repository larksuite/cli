// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDownloadAttachmentTaskSuccess(t *testing.T) {
	factory, stdout, stderr, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	metadataStub := taskAttachmentMetadataStub("att-guid-1", "report.pdf", 7, "https://download.example/report?code=temporary")
	downloadStub := &httpmock.Stub{
		Method:      http.MethodGet,
		URL:         "https://download.example/report?code=temporary",
		RawBody:     []byte("PDFDATA"),
		ContentType: "application/pdf",
	}
	reg.Register(metadataStub)
	reg.Register(downloadStub)

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./downloads/",
		"--as", "bot",
		"--format", "json",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runMountedTaskShortcut() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "downloads", "report.pdf"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "PDFDATA" {
		t.Fatalf("downloaded content = %q, want PDFDATA", content)
	}
	if got := downloadStub.CapturedHeaders.Get("Authorization"); got != "" {
		t.Fatalf("download Authorization header = %q, want empty", got)
	}
	if output := stdout.String(); strings.Contains(output, "code=temporary") || !strings.Contains(output, `"attachment_guid": "att-guid-1"`) {
		t.Fatalf("stdout leaked temporary URL or omitted attachment guid:\n%s", output)
	}
	if output := stderr.String(); strings.Contains(output, "code=temporary") {
		t.Fatalf("stderr leaked temporary URL:\n%s", output)
	}
}

func TestDownloadAttachmentTaskRefreshesExpiredURLOnce(t *testing.T) {
	factory, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	reg.Register(taskAttachmentMetadataStub("att-guid-1", "note.txt", 4, "https://download.example/expired"))
	reg.Register(&httpmock.Stub{
		Method:  http.MethodGet,
		URL:     "https://download.example/expired",
		Status:  http.StatusForbidden,
		RawBody: []byte("expired"),
	})
	reg.Register(taskAttachmentMetadataStub("att-guid-1", "note.txt", 4, "https://download.example/fresh"))
	reg.Register(&httpmock.Stub{
		Method:      http.MethodGet,
		URL:         "https://download.example/fresh",
		RawBody:     []byte("DATA"),
		ContentType: "text/plain",
	})

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./note.txt",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runMountedTaskShortcut() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil || string(content) != "DATA" {
		t.Fatalf("downloaded content = %q, %v; want DATA", content, err)
	}
}

func TestDownloadAttachmentTaskRejectsOverwriteBeforeDownload(t *testing.T) {
	factory, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("report.pdf", []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg.Register(taskAttachmentMetadataStub("att-guid-1", "report.pdf", 7, "https://download.example/report"))
	downloadStub := &httpmock.Stub{
		Method:   http.MethodGet,
		URL:      "https://download.example/report",
		RawBody:  []byte("newdata"),
		Optional: true,
	}
	reg.Register(downloadStub)

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./report.pdf",
		"--as", "bot",
	}, factory, stdout)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeAlreadyExists || validationErr.Param != "--output" {
		t.Fatalf("error = %#v, want --output already_exists validation error", err)
	}
	if len(downloadStub.CapturedBodies) != 0 {
		t.Fatal("temporary download URL was consumed before overwrite validation")
	}
	content, readErr := os.ReadFile("report.pdf")
	if readErr != nil || string(content) != "existing" {
		t.Fatalf("existing file = %q, %v; want unchanged", content, readErr)
	}
}

func TestDownloadAttachmentTaskRejectsInsecureTemporaryURL(t *testing.T) {
	factory, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	reg.Register(taskAttachmentMetadataStub("att-guid-1", "note.txt", 4, "http://download.example/note"))

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./note.txt",
		"--as", "bot",
	}, factory, stdout)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, %v; want internal/invalid_response", problem, ok)
	}
}

func TestDownloadAttachmentTaskDoesNotLeakMalformedTemporaryURL(t *testing.T) {
	factory, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	reg.Register(taskAttachmentMetadataStub("att-guid-1", "note.txt", 4, "https://%secret-value"))

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./note.txt",
		"--as", "bot",
	}, factory, stdout)
	if err == nil {
		t.Fatal("error = nil, want invalid response")
	}
	if message := err.Error(); strings.Contains(message, "secret-value") {
		t.Fatalf("error leaked temporary URL: %q", message)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, %v; want internal/invalid_response", problem, ok)
	}
}

func TestTaskAttachmentFileName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "report.pdf", want: "report.pdf"},
		{name: "../report.pdf", want: "report.pdf"},
		{name: `..\\report.pdf`, want: "report.pdf"},
		{name: "..", want: "attachment"},
		{name: "bad\nname.txt", want: "attachment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskAttachmentFileName(tt.name); got != tt.want {
				t.Fatalf("taskAttachmentFileName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDownloadAttachmentTaskRejectsUnsafeOutput(t *testing.T) {
	factory, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, factory, reg)

	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "../escape.txt",
		"--as", "bot",
	}, factory, stdout)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--output" {
		t.Fatalf("error = %#v, want --output validation error", err)
	}
}

func TestDownloadAttachmentTaskDryRun(t *testing.T) {
	factory, stdout, _, _ := taskShortcutTestFactory(t)
	err := runMountedTaskShortcut(t, DownloadAttachmentTask, []string{
		"+download-attachment",
		"--attachment-guid", "att-guid-1",
		"--output", "./downloads/",
		"--user-id-type", "union_id",
		"--dry-run",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"method": "GET"`,
		`/open-apis/task/v2/attachments/att-guid-1`,
		`"user_id_type": "union_id"`,
		`temporary_attachment_url`,
		`"output": "./downloads/"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}
}

func taskAttachmentMetadataStub(guid, name string, size int64, downloadURL string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/task/v2/attachments/" + guid,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"attachment": map[string]interface{}{
					"guid":       guid,
					"file_token": "file-token-1",
					"name":       name,
					"size":       size,
					"url":        downloadURL,
				},
			},
		},
	}
}
