// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestReplacePagesDeclaredScopes(t *testing.T) {
	if got := SlidesReplacePages.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"slides:presentation:update", "slides:presentation:write_only"}) {
		t.Fatalf("user preflight scopes = %#v, want slides update/write_only only", got)
	}
	if got := SlidesReplacePages.ScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"slides:presentation:update", "slides:presentation:write_only"}) {
		t.Fatalf("bot preflight scopes = %#v, want slides update/write_only only", got)
	}

	got := SlidesReplacePages.DeclaredScopesForIdentity("user")
	want := []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declared scopes = %#v, want %#v", got, want)
	}
}

func TestReplacePagesCreatesBeforeThenDeletesOld(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var requestOrder []string
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"slide_id": "new2", "revision_id": 11, "issues": "slide schema issue"},
		},
		OnMatch: func(req *http.Request) {
			requestOrder = append(requestOrder, req.Method)
		},
	}
	reg.Register(createStub)
	var deleteQuery map[string][]string
	deleteStub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"revision_id": 12},
		},
		OnMatch: func(req *http.Request) {
			requestOrder = append(requestOrder, req.Method)
			deleteQuery = req.URL.Query()
		},
	}
	reg.Register(deleteStub)

	pages := `[{"slide_id":"old2","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
		"+replace-pages",
		"--presentation", "pres_abc",
		"--pages", pages,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var createBody struct {
		Slide struct {
			Content string `json:"content"`
		} `json:"slide"`
		BeforeSlideID string `json:"before_slide_id"`
	}
	if err := json.Unmarshal(createStub.CapturedBody, &createBody); err != nil {
		t.Fatalf("decode create body: %v\nraw=%s", err, createStub.CapturedBody)
	}
	if createBody.BeforeSlideID != "old2" {
		t.Fatalf("before_slide_id = %q, want old2", createBody.BeforeSlideID)
	}
	if !strings.Contains(createBody.Slide.Content, "<slide") {
		t.Fatalf("create content = %q", createBody.Slide.Content)
	}
	if !reflect.DeepEqual(requestOrder, []string{"POST", "DELETE"}) {
		t.Fatalf("request order = %#v, want POST then DELETE", requestOrder)
	}
	deleteURL := string(deleteStub.CapturedBody)
	if deleteURL != "" {
		t.Fatalf("delete body = %q, want empty", deleteURL)
	}
	if got := deleteQuery["slide_id"]; !reflect.DeepEqual(got, []string{"old2"}) {
		t.Fatalf("delete slide_id = %#v, want old2", got)
	}
	if got := deleteQuery["revision_id"]; !reflect.DeepEqual(got, []string{"11"}) {
		t.Fatalf("delete revision_id = %#v, want 11 from create response", got)
	}

	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v", data["xml_presentation_id"])
	}
	if data["revision_id"] != float64(12) {
		t.Fatalf("revision_id = %v, want 12", data["revision_id"])
	}
	summary, _ := data["summary"].(map[string]interface{})
	if summary["failed"] != float64(0) {
		t.Fatalf("summary.failed = %v, want 0", summary["failed"])
	}
	results, _ := data["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	first, _ := results[0].(map[string]interface{})
	if first["old_slide_id"] != "old2" || first["new_slide_id"] != "new2" || first["status"] != "replaced" {
		t.Fatalf("result = %#v", first)
	}
	if first["issues"] != "slide schema issue" {
		t.Fatalf("result.issues = %v, want slide schema issue", first["issues"])
	}
}

func TestReplacePagesContinueOnErrorReturnsPartialFailure(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 3350001,
			"msg":  "invalid param",
			"data": map[string]interface{}{},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"slide_id": "new2", "revision_id": 11},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"revision_id": 12},
		},
	})

	pages := `[
		{"slide_id":"old1","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"},
		{"slide_id":"old2","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"}
	]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
		"+replace-pages",
		"--presentation", "pres_abc",
		"--pages", pages,
		"--continue-on-error",
		"--as", "user",
	})
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("err = %T %v, want *output.PartialFailureError", err, err)
	}

	env := decodeReplacePagesEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("stdout ok = true, want false for partial failure")
	}
	data := env.Data
	if data["status"] != "partial_failure" {
		t.Fatalf("status = %v, want partial_failure", data["status"])
	}
	summary, _ := data["summary"].(map[string]interface{})
	if summary["replaced"] != float64(1) || summary["failed"] != float64(1) || summary["total"] != float64(2) {
		t.Fatalf("summary = %#v, want replaced=1 failed=1 total=2", summary)
	}
	results, _ := data["results"].([]interface{})
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	first, _ := results[0].(map[string]interface{})
	second, _ := results[1].(map[string]interface{})
	if first["status"] != "create_failed" {
		t.Fatalf("first status = %v, want create_failed", first["status"])
	}
	if second["status"] != "replaced" || second["new_slide_id"] != "new2" {
		t.Fatalf("second result = %#v, want replaced with new2", second)
	}
}

