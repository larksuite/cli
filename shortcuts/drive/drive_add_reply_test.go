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

func TestDriveReplyV1Elements(t *testing.T) {
	t.Parallel()

	elements, err := parseCommentReplyElements(`[
		{"type":"text","text":"a<b"},
		{"type":"mention_user","mention_user":"ou_123"},
		{"type":"link","link":"https://example.com"}
	]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := driveReplyV1Elements(elements)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0]["type"] != "text_run" {
		t.Fatalf("elements[0].type = %#v, want text_run", got[0]["type"])
	}
	textRun, ok := got[0]["text_run"].(map[string]interface{})
	if !ok {
		t.Fatalf("elements[0].text_run is %T, want map", got[0]["text_run"])
	}
	if textRun["text"] != "a&lt;b" {
		t.Fatalf("elements[0].text_run.text = %#v, want escaped a&lt;b", textRun["text"])
	}
	person, ok := got[1]["person"].(map[string]interface{})
	if !ok || got[1]["type"] != "person" {
		t.Fatalf("elements[1] = %#v, want person element", got[1])
	}
	if person["user_id"] != "ou_123" {
		t.Fatalf("elements[1].person.user_id = %#v, want ou_123", person["user_id"])
	}
	docsLink, ok := got[2]["docs_link"].(map[string]interface{})
	if !ok || got[2]["type"] != "docs_link" {
		t.Fatalf("elements[2] = %#v, want docs_link element", got[2])
	}
	if docsLink["url"] != "https://example.com" {
		t.Fatalf("elements[2].docs_link.url = %#v, want https://example.com", docsLink["url"])
	}
}

func TestDriveAddReplyExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "docx" {
				t.Errorf("file_type = %q, want docx", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"reply": map[string]interface{}{
					"reply_id": "reply_9",
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"收到，我来处理"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if _, ok := body["comment_id"]; ok {
		t.Fatalf("request body must not carry comment_id (it rides in the URL path): %v", body)
	}
	content := mustMapValue(t, body["content"], "request.content")
	elements := mustSliceValue(t, content["elements"], "request.content.elements")
	element := mustMapValue(t, elements[0], "request.content.elements[0]")
	if got := mustStringField(t, element, "type", "request.content.elements[0].type"); got != "text_run" {
		t.Fatalf("request element type = %q, want text_run", got)
	}
	elementText := mustMapValue(t, element["text_run"], "request.content.elements[0].text_run")
	if got := mustStringField(t, elementText, "text", "request.content.elements[0].text_run.text"); got != "收到，我来处理" {
		t.Fatalf("text_run.text = %q, want 收到，我来处理", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "comment_id", "data.comment_id"); got != "comment_1" {
		t.Fatalf("comment_id = %q, want comment_1", got)
	}
	if got := mustStringField(t, data, "reply_id", "data.reply_id"); got != "reply_9" {
		t.Fatalf("reply_id = %q, want reply_9", got)
	}
	if got := data["created"]; got != true {
		t.Fatalf("created = %#v, want true", got)
	}
}

func TestDriveAddReplyExecuteWikiResolvesToDocx(t *testing.T) {
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxFromWiki/comments/comment_1/replies",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply from wiki"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "docxFromWiki" {
		t.Fatalf("file_token = %q, want docxFromWiki", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
	if _, ok := data["reply_id"]; ok {
		t.Fatalf("reply_id should be omitted when the response carries none: %v", data)
	}
}

func TestDriveAddReplyRejectsUnsupportedTargets(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/drive/folder/folderResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), `unsupported --url resource type "folder"`) {
		t.Fatalf("expected unsupported-type error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--url")
}

func TestDriveAddReplyWikiResolvesToUnsupported(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "mindnote",
					"obj_token": "mindnoteFromWiki",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), `wiki resolved to "mindnote", but comment reply only supports`) {
		t.Fatalf("expected wiki-resolution error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--url")
}

func TestExtractDriveCreatedReplyID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{name: "nil data", data: nil, want: ""},
		{name: "top-level reply_id", data: map[string]interface{}{"reply_id": "r1"}, want: "r1"},
		{name: "nested reply object", data: map[string]interface{}{"reply": map[string]interface{}{"reply_id": "r2"}}, want: "r2"},
		{name: "nested reply without id falls through", data: map[string]interface{}{"reply": map[string]interface{}{}}, want: ""},
		{
			name: "reply_list wrapper",
			data: map[string]interface{}{"reply_list": map[string]interface{}{"replies": []interface{}{
				"not-a-map",
				map[string]interface{}{"reply_id": ""},
				map[string]interface{}{"reply_id": "r3"},
			}}},
			want: "r3",
		},
		{name: "reply_list without match", data: map[string]interface{}{"reply_list": map[string]interface{}{"replies": []interface{}{map[string]interface{}{}}}}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractDriveCreatedReplyID(tt.data); got != tt.want {
				t.Fatalf("extractDriveCreatedReplyID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDriveAddReplyRejectsUnsafeCommentID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "../admin",
		"--content", `[{"type":"text","text":"reply"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected comment-id validation error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--comment-id")
}

func TestDriveAddReplyPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/comment_1/replies",
		Body: map[string]interface{}{
			"code": 1069307,
			"msg":  "comment not found",
		},
	})

	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply"}]`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "comment not found") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveAddReplyDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply"}]`,
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
	if got := mustStringField(t, step2, "url", "api[1].url"); !strings.Contains(got, "/comments/comment_1/replies") {
		t.Fatalf("api[1].url = %q, want replies URL with comment ID", got)
	}
}

func TestDriveAddReplyInvalidContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", `not-json`,
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--content is not valid JSON") {
		t.Fatalf("expected content JSON error, got %v", err)
	}
}

func TestDriveAddReplyRejectsTooManyElements(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	elems := make([]string, 101)
	for i := range elems {
		elems[i] = `{"type":"text","text":"x"}`
	}
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", "[" + strings.Join(elems, ",") + "]",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "caps content.elements at 100") {
		t.Fatalf("expected 100-element cap error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--content")
}

func TestDriveAddReplyAcceptsMaxElements(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	elems := make([]string, 100)
	for i := range elems {
		elems[i] = `{"type":"text","text":"x"}`
	}
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", "[" + strings.Join(elems, ",") + "]",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("100 elements should be accepted, got %v", err)
	}
}

func TestDriveAddReplyDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveAddReply, []string{
		"+add-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-id", "comment_1",
		"--content", `[{"type":"text","text":"reply"}]`,
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
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/docxResource/comments/comment_1/replies") {
		t.Fatalf("api[0].url = %q, want reply create URL with comment ID", got)
	}
	body := mustMapValue(t, call["body"], "api[0].body")
	if _, ok := body["comment_id"]; ok {
		t.Fatalf("api[0].body must not carry comment_id: %v", body)
	}
	content := mustMapValue(t, body["content"], "api[0].body.content")
	if _, ok := content["elements"]; !ok {
		t.Fatalf("api[0].body.content.elements missing: %v", body)
	}
}
