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

func TestDriveReactReplyExecuteDocxAdd(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v2/files/docxResource/comments/reaction",
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

	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--reply-id", "reply_1",
		"--emoji", "THUMBSUP",
		"--action", "add",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := mustStringField(t, body, "action", "request.action"); got != "add" {
		t.Fatalf("request action = %q, want add", got)
	}
	if got := mustStringField(t, body, "reaction_type", "request.reaction_type"); got != "THUMBSUP" {
		t.Fatalf("request reaction_type = %q, want THUMBSUP", got)
	}
	if got := mustStringField(t, body, "reply_id", "request.reply_id"); got != "reply_1" {
		t.Fatalf("request reply_id = %q, want reply_1", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "reply_id", "data.reply_id"); got != "reply_1" {
		t.Fatalf("reply_id = %q, want reply_1", got)
	}
	if got := mustStringField(t, data, "reaction_type", "data.reaction_type"); got != "THUMBSUP" {
		t.Fatalf("reaction_type = %q, want THUMBSUP", got)
	}
	if got := mustStringField(t, data, "action", "data.action"); got != "add" {
		t.Fatalf("action = %q, want add", got)
	}
	if got := data["updated"]; got != true {
		t.Fatalf("updated = %#v, want true", got)
	}
}

func TestDriveReactReplyExecuteViaWikiDelete(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "slides",
					"obj_token": "slidesFromWiki",
				},
			},
		},
	})
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v2/files/slidesFromWiki/comments/reaction",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "slides" {
				t.Errorf("file_type = %q, want slides", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--token", "wikiResource",
		"--type", "wiki",
		"--reply-id", "reply_1",
		"--emoji", "ThumbsDown",
		"--action", "delete",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := mustStringField(t, body, "action", "request.action"); got != "delete" {
		t.Fatalf("request action = %q, want delete", got)
	}
	if got := mustStringField(t, body, "reaction_type", "request.reaction_type"); got != "ThumbsDown" {
		t.Fatalf("request reaction_type = %q, want ThumbsDown (case preserved)", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "slides" {
		t.Fatalf("file_type = %q, want slides", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

func TestDriveReactReplyValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name: "empty reply id",
			args: []string{
				"+react-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--reply-id", " ",
				"--emoji", "THUMBSUP",
				"--action", "add",
			},
			wantErr:   "--reply-id must not be empty",
			wantParam: "--reply-id",
		},
		{
			name: "unknown emoji",
			args: []string{
				"+react-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--reply-id", "reply_1",
				"--emoji", "FOOBAR",
				"--action", "add",
			},
			wantErr:   `unknown --emoji "FOOBAR"`,
			wantParam: "--emoji",
		},
		{
			name: "emoji is case sensitive",
			args: []string{
				"+react-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--reply-id", "reply_1",
				"--emoji", "thumbsup",
				"--action", "add",
			},
			wantErr:   `unknown --emoji "thumbsup"`,
			wantParam: "--emoji",
		},
		{
			name: "invalid action",
			args: []string{
				"+react-reply",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--reply-id", "reply_1",
				"--emoji", "THUMBSUP",
				"--action", "toggle",
			},
			wantErr:   `invalid value "toggle" for --action`,
			wantParam: "--action",
		},
		{
			name: "unsupported url type",
			args: []string{
				"+react-reply",
				"--url", "https://example.larksuite.com/drive/folder/folderResource",
				"--reply-id", "reply_1",
				"--emoji", "THUMBSUP",
				"--action", "add",
			},
			wantErr:   "reply reaction supports doc, docx, sheet, file, slides, bitable, base, apps, wiki",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			err := mountAndRunDrive(t, DriveReactReply, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestParseDriveReactReplyEmoji(t *testing.T) {
	t.Parallel()

	valid := []string{"THUMBSUP", "ThumbsDown", "Yes", "2021", " HEART "}
	for _, in := range valid {
		got, err := parseDriveReactReplyEmoji(in)
		if err != nil {
			t.Fatalf("parseDriveReactReplyEmoji(%q) unexpected error: %v", in, err)
		}
		if got != strings.TrimSpace(in) {
			t.Fatalf("parseDriveReactReplyEmoji(%q) = %q, want %q", in, got, strings.TrimSpace(in))
		}
	}

	for _, in := range []string{"", " ", "YES", "heart", "THUMBS_UP"} {
		if _, err := parseDriveReactReplyEmoji(in); err == nil {
			t.Fatalf("parseDriveReactReplyEmoji(%q) expected error, got nil", in)
		}
	}
}

func TestParseDriveReactReplyAction(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{"add": "add", " DELETE ": "delete", "Add": "add"} {
		got, err := parseDriveReactReplyAction(in)
		if err != nil {
			t.Fatalf("parseDriveReactReplyAction(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Fatalf("parseDriveReactReplyAction(%q) = %q, want %q", in, got, want)
		}
	}

	if _, err := parseDriveReactReplyAction("toggle"); err == nil {
		t.Fatal("parseDriveReactReplyAction(toggle) expected error, got nil")
	}
}

func TestDriveReactReplyPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v2/files/docxResource/comments/reaction",
		Body: map[string]interface{}{
			"code": 1069301,
			"msg":  "reply not found",
		},
	})

	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--reply-id", "reply_1",
		"--emoji", "THUMBSUP",
		"--action", "add",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "reply not found") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
	assertDriveCommentAPIError(t, err, 1069301)
}

func TestDriveReactReplyWikiNodeIncompleteResponse(t *testing.T) {
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

	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--reply-id", "reply_1",
		"--emoji", "THUMBSUP",
		"--action", "add",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "incomplete node data") {
		t.Fatalf("expected incomplete-node error, got %v", err)
	}
}

func TestDriveReactReplyDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--reply-id", "reply_1",
		"--emoji", "HEART",
		"--action", "add",
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
	if got := mustStringField(t, call, "method", "api[0].method"); got != "POST" {
		t.Fatalf("api[0].method = %q, want POST", got)
	}
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/drive/v2/files/docxResource/comments/reaction") {
		t.Fatalf("api[0].url = %q, want v2 reaction path", got)
	}
	body := mustMapValue(t, call["body"], "api[0].body")
	if got := mustStringField(t, body, "reaction_type", "api[0].body.reaction_type"); got != "HEART" {
		t.Fatalf("body.reaction_type = %q, want HEART", got)
	}
	if got := mustStringField(t, body, "action", "api[0].body.action"); got != "add" {
		t.Fatalf("body.action = %q, want add", got)
	}
	if got := mustStringField(t, body, "reply_id", "api[0].body.reply_id"); got != "reply_1" {
		t.Fatalf("body.reply_id = %q, want reply_1", got)
	}
}

func TestDriveReactReplyDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveReactReply, []string{
		"+react-reply",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--reply-id", "reply_1",
		"--emoji", "OK",
		"--action", "delete",
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
	if got := mustStringField(t, step2, "method", "api[1].method"); got != "POST" {
		t.Fatalf("api[1].method = %q, want POST", got)
	}
	if got := mustStringField(t, step2, "url", "api[1].url"); !strings.Contains(got, "/comments/reaction") {
		t.Fatalf("api[1].url = %q, want reaction path", got)
	}
	body := mustMapValue(t, step2["body"], "api[1].body")
	if got := mustStringField(t, body, "action", "api[1].body.action"); got != "delete" {
		t.Fatalf("api[1].body.action = %q, want delete", got)
	}
}
