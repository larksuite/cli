// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package fileevent

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
)

const testCapacityExpansionURL = "https://example.com/space/upload/pay/prepare"

type fakeRuntime struct {
	ctx         context.Context
	command     string
	commandPath string
	requests    []*larkcore.ApiReq
	response    *larkcore.ApiResp
	err         error
	doAPI       func(context.Context, *larkcore.ApiReq) (*larkcore.ApiResp, error)
	budget      *Budget
}

func newFakeRuntime(body string) *fakeRuntime {
	return &fakeRuntime{
		ctx: context.Background(),
		response: &larkcore.ApiResp{
			StatusCode: http.StatusOK,
			RawBody:    []byte(body),
		},
		budget: NewBudget(),
	}
}

func (r *fakeRuntime) Ctx() context.Context     { return r.ctx }
func (r *fakeRuntime) Command() string          { return r.command }
func (r *fakeRuntime) CommandPath() string      { return r.commandPath }
func (r *fakeRuntime) FileEventBudget() *Budget { return r.budget }

func (r *fakeRuntime) DoAPIWithContext(ctx context.Context, req *larkcore.ApiReq, _ ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error) {
	r.requests = append(r.requests, req)
	if r.doAPI != nil {
		return r.doAPI(ctx, req)
	}
	return r.response, r.err
}

func TestIsTenantCapacityExceeded(t *testing.T) {
	if !IsTenantCapacityExceeded(errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)) {
		t.Fatal("code 1061101 should be recognized as tenant capacity exceeded")
	}
	for _, code := range []int{11001, 90008072, 90003081, 10690008072, 10690003081, 12345} {
		err := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(code)
		if IsTenantCapacityExceeded(err) {
			t.Fatalf("code %d must not be recognized as tenant capacity exceeded", code)
		}
	}
	if IsTenantCapacityExceeded(errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input")) {
		t.Fatal("non-API error must not be recognized")
	}
}

func TestReportUploadReportsEveryCallWithMinimalBody(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0,"data":{}}`)
	meta := UploadMeta{
		APIPath:      "/open-apis/drive/v1/medias/upload_all",
		Command:      "drive +upload",
		UploadMode:   "singlepart",
		ResourceType: "media",
		ParentType:   "docx_file",
		FileToken:    "boxcnabc123",
	}

	ReportUpload(runtime, meta)
	ReportUpload(runtime, meta)

	if len(runtime.requests) != 2 {
		t.Fatalf("report call count = %d, want 2", len(runtime.requests))
	}
	req := runtime.requests[0]
	if req.HttpMethod != http.MethodPost || req.ApiPath != ReportPath {
		t.Fatalf("request = %s %s, want POST %s", req.HttpMethod, req.ApiPath, ReportPath)
	}
	body := requestBody(t, req)
	assertReportEnvelope(t, body)
	tags := requestTags(t, body)
	wantTags := map[string]string{
		"status":        StatusSuccess,
		"api_path":      meta.APIPath,
		"command":       meta.Command,
		"upload_mode":   meta.UploadMode,
		"resource_type": meta.ResourceType,
		"mount_point":   meta.ParentType,
		"file_token":    meta.FileToken,
		"code":          "",
	}
	for key, want := range wantTags {
		if got := tags[key]; got != want {
			t.Fatalf("tags.%s = %q, want %q", key, got, want)
		}
	}
	if _, ok := body["user_id"]; ok {
		t.Fatal("user_id must be omitted")
	}
	if _, ok := body["tenant_id"]; ok {
		t.Fatal("tenant_id must be omitted")
	}
}

func TestBuildUploadReportRequestCommandOmitsBinaryName(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0}`)
	runtime.command = "+upload"
	runtime.commandPath = "lark-cli drive +upload"

	tags := requestTags(t, buildUploadReportRequest(runtime, UploadMeta{}))
	if got := tags["command"]; got != "drive +upload" {
		t.Fatalf("tags.command = %q, want drive +upload", got)
	}
}

func TestReportUploadErrorReportsAndPreservesError(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0,"data":{}}`)
	uploadErr := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(42)
	meta := UploadMeta{APIPath: "/open-apis/drive/v1/files/upload_all"}

	for range 2 {
		if returned := ReportUploadError(runtime, uploadErr, meta); returned != uploadErr {
			t.Fatalf("returned error changed: got %v, want original %v", returned, uploadErr)
		}
	}
	if len(runtime.requests) != 2 {
		t.Fatalf("report call count = %d, want 2", len(runtime.requests))
	}
	tags := requestTags(t, requestBody(t, runtime.requests[0]))
	if tags["status"] != StatusError || tags["code"] != "42" {
		t.Fatalf("status/code = %q/%q, want error/42", tags["status"], tags["code"])
	}
}

func TestReportUploadErrorReportFailureDoesNotReplaceUploadError(t *testing.T) {
	runtime := newFakeRuntime(`{"code":999,"msg":"report rejected"}`)
	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	if returned := ReportUploadError(runtime, uploadErr, UploadMeta{}); returned != uploadErr {
		t.Fatalf("returned error changed: got %v, want original %v", returned, uploadErr)
	}
	problem, _ := errs.ProblemOf(uploadErr)
	if problem.Hint != "" {
		t.Fatalf("failed report changed hint to %q", problem.Hint)
	}
}

func TestReportUploadErrorRejectsInvalidExpansionResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "HTTP error with success body",
			statusCode: http.StatusInternalServerError,
			body:       `{"code":0,"msg":"` + testCapacityExpansionURL + `"}`,
		},
		{
			name:       "missing business code",
			statusCode: http.StatusOK,
			body:       `{"msg":"` + testCapacityExpansionURL + `"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newFakeRuntime(test.body)
			runtime.response.StatusCode = test.statusCode
			uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)

			returned := ReportUploadError(runtime, uploadErr, UploadMeta{})
			problem, _ := errs.ProblemOf(returned)
			if problem.Hint != "" {
				t.Fatalf("invalid report response changed hint to %q", problem.Hint)
			}
		})
	}
}

