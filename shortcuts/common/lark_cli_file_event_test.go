// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func newUploadFileEventRuntime(t *testing.T) (*RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+upload"}, cfg, f, core.AsUser)
	return rt, reg
}

// testCapacityExpansionURL is a placeholder capacity-expansion URL used in
// tests. It intentionally uses example.com so no internal endpoint is embedded
// in the repository.
const testCapacityExpansionURL = "https://example.com/space/upload/pay/prepare"

func registerReportStub(t *testing.T, reg *httpmock.Registry, code int) *httpmock.Stub {
	t.Helper()
	return registerReportStubWithMsg(t, reg, code, "")
}

// registerReportStubWithMsg registers a report_file_event stub returning the
// given top-level code and msg.
func registerReportStubWithMsg(t *testing.T, reg *httpmock.Registry, code int, msg string) *httpmock.Stub {
	t.Helper()
	return registerReportStubWithBody(t, reg, map[string]interface{}{
		"code": code,
		"data": map[string]interface{}{},
		"msg":  msg,
	})
}

func registerReportStubWithBody(t *testing.T, reg *httpmock.Registry, body map[string]interface{}) *httpmock.Stub {
	t.Helper()
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      larkCLIReportFileEventPath,
		Body:     body,
		Reusable: true,
	}
	reg.Register(stub)
	return stub
}

func TestIsTenantCapacityExceeded(t *testing.T) {
	if !IsTenantCapacityExceeded(errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)) {
		t.Fatal("code 1061101 should be recognized as tenant capacity exceeded")
	}

	// Legacy quota codes are intentionally no longer recognized: only the
	// tenant-capacity-exceeded code 1061101 gates the expansion hint.
	for _, code := range []int{11001, 90008072, 90003081, 10690008072, 10690003081} {
		err := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(code)
		if IsTenantCapacityExceeded(err) {
			t.Fatalf("code %d must not be recognized as tenant capacity exceeded", code)
		}
	}

	if IsTenantCapacityExceeded(errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(12345)) {
		t.Fatal("unexpected recognition for unrelated quota code")
	}
	if IsTenantCapacityExceeded(errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input")) {
		t.Fatal("non api error must not be recognized")
	}
}

func TestReportUploadFileEvent_Success_ReportsOnceWithMinimalBody(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	reportStub := registerReportStub(t, reg, 0)

	meta := LarkCLIFileEventMeta{
		APIPath:      "/open-apis/drive/v1/medias/upload_all",
		Command:      "drive +upload",
		UploadMode:   "singlepart",
		ResourceType: "media",
		ParentType:   "docx_file",
		FileToken:    "boxcnabc123",
	}
	ReportUploadFileEvent(runtime, meta)
	ReportUploadFileEvent(runtime, meta)

	if len(reportStub.CapturedBodies) != 1 {
		t.Fatalf("report call count = %d, want 1", len(reportStub.CapturedBodies))
	}
	body := decodeCapturedDriveMediaJSONBody(t, reportStub)
	assertReportEnvelope(t, body)
	if _, ok := body["user_id"]; ok {
		t.Fatalf("user_id must be omitted, got %v", body["user_id"])
	}
	if _, ok := body["tenant_id"]; ok {
		t.Fatalf("tenant_id must be omitted, got %v", body["tenant_id"])
	}
	tags := assertTagsObject(t, body)
	if got := tags["status"]; got != uploadFileEventStatusSuccess {
		t.Fatalf("tags.status = %v, want success", got)
	}
	if got := tags["api_path"]; got != meta.APIPath {
		t.Fatalf("tags.api_path = %v, want %s", got, meta.APIPath)
	}
	if got := tags["command"]; got != meta.Command {
		t.Fatalf("tags.command = %v, want %s", got, meta.Command)
	}
	if got := tags["upload_mode"]; got != meta.UploadMode {
		t.Fatalf("tags.upload_mode = %v, want %s", got, meta.UploadMode)
	}
	if got := tags["resource_type"]; got != meta.ResourceType {
		t.Fatalf("tags.resource_type = %v, want %s", got, meta.ResourceType)
	}
	if got := tags["mount_point"]; got != meta.ParentType {
		t.Fatalf("tags.mount_point = %v, want %s", got, meta.ParentType)
	}
	if got := tags["file_token"]; got != meta.FileToken {
		t.Fatalf("tags.file_token = %v, want %s", got, meta.FileToken)
	}
}

