// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBuildPublishBody(t *testing.T) {
	// branch included when non-empty; app_id is NOT in body (it's in the path)
	b := buildPublishBody("feat/devops", "ship it")
	if b["branch"] != "feat/devops" {
		t.Errorf("body = %v", b)
	}
	if b["applyReason"] != "ship it" {
		t.Errorf("applyReason = %v", b["applyReason"])
	}
	if _, ok := b["apply_reason"]; ok {
		t.Errorf("apply_reason must not be in body, got %v", b)
	}
	if _, ok := b["app_id"]; ok {
		t.Errorf("app_id must not be in body, got %v", b)
	}
	// branch omitted when empty
	b2 := buildPublishBody("", "reason")
	if _, ok := b2["branch"]; ok {
		t.Errorf("branch should be omitted when empty, got %v", b2)
	}
	if b2["applyReason"] != "reason" {
		t.Errorf("applyReason = %v", b2["applyReason"])
	}
}

func TestValidateReleaseApplyReason(t *testing.T) {
	longASCII := strings.Repeat("a", maxReleaseApplyReasonRunes)
	longChinese := strings.Repeat("中", maxReleaseApplyReasonRunes)
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "whitespace", value: " \t\n", wantErr: "--apply-reason must not be empty"},
		{name: "lf", value: "before\nafter", wantErr: "--apply-reason must not contain control characters"},
		{name: "cr", value: "before\rafter", wantErr: "--apply-reason must not contain control characters"},
		{name: "tab", value: "before\tafter", wantErr: "--apply-reason must not contain control characters"},
		{name: "nul", value: "before\x00after", wantErr: "--apply-reason must not contain control characters"},
		{name: "c1", value: "before\u0085after", wantErr: "--apply-reason must not contain control characters"},
		{name: "zero width space only", value: "\u200B", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "line separator", value: "before\u2028after", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "paragraph separator", value: "before\u2029after", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "bidi override", value: "before\u202Eafter", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "bidi isolate", value: "before\u2066after", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "bidi isolate terminator", value: "before\u2069after", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "byte order mark", value: "before\uFEFFafter", wantErr: "--apply-reason must not contain dangerous Unicode characters"},
		{name: "invalid utf8", value: string([]byte{'b', 0xff, 'd'}), wantErr: "--apply-reason must be valid UTF-8"},
		{name: "too long ascii", value: longASCII + "a", wantErr: "--apply-reason must be at most 1000 characters"},
		{name: "too long chinese", value: longChinese + "中", wantErr: "--apply-reason must be at most 1000 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReleaseApplyReason(tt.value)
			problem := requireAppsValidationProblem(t, err)
			if problem.Message != tt.wantErr {
				t.Errorf("Message = %q, want %q", problem.Message, tt.wantErr)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != "--apply-reason" {
				t.Errorf("Param = %q, want --apply-reason", validationErr.Param)
			}
		})
	}
	for _, value := range []string{longASCII, longChinese, "$(rm -rf /); `echo unsafe` | cat"} {
		if err := validateReleaseApplyReason(value); err != nil {
			t.Errorf("validateReleaseApplyReason(%q) = %v", value, err)
		}
	}
}