func TestReportUploadErrorAppendsCapacityExpansionHint(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0,"msg":"success","data":{"msg":"` + testCapacityExpansionURL + `"}}`)
	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	returned := ReportUploadError(runtime, uploadErr, UploadMeta{})

	problem, ok := errs.ProblemOf(returned)
	if !ok || problem == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if !strings.Contains(problem.Hint, testCapacityExpansionURL) {
		t.Fatalf("hint = %q, want capacity expansion URL", problem.Hint)
	}
	if problem.Code != 1061101 || problem.Subtype != errs.SubtypeQuotaExceeded {
		t.Fatalf("code/subtype = %d/%q, want 1061101/%q", problem.Code, problem.Subtype, errs.SubtypeQuotaExceeded)
	}
}

func TestReportUploadErrorIgnoresUnusableExpansionMessages(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "generic top-level message", body: `{"code":0,"msg":"success","data":{}}`},
		{name: "invalid data URL", body: `{"code":0,"data":{"msg":"https://https://example.com/space/upload/pay/prepare"}}`},
		{name: "empty message", body: `{"code":0,"data":{}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newFakeRuntime(test.body)
			uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
			returned := ReportUploadError(runtime, uploadErr, UploadMeta{})
			problem, _ := errs.ProblemOf(returned)
			if problem.Hint != "" {
				t.Fatalf("unusable report message changed hint to %q", problem.Hint)
			}
		})
	}
}

func TestReportUploadErrorNonQuotaErrorKeepsHint(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0,"msg":"` + testCapacityExpansionURL + `"}`)
	uploadErr := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(42)
	problem, _ := errs.ProblemOf(uploadErr)
	problem.Hint = "retry the original operation"

	returned := ReportUploadError(runtime, uploadErr, UploadMeta{})
	returnedProblem, _ := errs.ProblemOf(returned)
	if returnedProblem.Hint != "retry the original operation" {
		t.Fatalf("non-quota hint = %q, want original hint", returnedProblem.Hint)
	}
}

func TestReportUploadErrorNilErrorIsNoop(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0}`)
	if err := ReportUploadError(runtime, nil, UploadMeta{}); err != nil {
		t.Fatalf("nil upload error should return nil, got %v", err)
	}
	if len(runtime.requests) != 0 {
		t.Fatalf("nil upload error made %d report calls, want 0", len(runtime.requests))
	}
}

func TestReportUploadTypedNilRuntimeIsNoop(t *testing.T) {
	var runtime *fakeRuntime
	ReportUpload(runtime, UploadMeta{})

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	if returned := ReportUploadError(runtime, uploadErr, UploadMeta{}); returned != uploadErr {
		t.Fatalf("returned error changed: got %v, want original %v", returned, uploadErr)
	}
}

func TestReportUploadUsesCommandScopedTotalBudget(t *testing.T) {
	runtime := newFakeRuntime("")
	runtime.budget = newBudget(20*time.Millisecond, 10*time.Millisecond)
	runtime.doAPI = func(ctx context.Context, _ *larkcore.ApiReq) (*larkcore.ApiResp, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ReportUpload(runtime, UploadMeta{})
	ReportUpload(runtime, UploadMeta{})
	if len(runtime.requests) != 1 {
		t.Fatalf("report request count = %d, want 1 after shared budget expires", len(runtime.requests))
	}
}

func TestTenantCapacityReportGetsOneSeparateAttempt(t *testing.T) {
	runtime := newFakeRuntime(`{"code":0,"msg":"` + testCapacityExpansionURL + `"}`)
	runtime.budget = newBudget(0, 20*time.Millisecond)
	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)

	first := ReportUploadError(runtime, uploadErr, UploadMeta{})
	second := ReportUploadError(runtime, uploadErr, UploadMeta{})
	if len(runtime.requests) != 1 {
		t.Fatalf("capacity report request count = %d, want 1", len(runtime.requests))
	}
	for _, returned := range []error{first, second} {
		problem, _ := errs.ProblemOf(returned)
		if !strings.Contains(problem.Hint, testCapacityExpansionURL) {
			t.Fatalf("hint = %q, want capacity expansion URL retained", problem.Hint)
		}
	}
}

func requestBody(t *testing.T, req *larkcore.ApiReq) map[string]interface{} {
	t.Helper()
	body, ok := req.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("request body = %T, want map[string]interface{}", req.Body)
	}
	return body
}

func requestTags(t *testing.T, body map[string]interface{}) map[string]string {
	t.Helper()
	tags, ok := body["tags"].(map[string]string)
	if !ok {
		t.Fatalf("tags = %T, want map[string]string", body["tags"])
	}
	return tags
}

func assertReportEnvelope(t *testing.T, body map[string]interface{}) {
	t.Helper()
	if body["file_scene"] != "lark-cli" || body["scene"] != "upload" || body["operation"] != "upload" {
		t.Fatalf("unexpected report envelope: %#v", body)
	}
}