func TestBuildUploadReportRequest_CommandOmitsBinaryName(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	drive := &cobra.Command{Use: "drive"}
	upload := &cobra.Command{Use: "+upload"}
	root.AddCommand(drive)
	drive.AddCommand(upload)

	body := buildUploadReportRequest(&RuntimeContext{Cmd: upload}, LarkCLIFileEventMeta{})
	tags := assertTagsObject(t, body)
	if got := tags["command"]; got != "drive +upload" {
		t.Fatalf("tags.command = %v, want drive +upload", got)
	}
}

func TestReportUploadFileEventOnError_ReportsAndPreservesError(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	reportStub := registerReportStub(t, reg, 0)

	uploadErr := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(42)
	meta := LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_all"}

	returned := ReportUploadFileEventOnError(runtime, uploadErr, meta)
	if returned != uploadErr {
		t.Fatalf("returned error changed: got %v want original %v", returned, uploadErr)
	}
	returned = ReportUploadFileEventOnError(runtime, uploadErr, meta)
	if returned != uploadErr {
		t.Fatalf("second call changed error: got %v want original %v", returned, uploadErr)
	}
	if len(reportStub.CapturedBodies) != 1 {
		t.Fatalf("report call count = %d, want 1", len(reportStub.CapturedBodies))
	}
	tags := assertTagsObject(t, decodeCapturedDriveMediaJSONBody(t, reportStub))
	if got := tags["status"]; got != uploadFileEventStatusError {
		t.Fatalf("tags.status = %v, want error", got)
	}
	if got := tags["code"]; got != "42" {
		t.Fatalf("tags.code = %v, want 42", got)
	}
}

func TestReportUploadFileEventOnError_ReportFailureDoesNotReplaceUploadError(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkCLIReportFileEventPath,
		Body:   map[string]interface{}{"code": 999, "msg": "report rejected"},
	})

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(10690008072)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_prepare"})
	if returned != uploadErr {
		t.Fatalf("returned error changed: got %v want original %v", returned, uploadErr)
	}
}

func TestReportUploadFileEventOnError_AppendsCapacityExpansionHint(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	registerReportStubWithBody(t, reg, map[string]interface{}{
		"code": 0,
		"msg":  "success",
		"data": map[string]interface{}{
			"msg": testCapacityExpansionURL,
		},
	})

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_prepare"})

	p, ok := errs.ProblemOf(returned)
	if !ok || p == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if !strings.Contains(p.Hint, testCapacityExpansionURL) {
		t.Fatalf("hint = %q, want it to contain %q", p.Hint, testCapacityExpansionURL)
	}
	if p.Code != 1061101 {
		t.Fatalf("code changed: got %d, want 1061101", p.Code)
	}
	if p.Subtype != errs.SubtypeQuotaExceeded {
		t.Fatalf("subtype changed: got %q, want %q", p.Subtype, errs.SubtypeQuotaExceeded)
	}
}

func TestReportUploadFileEventOnError_TopLevelSuccessMsgDoesNotBecomeHint(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	registerReportStubWithMsg(t, reg, 0, "success")

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_prepare"})

	p, ok := errs.ProblemOf(returned)
	if !ok || p == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if strings.TrimSpace(p.Hint) != "" {
		t.Fatalf("top-level success msg must not become hint, got %q", p.Hint)
	}
}

