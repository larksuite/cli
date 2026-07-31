// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestNormalizeDriveCommentIDs(t *testing.T) {
	t.Parallel()

	got, err := normalizeDriveCommentIDs([]string{" c1 ", "c2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Fatalf("normalizeDriveCommentIDs = %v, want [c1 c2]", got)
	}

	if _, err := normalizeDriveCommentIDs(nil); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected at-least-one error, got %v", err)
	}
	if _, err := normalizeDriveCommentIDs([]string{"c1", "  "}); err == nil || !strings.Contains(err.Error(), "element #2 is empty") {
		t.Fatalf("expected empty-element error, got %v", err)
	}

	tooMany := make([]string, driveBatchQueryCommentsMaxIDs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("c%d", i)
	}
	_, err = normalizeDriveCommentIDs(tooMany)
	if err == nil || !strings.Contains(err.Error(), "at most 100") {
		t.Fatalf("expected max-IDs error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--comment-ids")
}

func TestDriveBatchQueryCommentsExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/batch_query",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "docx" {
				t.Errorf("file_type = %q, want docx", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"comment_id": "comment_1", "is_solved": false},
					{"comment_id": "comment_2", "is_solved": true},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-ids", "comment_1,comment_2",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	ids := mustSliceValue(t, body["comment_ids"], "request.comment_ids")
	if len(ids) != 2 || ids[0] != "comment_1" || ids[1] != "comment_2" {
		t.Fatalf("request comment_ids = %v, want [comment_1 comment_2]", ids)
	}
	if _, ok := body["need_reaction"]; ok {
		t.Fatalf("request should omit need_reaction by default: %v", body)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "docxResource" {
		t.Fatalf("file_token = %q, want docxResource", got)
	}
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "docx" {
		t.Fatalf("file_type = %q, want docx", got)
	}
	if got := data["count"]; got != float64(2) {
		t.Fatalf("count = %#v, want 2", got)
	}
	if _, ok := data["wiki_token"]; ok {
		t.Fatalf("wiki_token should be omitted for direct targets: %v", data)
	}
}

func TestDriveBatchQueryCommentsExecuteWikiWithReaction(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("token"); got != "wikiResource" {
				t.Errorf("wiki token = %q, want wikiResource", got)
			}
		},
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
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/sheetFromWiki/comments/batch_query",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "sheet" {
				t.Errorf("file_type = %q, want sheet", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"comment_id": "comment_1"}},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--token", "wikiResource",
		"--type", "wiki",
		"--comment-ids", "comment_1",
		"--need-reaction",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := body["need_reaction"]; got != true {
		t.Fatalf("request need_reaction = %#v, want true", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "sheetFromWiki" {
		t.Fatalf("file_token = %q, want sheetFromWiki", got)
	}
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "sheet" {
		t.Fatalf("file_type = %q, want sheet", got)
	}
	if got := mustStringField(t, data, "wiki_token", "data.wiki_token"); got != "wikiResource" {
		t.Fatalf("wiki_token = %q, want wikiResource", got)
	}
}

func TestDriveBatchQueryCommentsExecuteAppsPageURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/appsPageResource/comments/batch_query",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "apps" {
				t.Errorf("file_type = %q, want apps", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"comment_id": "comment_1"}},
			},
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.feishu.cn/page/appsPageResource/",
		"--comment-ids", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "apps" {
		t.Fatalf("file_type = %q, want apps", got)
	}
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "appsPageResource" {
		t.Fatalf("file_token = %q, want appsPageResource", got)
	}
}

func TestDriveBatchQueryCommentsExecuteBaseURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/baseResource/comments/batch_query",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "bitable" {
				t.Errorf("file_type = %q, want bitable", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []map[string]interface{}{{"comment_id": "comment_1"}},
			},
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/base/baseResource",
		"--comment-ids", "comment_1",
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
}

func TestDriveBatchQueryCommentsWikiResolvesToUnsupported(t *testing.T) {
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
					"obj_token": "mindnoteToken",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-ids", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), `wiki resolved to "mindnote"`) {
		t.Fatalf("expected wiki-resolution error, got %v", err)
	}
	assertDriveCommentValidationError(t, err, "--url")
}

func TestDriveBatchQueryCommentsExecuteBaseAliasType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/baseToken/comments/batch_query",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("file_type"); got != "bitable" {
				t.Errorf("file_type = %q, want bitable (base alias normalized)", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"items": []map[string]interface{}{}},
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--token", "baseToken",
		"--type", "base",
		"--comment-ids", "comment_1",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDriveBatchQueryCommentsValidation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name: "url and token mutually exclusive",
			args: []string{
				"+batch-query-comments",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--token", "docxResource",
				"--comment-ids", "comment_1",
			},
			wantErr:   "mutually exclusive",
			wantParam: "--url",
		},
		{
			name: "blank comment id element",
			args: []string{
				"+batch-query-comments",
				"--url", "https://example.larksuite.com/docx/docxResource",
				"--comment-ids", " ",
			},
			wantErr:   "element #1 is empty",
			wantParam: "--comment-ids",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
			err := mountAndRunDrive(t, DriveBatchQueryComments, append(tt.args, "--as", "user"), f, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, tt.wantParam)
		})
	}
}

func TestDriveBatchQueryCommentsPropagatesAPIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/batch_query",
		Body: map[string]interface{}{
			"code": 1069307,
			"msg":  "comment not found",
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-ids", "comment_404",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "comment not found") {
		t.Fatalf("expected API error to propagate, got %v", err)
	}
}

func TestDriveBatchQueryCommentsDryRunDirect(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-ids", "comment_1,comment_2",
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
	if got := mustStringField(t, call, "url", "api[0].url"); !strings.Contains(got, "/files/docxResource/comments/batch_query") {
		t.Fatalf("api[0].url = %q, want resolved batch_query URL", got)
	}
	body := mustMapValue(t, call["body"], "api[0].body")
	if got := body["need_reaction"]; got != true {
		t.Fatalf("api[0].body.need_reaction = %#v, want true", got)
	}
}

func TestDriveBatchQueryCommentsDryRunWiki(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-ids", "comment_1",
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
	body := mustMapValue(t, step2["body"], "api[1].body")
	ids := mustSliceValue(t, body["comment_ids"], "api[1].body.comment_ids")
	if len(ids) != 1 || ids[0] != "comment_1" {
		t.Fatalf("api[1].body.comment_ids = %v, want [comment_1]", ids)
	}
}

func TestDriveBatchQueryCommentsDryRunWikiNeedRelation(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/wiki/wikiResource",
		"--comment-ids", "comment_1",
		"--need-relation",
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
	if got := body["need_relation"]; got != "<sent only when obj_type is docx>" {
		t.Fatalf("api[1].body.need_relation = %#v, want conditional placeholder", got)
	}
}

func TestDriveBatchQueryCommentsOmittedItemsNormalized(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-ids", "comment_1",
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

func TestDriveBatchQueryCommentsNeedRelationDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/docxResource/comments/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--comment-ids", "comment_1",
		"--need-relation",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if got := body["need_relation"]; got != true {
		t.Fatalf("request need_relation = %#v, want true", got)
	}
}

func TestDriveBatchQueryCommentsNeedRelationIgnoredForNonDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/sheetResource/comments/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveBatchQueryComments, []string{
		"+batch-query-comments",
		"--url", "https://example.larksuite.com/sheets/sheetResource",
		"--comment-ids", "comment_1",
		"--need-relation",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to decode captured request body: %v", err)
	}
	if _, ok := body["need_relation"]; ok {
		t.Fatalf("need_relation must be omitted for non-docx targets: %v", body)
	}
}
