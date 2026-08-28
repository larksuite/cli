// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func commentTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{AppID: "test-okr-comment", AppSecret: "secret-okr-comment", Brand: core.BrandFeishu}
}

func runCommentShortcut(t *testing.T, shortcut *common.Shortcut, args []string) (error, *bytes.Buffer) {
	t.Helper()
	f, stdout, _, _ := cmdutil.TestFactory(t, commentTestConfig(t))
	parent := &cobra.Command{Use: "okr"}
	shortcut.Mount(parent, f)
	args = append(args, "--as", "user")
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	return parent.Execute(), stdout
}

func runCommentWithFactory(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, shortcut *common.Shortcut, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	shortcut.Mount(parent, f)
	args = append(args, "--as", "user")
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.Execute()
}

func commentEnvelope(data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"code": 0, "msg": "ok", "data": data}
}

func commentItem(id, created string) map[string]interface{} {
	return map[string]interface{}{
		"id":             id,
		"target":         map[string]interface{}{"target_type": "objective", "target_id": "1"},
		"commentator_id": "ou_commentator", "status": "active",
		"create_time": created, "update_time": created,
		"content": map[string]interface{}{"blocks": []interface{}{}},
	}
}

func TestCommentValidationBranches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing target id", []string{"+comment-list", "--target-type", "cycle"}, "required"},
		{"bad target type", []string{"+comment-list", "--target-id", "1", "--target-type", "bad"}, "target-type"},
		{"bad target id", []string{"+comment-list", "--target-id", "0", "--target-type", "cycle"}, "positive int64"},
		{"bad user id type", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--user-id-type", "bad"}, "user-id-type"},
		{"bad style", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--style", "bad"}, "style"},
		{"bad page size", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--page-size", "101"}, "page-size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err, _ := runCommentShortcut(t, &OKRListComments, tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	for _, target := range []string{"cycle", "progress", "objective", "key_result"} {
		args := []string{"+comment-create", "--target-id", "1", "--target-type", target, "--content", "{\"text\":\"x\"}"}
		if target == "objective" || target == "key_result" {
			args = append(args, "--selected-text", "x")
		}
		args = append(args, "--dry-run")
		if err, _ := runCommentShortcut(t, &OKRCreateComment, args); err != nil {
			t.Fatalf("target %s: %v", target, err)
		}
	}
	invalid := [][]string{
		{"--select-all", "--selected-text", "x"},
		{"--ref-comment-id", "bad"},
		{"--target-type", "objective"},
		{"--target-type", "cycle", "--selected-text", "x"},
		{"--target-type", "cycle", "--select-all"},
	}
	for i, extra := range invalid {
		args := []string{"+comment-create", "--target-id", "1", "--target-type", "objective", "--content", "{\"text\":\"x\"}"}
		args = append(args, extra...)
		if err, _ := runCommentShortcut(t, &OKRCreateComment, args); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestCommentContentValidationBranches(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, content, style, want string }{
		{"missing", "", "simple", "required"},
		{"bad simple json", "not-json", "simple", "semi-plain JSON"},
		{"empty simple text", "{\"text\":\"  \"}", "simple", "cannot be empty"},
		{"simple docs", "{\"text\":\"x\",\"docs\":[{\"url\":\"u\"}]}", "simple", "docs and images"},
		{"bad richtext json", "not-json", "richtext", "ContentBlock JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shortcut := &OKRCreateComment
			args := []string{"+comment-create", "--target-id", "1", "--target-type", "cycle", "--content", tc.content, "--style", tc.style}
			if tc.name == "missing" {
				shortcut = &OKRPatchComment
				args = []string{"+comment-patch", "--comment-id", "1", "--style", tc.style}
			}
			err, _ := runCommentShortcut(t, shortcut, args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestCommentListExecutePaginationAndThreads(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
	var queries []url.Values
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/comments", OnMatch: func(r *http.Request) { queries = append(queries, r.URL.Query()) }, Reusable: true, Body: commentEnvelope(map[string]interface{}{"items": []interface{}{commentItem("2", "20"), commentItem("1", "10")}, "has_more": false, "page_token": "next"})})
	err := runCommentWithFactory(t, f, stdout, &OKRListComments, []string{"+comment-list", "--target-id", "1", "--target-type", "objective", "--page-size", "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queries) != 1 || queries[0].Get("page_size") != "2" {
		t.Fatalf("queries = %#v", queries)
	}
	data := decodeEnvelope(t, stdout)
	threads, ok := data["comments"].([]interface{})
	if !ok || len(threads) != 2 || len(threads[0].([]interface{})) != 1 || len(threads[1].([]interface{})) != 1 {
		t.Fatalf("comments = %#v", data["comments"])
	}
}

func TestCommentGetPatchExecute(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		shortcut     *common.Shortcut
		args         []string
		method, path string
	}{
		{"get", &OKRGetComment, []string{"+comment-get", "--comment-id", "9"}, "GET", "/open-apis/okr/v2/comments/9"},
		{"patch", &OKRPatchComment, []string{"+comment-patch", "--comment-id", "9", "--content", "{\"text\":\"updated\"}"}, "PATCH", "/open-apis/okr/v2/comments/9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
			reg.Register(&httpmock.Stub{Method: tc.method, URL: tc.path, Body: commentEnvelope(map[string]interface{}{"comment": commentItem("9", "1735776000000")})})
			if err := runCommentWithFactory(t, f, stdout, tc.shortcut, tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(stdout.String(), "9") {
				t.Fatalf("output = %s", stdout.String())
			}
		})
	}
}

func TestCommentCreateSolveReopenDeleteExecute(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		shortcut     *common.Shortcut
		args         []string
		method, path string
		response     map[string]interface{}
	}{
		{"create", &OKRCreateComment, []string{"+comment-create", "--target-type", "objective", "--target-id", "1", "--content", "{\"text\":\"hello\"}", "--selected-text", "he"}, "POST", "/open-apis/okr/v2/comments", map[string]interface{}{"comment_id": "10", "selection_id": "s1"}},
		{"solve", &OKRSolveComment, []string{"+comment-solve", "--comment-id", "10"}, "POST", "/open-apis/okr/v2/comments/10/solve", map[string]interface{}{"affected_comments": []interface{}{commentItem("10", "10")}}},
		{"reopen", &OKRReopenComment, []string{"+comment-reopen", "--comment-id", "10"}, "POST", "/open-apis/okr/v2/comments/10/reopen", map[string]interface{}{"affected_comments": []interface{}{}}},
		{"delete", &OKRDeleteComment, []string{"+comment-delete", "--comment-id", "10", "--yes"}, "DELETE", "/open-apis/okr/v2/comments/10", map[string]interface{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
			reg.Register(&httpmock.Stub{Method: tc.method, URL: tc.path, Body: commentEnvelope(tc.response)})
			if err := runCommentWithFactory(t, f, stdout, tc.shortcut, tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCommentExecuteAPIErrorAndMalformedResponse(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/comments/500", Status: 500, Body: map[string]interface{}{"code": 500, "msg": "error"}})
	if err := runCommentWithFactory(t, f, stdout, &OKRGetComment, []string{"+comment-get", "--comment-id", "500"}); err == nil {
		t.Fatal("expected API error")
	}
	f, stdout, _, reg = cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/comments/501", Body: commentEnvelope(map[string]interface{}{})})
	if err := runCommentWithFactory(t, f, stdout, &OKRGetComment, []string{"+comment-get", "--comment-id", "501"}); err == nil {
		t.Fatal("expected missing comment error")
	}
}

func TestCommentResponseParsingErrors(t *testing.T) {
	if _, err := parseComment(make(chan int)); err == nil {
		t.Fatal("parseComment should reject values that cannot be marshaled")
	}
	if _, _, _, err := commentListResponse(map[string]interface{}{"items": []interface{}{make(chan int)}}); err == nil {
		t.Fatal("commentListResponse should reject malformed items")
	}
	if _, err := commentResponse(map[string]interface{}{}); err == nil {
		t.Fatal("commentResponse should reject a missing comment")
	}
}

func TestCommentDetailTargetsAndExecute(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/cycles/1/objectives", Body: commentEnvelope(map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "o1"}}})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/objectives/o1/key_results", Body: commentEnvelope(map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "k1"}}})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/objectives/o1/progresses", Body: commentEnvelope(map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "p1"}}})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/key_results/k1/progresses", Body: commentEnvelope(map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "p2"}}})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/comments", Reusable: true, Body: commentEnvelope(map[string]interface{}{"items": []interface{}{}, "has_more": false})})
	if err := runCommentWithFactory(t, f, stdout, &OKRCycleCommentDetail, []string{"+comment-detail", "--cycle-id", "1"}); err != nil {
		t.Fatalf("unexpected detail error: %v", err)
	}
	if !strings.Contains(stdout.String(), "cycle_id") {
		t.Fatalf("output = %s", stdout.String())
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["data"] == nil {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestCommentDetailFetchCommentsPagination(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/cycles/2/objectives",
		Body: commentEnvelope(map[string]interface{}{"items": []interface{}{}, "has_more": false}),
	})
	var queries []url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/comments",
		OnMatch: func(r *http.Request) { queries = append(queries, r.URL.Query()) },
		Body: commentEnvelope(map[string]interface{}{
			"items": []interface{}{commentItem("1", "10")}, "has_more": true, "page_token": "comment-next",
		}),
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/comments",
		OnMatch: func(r *http.Request) { queries = append(queries, r.URL.Query()) },
		Body: commentEnvelope(map[string]interface{}{
			"items": []interface{}{commentItem("2", "20")}, "has_more": false,
		}),
	})
	if err := runCommentWithFactory(t, f, stdout, &OKRCycleCommentDetail, []string{"+comment-detail", "--cycle-id", "2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queries) != 2 || queries[1].Get("page_token") != "comment-next" {
		t.Fatalf("queries = %#v", queries)
	}
}

func TestCommentDetailProgressPaginationAndFailure(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/cycles/3/objectives",
		Body: commentEnvelope(map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "o3"}}, "has_more": false}),
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/objectives/o3/key_results",
		Body: commentEnvelope(map[string]interface{}{"items": []interface{}{}, "has_more": false}),
	})
	var queries []url.Values
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/objectives/o3/progresses",
		OnMatch: func(r *http.Request) { queries = append(queries, r.URL.Query()) },
		Body: commentEnvelope(map[string]interface{}{
			"items": []interface{}{map[string]interface{}{"id": "p1"}}, "has_more": true, "page_token": "progress-next",
		}),
	})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/okr/v2/objectives/o3/progresses",
		OnMatch: func(r *http.Request) { queries = append(queries, r.URL.Query()) },
		Body: commentEnvelope(map[string]interface{}{
			"items": []interface{}{map[string]interface{}{"id": "p2"}}, "has_more": false,
		}),
	})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/comments", Reusable: true, Body: commentEnvelope(map[string]interface{}{"items": []interface{}{}, "has_more": false})})
	if err := runCommentWithFactory(t, f, stdout, &OKRCycleCommentDetail, []string{"+comment-detail", "--cycle-id", "3"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queries) != 2 || queries[1].Get("page_token") != "progress-next" {
		t.Fatalf("queries = %#v", queries)
	}

	f, _, _, reg = cmdutil.TestFactory(t, commentTestConfig(t))
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/okr/v2/cycles/4/objectives", Status: 500, Body: map[string]interface{}{"code": 500, "msg": "error"}})
	if err := runCommentWithFactory(t, f, stdout, &OKRCycleCommentDetail, []string{"+comment-detail", "--cycle-id", "4"}); err == nil {
		t.Fatal("expected cycle detail API error")
	}
}

