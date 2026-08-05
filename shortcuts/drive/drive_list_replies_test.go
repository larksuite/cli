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

func TestDriveListRepliesExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		OnMatch: func(req *http.Request) {
			query := req.URL.Query()
			if got := query.Get("file_type"); got != "docx" {
				t.Errorf("file_type = %q, want docx", got)
			}
			if got := query.Get("page_size"); got != "50" {
				t.Errorf("page_size = %q, want 50 (default)", got)
			}
			if query.Has("page_token") {
				t.Errorf("page_token should be omitted when not set, got %q", query.Get("page_token"))
			}
			if query.Has("need_reaction") {
				t.Errorf("need_reaction should be omitted when not set, got %q", query.Get("need_reaction"))
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"reply_id": "reply_1",
						"content": map[string]interface{}{
							"elements": []interface{}{
								map[string]interface{}{
									"type":     "text_run",
									"text_run": map[string]interface{}{"text": "根回复正文"},
								},
							},
						},
					},
					map[string]interface{}{"reply_id": "reply_2"},
				},
				"has_more":   true,
				"page_token": "next_page",
			},
		},
	})

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
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
	items := mustSliceValue(t, data["items"], "data.items")
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	firstItem := mustMapValue(t, items[0], "data.items[0]")
	if got := mustStringField(t, firstItem, "reply_id", "data.items[0].reply_id"); got != "reply_1" {
		t.Fatalf("items[0].reply_id = %q, want reply_1", got)
	}
	firstContent := mustMapValue(t, firstItem["content"], "data.items[0].content")
	firstElements := mustSliceValue(t, firstContent["elements"], "data.items[0].content.elements")
	firstElement := mustMapValue(t, firstElements[0], "data.items[0].content.elements[0]")
	firstText := mustMapValue(t, firstElement["text_run"], "data.items[0].content.elements[0].text_run")
	if got := mustStringField(t, firstText, "text", "data.items[0].content.elements[0].text_run.text"); got != "根回复正文" {
		t.Fatalf("items[0] text = %q, want 根回复正文", got)
	}
	secondItem := mustMapValue(t, items[1], "data.items[1]")
	if got := mustStringField(t, secondItem, "reply_id", "data.items[1].reply_id"); got != "reply_2" {
		t.Fatalf("items[1].reply_id = %q, want reply_2", got)
	}
	if got := data["count"]; got != float64(2) {
		t.Fatalf("count = %#v, want 2", got)
	}
	if got := data["has_more"]; got != true {
		t.Fatalf("has_more = %#v, want true", got)
	}
	if got := mustStringField(t, data, "page_token", "data.page_token"); got != "next_page" {
		t.Fatalf("page_token = %q, want next_page", got)
	}
}

func TestDriveListRepliesExecuteViaWikiToBitable(t *testing.T) {
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
					"obj_token": "bitableFromWiki",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/bitableFromWiki/comments/comment_1/replies",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "bitable" {
				t.Errorf("file_type = %q, want bitable", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"reply_id": "reply_1"}},
			},
		},
	})

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--token", "wikiResource",
		"--type", "wiki",
		"--comment-id", "comment_1",
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
	items := mustSliceValue(t, data["items"], "data.items")
	item := mustMapValue(t, items[0], "data.items[0]")
	if got := mustStringField(t, item, "reply_id", "data.items[0].reply_id"); got != "reply_1" {
		t.Fatalf("items[0].reply_id = %q, want reply_1", got)
	}
}

func TestDriveListRepliesPaginationAndReactionParams(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		OnMatch: func(req *http.Request) {
			query := req.URL.Query()
			if got := query.Get("page_size"); got != "10" {
				t.Errorf("page_size = %q, want 10", got)
			}
			if got := query.Get("page_token"); got != "cursor_1" {
				t.Errorf("page_token = %q, want cursor_1", got)
			}
			if got := query.Get("need_reaction"); got != "true" {
				t.Errorf("need_reaction = %q, want true", got)
			}
			if got := query.Get("user_id_type"); got != "" {
				t.Errorf("user_id_type = %q, want omitted (flag removed)", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	})

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--page-size", "10",
		"--page-token", "cursor_1",
		"--need-reaction",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := data["count"]; got != float64(0) {
		t.Fatalf("count = %#v, want 0", got)
	}
}

func TestDriveListRepliesValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name: "unsafe comment id",
			args: []string{
				"+list-replies",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "../admin",
			},
			wantErr:   "path traversal",
			wantParam: "--comment-id",
		},
		{
			name: "empty comment id",
			args: []string{
				"+list-replies",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", " ",
			},
			wantErr:   "--comment-id must not be empty",
			wantParam: "--comment-id",
		},
		{
			name: "page size too small",
			args: []string{
				"+list-replies",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--page-size", "0",
			},
			wantErr:   "--page-size must be between 1 and 100",
			wantParam: "--page-size",
		},
		{
			name: "page size too large",
			args: []string{
				"+list-replies",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-id", "comment_1",
				"--page-size", "101",
			},
			wantErr:   "--page-size must be between 1 and 100",
			wantParam: "--page-size",
		},
		{
			name: "unsupported url type",
			args: []string{
				"+list-replies",
				"--url", "https://example.larksuite.com/drive/folder/folderResource",
				"--comment-id", "comment_1",
			},
			wantErr:   "replies list supports doc, docx, sheet, file, slides, bitable, base, apps, wiki",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			err := mountAndRunDrive(t, DriveListReplies, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestDriveListRepliesPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		Body: map[string]interface{}{
			"code": 1069301,
			"msg":  "comment not found",
		},
	})

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "comment not found") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveListRepliesWikiNodeIncompleteResponse(t *testing.T) {
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

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "incomplete node data") {
		t.Fatalf("expected incomplete-node error, got %v", err)
	}
}

func TestDriveListRepliesDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--need-reaction",
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
	if got := mustStringField(t, call, "method", "api[0].method"); got != "GET" {
		t.Fatalf("api[0].method = %q, want GET", got)
	}
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/docxResource/comments/comment_1/replies") {
		t.Fatalf("api[0].url = %q, want resolved path segments", got)
	}
	params := mustMapValue(t, call["params"], "api[0].params")
	if got := params["need_reaction"]; got != true {
		t.Fatalf("api[0].params.need_reaction = %#v, want true", got)
	}
}

func TestDriveListRepliesDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
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
	step1 := mustMapValue(t, api[0], "api[0]")
	if got := mustStringField(t, step1, "url", "api[0].url"); !strings.Contains(got, "/wiki/v2/spaces/get_node") {
		t.Fatalf("api[0].url = %q, want wiki get_node", got)
	}
	step2 := mustMapValue(t, api[1], "api[1]")
	if got := mustStringField(t, step2, "method", "api[1].method"); got != "GET" {
		t.Fatalf("api[1].method = %q, want GET", got)
	}
	if got := mustStringField(t, step2, "url", "api[1].url"); !strings.Contains(got, "/comments/comment_1/replies") {
		t.Fatalf("api[1].url = %q, want resolved comment ID", got)
	}
}

func TestDriveListRepliesOmittedItemsNormalized(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"has_more": false, "page_token": ""},
		},
	})

	err := mountAndRunDrive(t, DriveListReplies, []string{
		"+list-replies",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	items, ok := data["items"].([]interface{})
	if !ok {
		t.Fatalf("items must be a JSON array even when the server omits it, got %#v", data["items"])
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
	if got := data["count"]; got != float64(0) {
		t.Fatalf("count = %#v, want 0", got)
	}
}
