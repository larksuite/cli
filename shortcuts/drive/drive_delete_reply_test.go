// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveDeleteReplyExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies/reply_2",
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
	})

	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--yes",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "comment_id", "data.comment_id"); got != "comment_1" {
		t.Fatalf("comment_id = %q, want comment_1", got)
	}
	if got := mustStringField(t, data, "reply_id", "data.reply_id"); got != "reply_2" {
		t.Fatalf("reply_id = %q, want reply_2", got)
	}
	if got := data["deleted"]; got != true {
		t.Fatalf("deleted = %#v, want true", got)
	}
}

func TestDriveDeleteReplyExecuteViaWiki(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "file",
					"obj_token": "fileFromWiki",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/fileFromWiki/comments/comment_1/replies/reply_2",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "file" {
				t.Errorf("file_type = %q, want file", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--token", "wikiResource",
		"--type", "wiki",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--yes",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "file" {
		t.Fatalf("file_type = %q, want file", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

func TestDriveDeleteReplyValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name: "unsafe reply id",
			args: []string{
				"+delete-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--reply-id", "../reply",
			},
			wantErr:   "path traversal",
			wantParam: "--reply-id",
		},
		{
			name: "empty reply id",
			args: []string{
				"+delete-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--reply-id", " ",
			},
			wantErr:   "--reply-id must not be empty",
			wantParam: "--reply-id",
		},
		{
			name: "unsafe comment id",
			args: []string{
				"+delete-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "../admin",
				"--reply-id", "reply_2",
			},
			wantErr:   "path traversal",
			wantParam: "--comment-id",
		},
		{
			name: "unsupported url type",
			args: []string{
				"+delete-reply",
				"--url", "https://example.larksuite.com/drive/folder/folderResource",
				"--comment-id", "comment_1",
				"--reply-id", "reply_2",
			},
			wantErr:   "reply delete supports doc, docx, sheet, file, slides, bitable, base, apps, wiki",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			err := mountAndRunDrive(t, DriveDeleteReply, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestDriveDeleteReplyPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies/reply_2",
		Body: map[string]interface{}{
			"code": 1069307,
			"msg":  "reply not found",
		},
	})

	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--yes",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "reply not found") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveDeleteReplyWikiNodeIncompleteResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "docx"},
			},
		},
	})

	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--yes",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "incomplete node data") {
		t.Fatalf("expected incomplete-node error, got %v", err)
	}
}

func TestDriveDeleteReplyDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
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
	if got := mustStringField(t, call, "method", "api[0].method"); got != "DELETE" {
		t.Fatalf("api[0].method = %q, want DELETE", got)
	}
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/docxResource/comments/comment_1/replies/reply_2") {
		t.Fatalf("api[0].url = %q, want resolved path segments", got)
	}
}

func TestDriveDeleteReplyDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveDeleteReply, []string{
		"+delete-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
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
	if got := mustStringField(t, step2, "method", "api[1].method"); got != "DELETE" {
		t.Fatalf("api[1].method = %q, want DELETE", got)
	}
	if got := mustStringField(t, step2, "url", "api[1].url"); !strings.Contains(got, "/comments/comment_1/replies/reply_2") {
		t.Fatalf("api[1].url = %q, want resolved comment and reply IDs", got)
	}
}
