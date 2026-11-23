// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"
)

type commentTestRoundTripper func(*http.Request) (*http.Response, error)

func (f commentTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

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

func requireCommentValidationError(t *testing.T, err error, param string) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T (%v), want *errs.ValidationError", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != param {
		t.Fatalf("Param = %q, want %q", ve.Param, param)
	}
	return ve
}

func TestCommentValidationBranches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, param string
		args        []string
	}{
		{"bad target type", "--target-type", []string{"+comment-list", "--target-id", "1", "--target-type", "bad"}},
		{"bad target id", "--target-id", []string{"+comment-list", "--target-id", "0", "--target-type", "cycle"}},
		{"bad user id type", "--user-id-type", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--user-id-type", "bad"}},
		{"bad style", "--style", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--style", "bad"}},
		{"bad page size", "--page-size", []string{"+comment-list", "--target-id", "1", "--target-type", "cycle", "--page-size", "101"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err, _ := runCommentShortcut(t, &OKRListComments, tc.args)
			requireCommentValidationError(t, err, tc.param)
		})
	}
	if err, _ := runCommentShortcut(t, &OKRListComments, []string{"+comment-list", "--target-type", "cycle"}); err == nil {
		t.Fatalf("missing required target-id error = %v", err)
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
		{"--selected-text", "x", "--ref-comment-id", "1"},
		{"--select-all", "--ref-comment-id", "1"},
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
	cases := []struct {
		name, command, content, style, param string
		shortcut                             *common.Shortcut
		wantCause                            bool
	}{
		{"bad simple json", "+comment-create", "not-json", "simple", "--content", &OKRCreateComment, true},
		{"missing create content", "+comment-create", "", "simple", "--content", &OKRCreateComment, false},
		{"missing patch content", "+comment-patch", "", "simple", "--content", &OKRPatchComment, false},
		{"empty simple text", "+comment-create", "{\"text\":\"  \"}", "simple", "--content", &OKRCreateComment, false},
		{"simple docs", "+comment-create", "{\"text\":\"x\",\"docs\":[{\"url\":\"u\"}]}", "simple", "--content", &OKRCreateComment, false},
		{"bad richtext json", "+comment-create", "not-json", "richtext", "--content", &OKRCreateComment, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{tc.command, "--content", tc.content, "--style", tc.style}
			if tc.shortcut == &OKRCreateComment {
				args = append(args, "--target-id", "1", "--target-type", "cycle")
			} else {
				args = append(args, "--comment-id", "1")
			}
			err, _ := runCommentShortcut(t, tc.shortcut, args)
			ve := requireCommentValidationError(t, err, tc.param)
			if tc.wantCause {
				var syntaxErr *json.SyntaxError
				if ve.Cause == nil || !errors.As(err, &syntaxErr) {
					t.Fatal("expected the original JSON parse cause to be preserved through the error chain")
				}
			}
		})
	}
	if err, _ := runCommentShortcut(t, &OKRPatchComment, []string{"+comment-patch", "--comment-id", "1", "--style", "simple"}); err == nil {
		t.Fatalf("missing required content error = %v", err)
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

func TestDecodeCommentProgressPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		data      map[string]interface{}
		wantItems int
		wantError bool
	}{
		{name: "items", data: map[string]interface{}{"items": []interface{}{map[string]interface{}{"id": "p1"}}}, wantItems: 1},
		{name: "empty items", data: map[string]interface{}{"items": []interface{}{}}, wantItems: 0},
		{name: "missing items", data: map[string]interface{}{}, wantError: true},
		{name: "wrong items shape", data: map[string]interface{}{"items": "not-an-array"}, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items, err := decodeCommentProgressPage(tc.data)
			if !tc.wantError {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(items) != tc.wantItems {
					t.Fatalf("items = %#v, want %d items", items, tc.wantItems)
				}
				return
			}
			if err == nil {
				t.Fatal("expected invalid response error")
			}
			var internalErr *errs.InternalError
			if !errors.As(err, &internalErr) {
				t.Fatalf("error = %T (%v), want *errs.InternalError", err, err)
			}
			if internalErr.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
			}
		})
	}
	if _, err := decodeCommentProgressPage(map[string]interface{}{"items": make(chan int)}); err == nil {
		t.Fatal("expected marshal failure")
	} else {
		var internalErr *errs.InternalError
		if !errors.As(err, &internalErr) || internalErr.Cause == nil {
			t.Fatalf("marshal error = %T (%v), cause was not preserved", err, err)
		}
	}
}

func TestCommentAPIWithContextHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	cfg := commentTestConfig(t)
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	factory.LarkClient = func() (*lark.Client, error) {
		return lark.NewClient(cfg.AppID, cfg.AppSecret,
			lark.WithEnableTokenCache(false),
			lark.WithLogLevel(larkcore.LogLevelError),
			lark.WithHttpClient(&http.Client{Transport: commentTestRoundTripper(func(req *http.Request) (*http.Response, error) {
				close(started)
				<-req.Context().Done()
				return nil, req.Context().Err()
			})}),
		), nil
	}
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+comment-detail"}, cfg, factory, core.AsUser)
	callCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := commentAPIWithContext(callCtx, runtime, "GET", "/open-apis/okr/v2/comments", nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %T (%v), want context.Canceled cause", err, err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request did not return promptly")
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
	requireCommentValidationError(t, err, "--selected-text")
}

func TestCommentActionValidationUserIDType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		shortcut *common.Shortcut
		command  string
	}{
		{name: "solve", shortcut: &OKRSolveComment, command: "+comment-solve"},
		{name: "reopen", shortcut: &OKRReopenComment, command: "+comment-reopen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err, _ := runCommentShortcut(t, tc.shortcut, []string{tc.command, "--comment-id", "10", "--user-id-type", "invalid"})
			requireCommentValidationError(t, err, "--user-id-type")
		})
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
	if !strings.Contains(output, `"user_id_type": "open_id"`) {
		t.Fatalf("dry-run should include default user_id_type, got: %s", output)
	}
	if strings.Contains(output, "department_id_type") {
		t.Fatalf("dry-run must omit department_id_type, got: %s", output)
	}
}
