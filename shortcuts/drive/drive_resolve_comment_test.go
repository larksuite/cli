// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveResolveCommentExecute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "docx" {
				t.Errorf("file_type = %q, want docx", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := body["is_solved"]; got != true {
		t.Fatalf("request is_solved = %#v, want true", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "comment_id", "data.comment_id"); got != "comment_1" {
		t.Fatalf("comment_id = %q, want comment_1", got)
	}
	if got := mustStringField(t, data, "action", "data.action"); got != "resolve" {
		t.Fatalf("action = %q, want resolve", got)
	}
	if got := data["is_solved"]; got != true {
		t.Fatalf("is_solved = %#v, want true", got)
	}
	if got := data["updated"]; got != true {
		t.Fatalf("updated = %#v, want true", got)
	}
}

func TestDriveRestoreCommentExecuteViaWiki(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "docxFromWiki",
				},
			},
		},
	})
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/docxFromWiki/comments/comment_9",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveRestoreComment, []string{
		"+restore-comment",
		"--token", "wikiResource",
		"--type", "wiki",
		"--comment-id", "comment_9",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := body["is_solved"]; got != false {
		t.Fatalf("request is_solved = %#v, want false", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "action", "data.action"); got != "restore" {
		t.Fatalf("action = %q, want restore", got)
	}
	if got := data["is_solved"]; got != false {
		t.Fatalf("is_solved = %#v, want false", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

func TestDriveResolveCommentExecuteWikiResolvesToBitable(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "bitable",
					"obj_token": "baseFromWiki",
				},
			},
		},
	})
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/baseFromWiki/comments/comment_3",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "bitable" {
				t.Errorf("file_type = %q, want bitable", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_3",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "bitable" {
		t.Fatalf("file_type = %q, want bitable", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

func TestDriveCommentSolvedValidation(t *testing.T) {
	tests := []struct {
		name      string
		shortcut  string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name:     "unsafe comment id",
			shortcut: "resolve",
			args: []string{
				"+resolve-comment",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "../admin",
			},
			wantErr:   "path traversal",
			wantParam: "--comment-id",
		},
		{
			name:     "empty comment id",
			shortcut: "resolve",
			args: []string{
				"+resolve-comment",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "  ",
			},
			wantErr:   "must not be empty",
			wantParam: "--comment-id",
		},
		{
			name:     "restore rejects unsupported url type",
			shortcut: "restore",
			args: []string{
				"+restore-comment",
				"--url", "https://example.larksuite.com/drive/folder/folderResource",
				"--comment-id", "comment_1",
			},
			wantErr:   "comment restore supports doc, docx, sheet, file, slides, bitable, base, apps, wiki",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			shortcut := DriveResolveComment
			if tt.shortcut == "restore" {
				shortcut = DriveRestoreComment
			}
			err := mountAndRunDrive(t, shortcut, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestDriveResolveCommentInputConflict(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--token", "docxResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--url")
}

func TestDriveResolveCommentPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1",
		Body: map[string]interface{}{
			"code": 1069303,
			"msg":  "no comment permission",
		},
	})

	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "no comment permission") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveResolveCommentPropagatesWikiResolveError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 230005,
			"msg":  "wiki node not found",
		},
	})

	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "wiki node not found") {
		t.Fatalf("expected wiki resolve error to propagate, got %v", err)
	}
}

func TestDriveResolveCommentDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveResolveComment, []string{
		"+resolve-comment",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := dryRunDataMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "data.api")
	if len(api) != 2 {
		t.Fatalf("dry-run api call count = %d, want 2\nstdout:\n%s", len(api), stdout.String())
	}
	step2 := mustMapValue(t, api[1], "api[1]")
	if got := mustStringField(t, step2, "method", "api[1].method"); got != "PATCH" {
		t.Fatalf("api[1].method = %q, want PATCH", got)
	}
	body := mustMapValue(t, step2["body"], "api[1].body")
	if got := body["is_solved"]; got != true {
		t.Fatalf("api[1].body.is_solved = %#v, want true", got)
	}
}

func TestDriveRestoreCommentDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveRestoreComment, []string{
		"+restore-comment",
		"--url", "https://example.larksuite.com/sheets/sheetResource",
		"--comment-id", "comment_1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := dryRunDataMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "data.api")
	if len(api) != 1 {
		t.Fatalf("dry-run api call count = %d, want 1\nstdout:\n%s", len(api), stdout.String())
	}
	call := mustMapValue(t, api[0], "api[0]")
	if got := mustStringField(t, call, "method", "api[0].method"); got != "PATCH" {
		t.Fatalf("api[0].method = %q, want PATCH", got)
	}
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/sheetResource/comments/comment_1") {
		t.Fatalf("api[0].url = %q, want resolved file and comment tokens", got)
	}
	body := mustMapValue(t, call["body"], "api[0].body")
	if got := body["is_solved"]; got != false {
		t.Fatalf("api[0].body.is_solved = %#v, want false", got)
	}
}

func TestDriveRestoreCommentDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveRestoreComment, []string{
		"+restore-comment",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := dryRunDataMap(t, stdout.String())
	api := mustSliceValue(t, out["api"], "data.api")
	if len(api) != 2 {
		t.Fatalf("dry-run api call count = %d, want 2\nstdout:\n%s", len(api), stdout.String())
	}
	step2 := mustMapValue(t, api[1], "api[1]")
	body := mustMapValue(t, step2["body"], "api[1].body")
	if got := body["is_solved"]; got != false {
		t.Fatalf("api[1].body.is_solved = %#v, want false", got)
	}
}