// TestReplacePagesContinueOnErrorKeepsLintRecoveryPerItem pins the one thing
// --continue-on-error changes about a refusal: nothing. A batch that keeps going
// reports its failures only through the per-item records, so if the enrichment
// lived on the returning path the caller with the most pages to fix would be the
// one told least about how to fix them.
//
// The report travels verbatim in the item for the same reason it does in the
// error: it is the server's document and one parse has to work on it wherever it
// surfaced. The code and the hint travel beside it because Problem.Error() is
// the message alone, so an item built from the string would name neither the
// refusal nor the flag that gets past it.
func TestReplacePagesContinueOnErrorKeepsLintRecoveryPerItem(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:   map[string]interface{}{"code": lintBlockedCode, "msg": lintBlockMessage},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"slide_id": "new2", "revision_id": 11},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"revision_id": 12},
		},
	})

	pages := `[
		{"slide_id":"old1","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"},
		{"slide_id":"old2","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"}
	]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
		"+replace-pages",
		"--presentation", "pres_abc",
		"--pages", pages,
		"--continue-on-error",
		"--as", "user",
	})
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("err = %T %v, want *output.PartialFailureError", err, err)
	}

	env := decodeReplacePagesEnvelope(t, stdout)
	results, _ := env.Data["results"].([]interface{})
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	refused, _ := results[0].(map[string]interface{})
	if refused["status"] != "create_failed" {
		t.Fatalf("refused item status = %v, want create_failed", refused["status"])
	}
	if refused["error_code"] != float64(lintBlockedCode) {
		t.Fatalf("refused item error_code = %v, want %d: the item cannot be recognised as a lint refusal without it",
			refused["error_code"], lintBlockedCode)
	}
	if refused["error"] != lintBlockMessage {
		t.Fatalf("refused item error = %v, want the server report verbatim", refused["error"])
	}
	hint, _ := refused["hint"].(string)
	for _, want := range []string{"xml lint blocked: 2 error(s)", "--no-lint"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("refused item hint lost %q\ngot: %q", want, hint)
		}
	}
	// The position belongs to the stop-on-error path. Here every item has its own
	// record, so a "stopped at item 1/2" would describe a batch that did not stop.
	if strings.Contains(hint, "stopped at item") {
		t.Fatalf("refused item hint = %q, want no stop-on-error progress line", hint)
	}
	// A refusal is not a partial write: the page the batch moved on from must not
	// look like it left a new page behind.
	if _, ok := refused["new_slide_id"]; ok {
		t.Fatalf("refused item = %#v, want no new_slide_id", refused)
	}
	survivor, _ := results[1].(map[string]interface{})
	if survivor["status"] != "replaced" || survivor["new_slide_id"] != "new2" {
		t.Fatalf("second result = %#v, want replaced with new2", survivor)
	}
	if _, ok := survivor["error_code"]; ok {
		t.Fatalf("second result = %#v, want no error_code on a page that landed", survivor)
	}
}

func TestReplacePagesContinueOnErrorDeleteFailureIncludesNewSlideID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"slide_id": "new1", "revision_id": 11},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{
			"code": 3350001,
			"msg":  "invalid param",
			"data": map[string]interface{}{},
		},
	})

	pages := `[{"slide_id":"old1","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
		"+replace-pages",
		"--presentation", "pres_abc",
		"--pages", pages,
		"--continue-on-error",
		"--as", "user",
	})
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("err = %T %v, want *output.PartialFailureError", err, err)
	}

	env := decodeReplacePagesEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("stdout ok = true, want false for partial failure")
	}
	results, _ := env.Data["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	first, _ := results[0].(map[string]interface{})
	if first["status"] != "delete_failed" {
		t.Fatalf("status = %v, want delete_failed", first["status"])
	}
	if first["new_slide_id"] != "new1" {
		t.Fatalf("new_slide_id = %v, want new1", first["new_slide_id"])
	}
}

func TestReplacePagesDryRunPlansOnly(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	pages := `[{"slide_id":"old2","content":"<slide xmlns=\"https://www.larkoffice.com/sml/2.0\"><data></data></slide>"}]`
	err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
		"+replace-pages",
		"--presentation", "pres_abc",
		"--pages", pages,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode dry-run: %v\nraw=%s", err, stdout.String())
	}
	data, _ := out["data"].(map[string]interface{})
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v", data["xml_presentation_id"])
	}
	plan, _ := data["plan"].([]interface{})
	if len(plan) != 1 {
		t.Fatalf("plan len = %d, want 1", len(plan))
	}
	item, _ := plan[0].(map[string]interface{})
	if item["old_slide_id"] != "old2" || item["action"] != "create_before_then_delete_old" {
		t.Fatalf("plan item = %#v", item)
	}
	api, _ := data["api"].([]interface{})
	if len(api) != 2 {
		t.Fatalf("api len = %d, want create/delete plan", len(api))
	}
}

func TestReplacePagesValidationParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		pages string
	}{
		{"empty pages", `[]`},
		{"slide number no longer supported", `[{"slide_number":1,"content":"<slide/>"}]`},
		{"no locator", `[{"content":"<slide/>"}]`},
		{"empty content", `[{"slide_id":"s1","content":"  "}]`},
		{"not slide XML", `[{"slide_id":"s1","content":"<shape/>"}]`},
		{"duplicate id", `[{"slide_id":"s1","content":"<slide/>"},{"slide_id":"s1","content":"<slide/>"}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, []string{
				"+replace-pages",
				"--presentation", "pres_abc",
				"--pages", tt.pages,
				"--as", "user",
			})
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			if ve.Param != "--pages" {
				t.Fatalf("Param = %q, want --pages", ve.Param)
			}
		})
	}
}

type replacePagesEnvelope struct {
	OK   bool                   `json:"ok"`
	Data map[string]interface{} `json:"data"`
}

func decodeReplacePagesEnvelope(t *testing.T, stdout interface{ Bytes() []byte }) replacePagesEnvelope {
	t.Helper()
	var env replacePagesEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, string(stdout.Bytes()))
	}
	if env.Data == nil {
		t.Fatalf("missing data: %#v", env)
	}
	return env
}