func TestCommentValidationSelectionModes(t *testing.T) {
	args := []string{"+comment-create", "--target-id", "1", "--target-type", "objective", "--content", "{\"text\":\"x\"}"}
	err, _ := runCommentShortcut(t, &OKRCreateComment, args)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error = %v", err)
	}
}

func TestCommentThreadsGroupSelectionAndSort(t *testing.T) {
	t.Parallel()
	selection := "77"
	threads := groupCommentThreads([]Comment{
		{ID: "2", CreateTime: "20", Selection: &CommentSelection{ID: selection}},
		{ID: "1", CreateTime: "10", Selection: &CommentSelection{ID: selection}},
		{ID: "3", CreateTime: "15"},
	})
	if len(threads) != 2 || threads[0][0].ID != "1" || threads[0][1].ID != "2" || threads[1][0].ID != "3" {
		t.Fatalf("threads = %#v", threads)
	}
	tie := groupCommentThreads([]Comment{{ID: "b", CreateTime: "10"}, {ID: "a", CreateTime: "10"}})
	if tie[0][0].ID != "a" || tie[1][0].ID != "b" {
		t.Fatalf("tie ordering = %#v", tie)
	}
}

func TestCommentThreadOutputUsesDetailShape(t *testing.T) {
	threads := commentThreadOutput("simple", []Comment{
		{ID: "2", CreateTime: "20", Selection: &CommentSelection{ID: "77"}, Content: &ContentBlock{}},
		{ID: "1", CreateTime: "10", Selection: &CommentSelection{ID: "77"}, Content: &ContentBlock{}},
		{ID: "3", CreateTime: "15", Content: &ContentBlock{}},
	})
	if len(threads) != 2 || len(threads[0]) != 2 || threads[0][0].ID != "1" || threads[0][1].ID != "2" || len(threads[1]) != 1 || threads[1][0].ID != "3" {
		t.Fatalf("threads = %#v", threads)
	}
}

func TestCommentCreateDryRunUsesWildcardSelection(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, commentTestConfig(t))
	parent := &cobra.Command{Use: "okr"}
	OKRCreateComment.Mount(parent, f)
	parent.SetArgs([]string{
		"+comment-create",
		"--target-type", "objective",
		"--target-id", "1",
		"--content", "{\"text\":\"hello\"}",
		"--select-all",
		"--as", "user",
		"--dry-run",
	})
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "\"selected_text\": \"*****\"") {
		t.Fatalf("dry-run should use wildcard selection, got: %s", output)
	}
	if strings.Contains(output, "department_id_type") {
		t.Fatalf("dry-run must omit department_id_type, got: %s", output)
	}
}

func TestCommentReopenDryRunUsesReopenPath(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, commentTestConfig(t))
	parent := &cobra.Command{Use: "okr"}
	OKRReopenComment.Mount(parent, f)
	parent.SetArgs([]string{"+comment-reopen", "--comment-id", "2", "--as", "user", "--dry-run"})
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/comments/2/reopen") {
		t.Fatalf("dry-run should use reopen endpoint, got: %s", output)
	}
	if strings.Contains(output, "department_id_type") {
		t.Fatalf("dry-run must omit department_id_type, got: %s", output)
	}
}