func TestReportUploadFileEventOnError_InvalidURLInDataMsgIsIgnored(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	registerReportStubWithBody(t, reg, map[string]interface{}{
		"code": 0,
		"msg":  "success",
		"data": map[string]interface{}{
			"msg": "https://https://example.com/space/upload/pay/prepare",
		},
	})

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_prepare"})

	p, ok := errs.ProblemOf(returned)
	if !ok || p == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if strings.TrimSpace(p.Hint) != "" {
		t.Fatalf("invalid data.msg URL must be ignored, got %q", p.Hint)
	}
}

func TestReportUploadFileEventOnError_EmptyReportMsgYieldsNoHint(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	// report returns code 0 but no msg: no capacity-expansion URL to surface.
	registerReportStub(t, reg, 0)

	uploadErr := errs.NewAPIError(errs.SubtypeQuotaExceeded, "quota exceeded").WithCode(1061101)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{APIPath: "/open-apis/drive/v1/files/upload_prepare"})

	p, ok := errs.ProblemOf(returned)
	if !ok || p == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if strings.TrimSpace(p.Hint) != "" {
		t.Fatalf("empty report msg must yield no hint, got %q", p.Hint)
	}
	if p.Code != 1061101 {
		t.Fatalf("code changed: got %d, want 1061101", p.Code)
	}
}

func TestReportUploadFileEventOnError_NonQuotaErrorKeepsHint(t *testing.T) {
	runtime, reg := newUploadFileEventRuntime(t)
	registerReportStubWithMsg(t, reg, 0, testCapacityExpansionURL)

	uploadErr := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithCode(42)
	returned := ReportUploadFileEventOnError(runtime, uploadErr, LarkCLIFileEventMeta{})
	p, ok := errs.ProblemOf(returned)
	if !ok || p == nil {
		t.Fatalf("expected typed problem, got %T (%v)", returned, returned)
	}
	if strings.Contains(p.Hint, testCapacityExpansionURL) {
		t.Fatalf("non-quota error must not get expansion hint, got %q", p.Hint)
	}
}

func TestReportUploadFileEventOnError_NilErrorIsNoop(t *testing.T) {
	runtime, _ := newUploadFileEventRuntime(t)

	// No report stub is registered: a nil upload error must not attempt a
	// report at all (an unexpected POST would fail with "no stub").
	if err := ReportUploadFileEventOnError(runtime, nil, LarkCLIFileEventMeta{}); err != nil {
		t.Fatalf("nil upload error should return nil, got %v", err)
	}
	// The reporting mark must remain unconsumed, proving no report fired.
	if !runtime.MarkFileEventReported() {
		t.Fatal("nil error path must not consume the file-event report mark")
	}
}

type contextBlockingRoundTripper struct{}

func (contextBlockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestPostUploadFileEventWithTimeout_BoundsBestEffortRequest(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	f.LarkClient = func() (*lark.Client, error) {
		return lark.NewClient("cli_x", "test-secret", lark.WithHttpClient(&http.Client{
			Transport: contextBlockingRoundTripper{},
		})), nil
	}
	runtime := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+upload"}, cfg, f, core.AsUser)

	started := time.Now()
	if got := postUploadFileEventWithTimeout(runtime, LarkCLIFileEventMeta{}, 10*time.Millisecond); got != "" {
		t.Fatalf("postUploadFileEventWithTimeout() = %q, want empty result on timeout", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("best-effort report took %s, want it bounded by the request context", elapsed)
	}
}

func assertReportEnvelope(t *testing.T, body map[string]interface{}) {
	t.Helper()
	if got := body["file_scene"]; got != "lark-cli" {
		t.Fatalf("file_scene = %v, want lark-cli", got)
	}
	if got := body["scene"]; got != "upload" {
		t.Fatalf("scene = %v, want upload", got)
	}
	if got := body["operation"]; got != "upload" {
		t.Fatalf("operation = %v, want upload", got)
	}
}

func assertTagsObject(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	switch tags := body["tags"].(type) {
	case map[string]interface{}:
		return tags
	case map[string]string:
		result := make(map[string]interface{}, len(tags))
		for key, value := range tags {
			result[key] = value
		}
		return result
	default:
		t.Fatalf("tags = %#v, want object", body["tags"])
		return nil
	}
}