func TestProjectReleaseCreateData(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want releaseCreateOutput
	}{
		{name: "camel", data: map[string]interface{}{"releaseID": "camel", "status": "done", "sync": true}, want: releaseCreateOutput{ReleaseID: "camel", Status: "done", Sync: true}},
		{name: "legacy", data: map[string]interface{}{"release_id": "legacy", "status": "done", "sync": true}, want: releaseCreateOutput{ReleaseID: "legacy", Status: "done", Sync: true}},
		{name: "camel wins", data: map[string]interface{}{"releaseID": "camel", "release_id": "legacy"}, want: releaseCreateOutput{ReleaseID: "camel"}},
		{name: "empty camel wins", data: map[string]interface{}{"releaseID": "", "release_id": "legacy"}, want: releaseCreateOutput{}},
		{name: "wrong type camel wins", data: map[string]interface{}{"releaseID": 42, "release_id": "legacy"}, want: releaseCreateOutput{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectReleaseCreateData(tt.data); got != tt.want {
				t.Errorf("projectReleaseCreateData() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAppsReleaseCreateMeta(t *testing.T) {
	if AppsReleaseCreate.Command != "+release-create" || AppsReleaseCreate.Risk != "write" {
		t.Errorf("meta mismatch: %+v", AppsReleaseCreate)
	}
	if len(AppsReleaseCreate.Scopes) != 1 || AppsReleaseCreate.Scopes[0] != "spark:app:write" {
		t.Errorf("scopes = %v", AppsReleaseCreate.Scopes)
	}
	flags := make(map[string]common.Flag, len(AppsReleaseCreate.Flags))
	for _, flag := range AppsReleaseCreate.Flags {
		flags[flag.Name] = flag
	}
	if !flags["app-id"].Required || !flags["apply-reason"].Required {
		t.Fatalf("app-id and apply-reason must be required: %+v", flags)
	}
	if flags["apply-reason"].Desc != "release application reason (max 1000 characters)" {
		t.Fatalf("apply-reason desc = %q", flags["apply-reason"].Desc)
	}
}

func TestAppsReleaseCreateTipsRequireApplyReason(t *testing.T) {
	for _, tip := range AppsReleaseCreate.Tips {
		if strings.Contains(tip, "+release-create") && !strings.Contains(tip, "--apply-reason") {
			t.Errorf("release-create tip omits required --apply-reason: %q", tip)
		}
	}
}

func TestAppsReleaseCreateRequiresApplyReason(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsReleaseCreate,
		[]string{"+release-create", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "apply-reason") {
		t.Fatalf("expected --apply-reason required error, got %v", err)
	}
}

func TestAppsReleaseCreateRejectsDangerousUnicodeBeforePlanningOrRequest(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		dryRun bool
	}{
		{name: "dry run zero width", reason: "deploy\u200Bnow", dryRun: true},
		{name: "execute bidi override", reason: "deploy\u202Enow"},
		{name: "execute line separator", reason: "deploy\u2028now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			stub := &httpmock.Stub{
				Method:   "POST",
				URL:      "/open-apis/spark/v1/apps/app_x/releases",
				Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{"releaseID": "unexpected"}},
				Optional: true,
			}
			reg.Register(stub)

			args := []string{
				"+release-create", "--app-id", "app_x", "--apply-reason", tt.reason, "--as", "user",
			}
			if tt.dryRun {
				args = append(args, "--dry-run")
			}
			err := runAppsShortcut(t, AppsReleaseCreate, args, factory, stdout)
			problem := requireAppsValidationProblem(t, err)
			if problem.Message != "--apply-reason must not contain dangerous Unicode characters" {
				t.Errorf("Message = %q", problem.Message)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != "--apply-reason" {
				t.Errorf("Param = %q, want --apply-reason", validationErr.Param)
			}
			if stdout.Len() != 0 {
				t.Errorf("validation must fail before writing a dry-run or success result, got %q", stdout.String())
			}
			if len(stub.CapturedBodies) != 0 {
				t.Errorf("validation must fail before the release-create request, got %d request(s)", len(stub.CapturedBodies))
			}
		})
	}
}

func TestAppsReleaseCreateDryRunBody(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	reason := "  fix $() `echo nope`; use | safely  "
	if err := runAppsShortcut(t, AppsReleaseCreate, []string{
		"+release-create", "--app-id", "app_x", "--branch", "  sprint/default  ",
		"--apply-reason", reason, "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if len(env.Data.API) != 1 || env.Data.API[0].Method != "POST" || env.Data.API[0].URL != "/open-apis/spark/v1/apps/app_x/releases" {
		t.Fatalf("dry-run API = %+v", env.Data.API)
	}
	body := env.Data.API[0].Body
	if body["branch"] != "sprint/default" || body["applyReason"] != reason {
		t.Fatalf("dry-run body = %#v", body)
	}
	if _, ok := body["apply_reason"]; ok {
		t.Fatalf("dry-run body contains apply_reason: %#v", body)
	}
	if _, ok := body["app_id"]; ok {
		t.Fatalf("dry-run body contains app_id: %#v", body)
	}

	factory2, stdout2, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsReleaseCreate, []string{
		"+release-create", "--app-id", "app_x", "--apply-reason", reason, "--dry-run", "--as", "user",
	}, factory2, stdout2); err != nil {
		t.Fatalf("dry-run without branch err=%v", err)
	}
	var env2 struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout2.Bytes(), &env2); err != nil {
		t.Fatalf("decode dry-run without branch: %v\n%s", err, stdout2.String())
	}
	body = env2.Data.API[0].Body
	if len(body) != 1 || body["applyReason"] != reason {
		t.Fatalf("dry-run body without branch = %#v", body)
	}
}

