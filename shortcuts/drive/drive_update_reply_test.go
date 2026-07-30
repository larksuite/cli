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

func TestDriveUpdateReplyExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "PUT",
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
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"更新后的回复"},{"type":"mention_user","mention_user":"ou_123"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	content := mustMapValue(t, body["content"], "request.content")
	elements := mustSliceValue(t, content["elements"], "request.content.elements")
	if len(elements) != 2 {
		t.Fatalf("len(request.content.elements) = %d, want 2", len(elements))
	}
	first := mustMapValue(t, elements[0], "request.content.elements[0]")
	if got := mustStringField(t, first, "type", "request.content.elements[0].type"); got != "text_run" {
		t.Fatalf("request element type = %q, want text_run", got)
	}
	firstText := mustMapValue(t, first["text_run"], "request.content.elements[0].text_run")
	if got := mustStringField(t, firstText, "text", "request.content.elements[0].text_run.text"); got != "更新后的回复" {
		t.Fatalf("text_run.text = %q, want 更新后的回复", got)
	}
	second := mustMapValue(t, elements[1], "request.content.elements[1]")
	if got := mustStringField(t, second, "type", "request.content.elements[1].type"); got != "person" {
		t.Fatalf("request element type = %q, want person", got)
	}
	secondPerson := mustMapValue(t, second["person"], "request.content.elements[1].person")
	if got := mustStringField(t, secondPerson, "user_id", "request.content.elements[1].person.user_id"); got != "ou_123" {
		t.Fatalf("person.user_id = %q, want ou_123", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "comment_id", "data.comment_id"); got != "comment_1" {
		t.Fatalf("comment_id = %q, want comment_1", got)
	}
	if got := mustStringField(t, data, "reply_id", "data.reply_id"); got != "reply_2" {
		t.Fatalf("reply_id = %q, want reply_2", got)
	}
	if got := data["updated"]; got != true {
		t.Fatalf("updated = %#v, want true", got)
	}
}

func TestDriveUpdateReplyExecuteViaWiki(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": "sheetFromWiki",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/drive/v1/files/sheetFromWiki/comments/comment_1/replies/reply_2",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "sheet" {
				t.Errorf("file_type = %q, want sheet", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--token", "wikiResource",
		"--type", "wiki",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"updated from wiki"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "sheet" {
		t.Fatalf("file_type = %q, want sheet", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

// The reply-update endpoint does not declare the 100-element cap (only
// reply-create does), so +update-reply must NOT reject >100 elements locally.
func TestDriveUpdateReplyDoesNotCapElements(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	elems := make([]string, 101)
	for i := range elems {
		elems[i] = `{"type":"text","text":"x"}`
	}
	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", "[" + strings.Join(elems, ",") + "]",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("+update-reply must not cap element count locally, got %v", err)
	}
}

func TestDriveUpdateReplyValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name: "unsafe comment id",
			args: []string{
				"+update-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "../admin",
				"--reply-id", "reply_2",
				"--content", `[{"type":"text","text":"x"}]`,
			},
			wantErr:   "path traversal",
			wantParam: "--comment-id",
		},
		{
			name: "unsafe reply id",
			args: []string{
				"+update-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--reply-id", "../reply",
				"--content", `[{"type":"text","text":"x"}]`,
			},
			wantErr:   "path traversal",
			wantParam: "--reply-id",
		},
		{
			name: "empty reply id",
			args: []string{
				"+update-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--reply-id", " ",
				"--content", `[{"type":"text","text":"x"}]`,
			},
			wantErr:   "--reply-id must not be empty",
			wantParam: "--reply-id",
		},
		{
			name: "invalid content json",
			args: []string{
				"+update-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--reply-id", "reply_2",
				"--content", `not-json`,
			},
			wantErr:   "--content is not valid JSON",
			wantParam: "--content",
		},
		{
			name: "unsupported url type",
			args: []string{
				"+update-reply",
				"--url", "https://example.larksuite.com/drive/folder/folderResource",
				"--comment-id", "comment_1",
				"--reply-id", "reply_2",
				"--content", `[{"type":"text","text":"x"}]`,
			},
			wantErr:   "reply update supports doc, docx, sheet, file, slides, bitable, base, apps, wiki",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			err := mountAndRunDrive(t, DriveUpdateReply, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestDriveUpdateReplyPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies/reply_2",
		Body: map[string]interface{}{
			"code": 1069307,
			"msg":  "no permission to edit reply",
		},
	})

	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"x"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "no permission to edit reply") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveUpdateReplyWikiNodeIncompleteResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_token": "tokenOnly"},
			},
		},
	})

	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"x"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "incomplete node data") {
		t.Fatalf("expected incomplete-node error, got %v", err)
	}
}

func TestDriveUpdateReplyDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"更新后的回复"}]`,
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
	if got := mustStringField(t, call, "method", "api[0].method"); got != "PUT" {
		t.Fatalf("api[0].method = %q, want PUT", got)
	}
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/docxResource/comments/comment_1/replies/reply_2") {
		t.Fatalf("api[0].url = %q, want resolved path segments", got)
	}
	body := mustMapValue(t, call["body"], "api[0].body")
	content := mustMapValue(t, body["content"], "api[0].body.content")
	elements := mustSliceValue(t, content["elements"], "api[0].body.content.elements")
	if len(elements) != 1 {
		t.Fatalf("api[0].body.content.elements length = %d, want 1", len(elements))
	}
}

func TestDriveUpdateReplyDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveUpdateReply, []string{
		"+update-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--reply-id", "reply_2",
		"--content", `[{"type":"text","text":"x"}]`,
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
	step1 := mustMapValue(t, api[0], "api[0]")
	if got := mustStringField(t, step1, "url", "api[0].url"); !strings.Contains(got, "/wiki/v2/spaces/get_node") {
		t.Fatalf("api[0].url = %q, want wiki get_node", got)
	}
	step2 := mustMapValue(t, api[1], "api[1]")
	if got := mustStringField(t, step2, "method", "api[1].method"); got != "PUT" {
		t.Fatalf("api[1].method = %q, want PUT", got)
	}
	if got := mustStringField(t, step2, "url", "api[1].url"); !strings.Contains(got, "/comments/comment_1/replies/reply_2") {
		t.Fatalf("api[1].url = %q, want resolved comment and reply IDs", got)
	}
}