// newReleaseCreateRuntimeContext builds a RuntimeContext whose cobra.Command has the
// flags that AppsReleaseCreate.Execute reads (app-id, branch, apply-reason). Flag values are set
// via the returned setter helper.
func newReleaseCreateRuntimeContext(t *testing.T, appID, branch, applyReason string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)

	cmd := &cobra.Command{Use: "test-release-create"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("app-id", "", "")
	cmd.Flags().String("branch", "", "")
	cmd.Flags().String("apply-reason", "", "")
	_ = cmd.Flags().Set("app-id", appID)
	if branch != "" {
		_ = cmd.Flags().Set("branch", branch)
	}
	_ = cmd.Flags().Set("apply-reason", applyReason)

	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestAppsReleaseCreateExecute_Success(t *testing.T) {
	reason := "  fix $() `echo nope`; use | safely  "
	rctx, stdoutBuf, reg := newReleaseCreateRuntimeContext(t, "app_x", "main", reason)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/releases",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"releaseID": "123",
				"status":    "publishing",
				"sync":      false,
			},
		},
	}
	reg.Register(stub)

	err := AppsReleaseCreate.Execute(context.Background(), rctx)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, stdoutBuf.String())
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got: %s", stdoutBuf.String())
	}
	if env.Data["release_id"] != "123" {
		t.Errorf("release_id = %v, want 123", env.Data["release_id"])
	}
	if env.Data["status"] != "publishing" {
		t.Errorf("status = %v, want publishing", env.Data["status"])
	}
	if env.Data["sync"] != false {
		t.Errorf("sync = %v, want false", env.Data["sync"])
	}
	if len(env.Data) != 3 {
		t.Errorf("public output keys = %v, want exactly release_id/status/sync", env.Data)
	}
	for _, key := range []string{"release_id", "status", "sync"} {
		if _, ok := env.Data[key]; !ok {
			t.Errorf("public output missing key %q: %v", key, env.Data)
		}
	}
	for _, key := range []string{"releaseID", "applyReason", "apply_reason"} {
		if _, ok := env.Data[key]; ok {
			t.Errorf("public output leaked key %q: %v", key, env.Data)
		}
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if sent["applyReason"] != reason || sent["branch"] != "main" {
		t.Errorf("request body = %v", sent)
	}
	if _, ok := sent["apply_reason"]; ok {
		t.Errorf("request body contains apply_reason: %v", sent)
	}
	if _, ok := sent["app_id"]; ok {
		t.Errorf("request body contains app_id: %v", sent)
	}
}

func TestAppsReleaseCreateExecute_OmitsEmptyBranch(t *testing.T) {
	rctx, _, reg := newReleaseCreateRuntimeContext(t, "app_no_branch", "   ", "reason")
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_no_branch/releases",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"releaseID": "789"},
		},
	}
	reg.Register(stub)
	if err := AppsReleaseCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(sent) != 1 || sent["applyReason"] != "reason" {
		t.Errorf("request body = %v", sent)
	}
}

func TestAppsReleaseCreate_SyncField(t *testing.T) {
	rctx, stdoutBuf, reg := newReleaseCreateRuntimeContext(t, "app_sync", "main", "sync release")
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_sync/releases",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"release_id": "456",
				"status":     "publishing",
				"sync":       true,
			},
		},
	})

	err := AppsReleaseCreate.Execute(context.Background(), rctx)
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal output: %v\nraw: %s", err, stdoutBuf.String())
	}
	if !env.OK {
		t.Fatalf("expected ok=true, got: %s", stdoutBuf.String())
	}
	if env.Data["release_id"] != "456" {
		t.Errorf("release_id = %v, want 456", env.Data["release_id"])
	}
	if env.Data["status"] != "publishing" {
		t.Errorf("status = %v, want publishing", env.Data["status"])
	}
	if env.Data["sync"] != true {
		t.Errorf("sync = %v, want true", env.Data["sync"])
	}
}
