// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package api

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

func TestApiCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Method != "GET" {
		t.Errorf("expected method GET, got %s", gotOpts.Method)
	}
	if gotOpts.Path != "/open-apis/test" {
		t.Errorf("expected path /open-apis/test, got %s", gotOpts.Path)
	}
	if gotOpts.As != core.AsBot {
		t.Errorf("expected as=bot, got %s", gotOpts.As)
	}
	if !gotOpts.DryRun {
		t.Error("expected DryRun=true")
	}
}

func TestApiCmd_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Error("expected dry run output")
	}
	if !strings.Contains(output, "/open-apis/test") {
		t.Error("expected path in dry run output")
	}
}

func TestApiCmd_BotMode(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	// Register API endpoint stub
	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/test",
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"result": "success"}},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "success") {
		t.Error("expected 'success' in output")
	}
}

func TestApiCmd_MissingArgs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET"}) // missing path
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing args")
	}
}

func TestApiCmd_InvalidParamsJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--params", "{bad"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected validation error for invalid JSON")
	}
}

func TestApiValidArgsFunction(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, nil)
	fn := cmd.ValidArgsFunction

	tests := []struct {
		name       string
		args       []string
		toComplete string
		wantComps  []string
		wantDir    cobra.ShellCompDirective
	}{
		{
			name:       "no args returns HTTP methods",
			args:       []string{},
			toComplete: "",
			wantComps:  []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			wantDir:    cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:       "one arg returns nil with NoFileComp",
			args:       []string{"GET"},
			toComplete: "",
			wantComps:  nil,
			wantDir:    cobra.ShellCompDirectiveNoFileComp,
		},
		{
			name:       "two args returns nil with NoFileComp",
			args:       []string{"GET", "/path"},
			toComplete: "",
			wantComps:  nil,
			wantDir:    cobra.ShellCompDirectiveNoFileComp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comps, dir := fn(cmd, tt.args, tt.toComplete)
			if dir != tt.wantDir {
				t.Errorf("directive = %d, want %d", dir, tt.wantDir)
			}
			if tt.wantComps == nil {
				if comps != nil {
					t.Errorf("completions = %v, want nil", comps)
				}
				return
			}
			sort.Strings(comps)
			sort.Strings(tt.wantComps)
			if len(comps) != len(tt.wantComps) {
				t.Errorf("completions = %v, want %v", comps, tt.wantComps)
				return
			}
			for i := range comps {
				if comps[i] != tt.wantComps[i] {
					t.Errorf("completions = %v, want %v", comps, tt.wantComps)
					break
				}
			}
		})
	}
}

func TestApiCmd_PageLimitDefault(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.PageLimit != 10 {
		t.Errorf("expected default PageLimit=10, got %d", gotOpts.PageLimit)
	}
}

func TestApiCmd_OutputAndPageAllConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--page-all", "--output", "file.bin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --output + --page-all conflict")
	}
	if gotOpts != nil && !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestApiCmd_BinaryResponse_AutoSave(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-bin", AppSecret: "test-secret-bin", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL:         "/open-apis/drive/v1/files/xxx/download",
		RawBody:     []byte("fake-binary-content"),
		ContentType: "application/octet-stream",
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/drive/v1/files/xxx/download", "--as", "bot"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "binary response detected") {
		t.Error("expected binary response hint in stderr")
	}
	if !strings.Contains(stdout.String(), "saved_path") {
		t.Error("expected saved_path in output")
	}
}

func TestApiCmd_PageAll_NonBatchAPI_FallbackToJSON(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall1", AppSecret: "test-secret-pageall1", Brand: core.BrandFeishu,
	})

	// Register a non-batch API that returns scalar data (no array field)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users/u123",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"user_id": "u123",
				"name":    "Test User",
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/contact/v3/users/u123", "--as", "bot", "--page-all", "--format", "ndjson"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should print fallback warning to stderr
	if !strings.Contains(stderr.String(), "warning: this API does not return a list") {
		t.Error("expected fallback warning in stderr")
	}
	if !strings.Contains(stderr.String(), "falling back to json") {
		t.Error("expected 'falling back to json' in stderr")
	}
	// Should output JSON result to stdout
	if !strings.Contains(stdout.String(), "u123") {
		t.Error("expected user_id in JSON output")
	}
}

func TestApiCmd_PageAll_NonBatchAPI_ErrorStillOutputsJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-err", AppSecret: "test-secret-pageall-err", Brand: core.BrandFeishu,
	})

	// Non-batch API that returns a business error (code != 0)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_xxx/announcement",
		Body: map[string]interface{}{
			"code": 230001, "msg": "no permission",
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/im/v1/chats/oc_xxx/announcement", "--as", "bot", "--page-all"})
	err := cmd.Execute()
	// Should return an error
	if err == nil {
		t.Fatal("expected an error for non-zero code")
	}
	// Should still output the response body so user can see the error details
	if !strings.Contains(stdout.String(), "230001") {
		t.Errorf("expected error response in stdout, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "no permission") {
		t.Errorf("expected error message in stdout, got: %s", stdout.String())
	}
}

func TestApiCmd_PageAll_BatchAPI_StreamsItems(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall2", AppSecret: "test-secret-pageall2", Brand: core.BrandFeishu,
	})

	// Register a batch API that returns an array field
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}, map[string]interface{}{"id": "2"}},
				"has_more": false,
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all", "--format", "ndjson"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should NOT print fallback warning
	if strings.Contains(stderr.String(), "warning: this API does not return a list") {
		t.Error("expected no fallback warning for batch API")
	}
	// Should stream ndjson items
	if !strings.Contains(stdout.String(), `"id"`) {
		t.Error("expected streamed items in output")
	}
}

func TestNormalisePath_StripsQueryAndFragment(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  string
		want string
	}{
		{"plain path", "/open-apis/test", "/open-apis/test"},
		{"with query", "/open-apis/test?admin=true", "/open-apis/test"},
		{"with fragment", "/open-apis/test#section", "/open-apis/test"},
		{"with both", "/open-apis/test?a=1#frag", "/open-apis/test"},
		{"full URL with query", "https://open.feishu.cn/open-apis/foo?bar=1", "/open-apis/foo"},
		{"short path with query", "contact/v3/users?page_size=50", "/open-apis/contact/v3/users"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := normalisePath(tt.raw)
			if got != tt.want {
				t.Errorf("normalisePath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestApiCmd_APIError_IsRaw(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-raw", AppSecret: "test-secret-raw", Brand: core.BrandFeishu,
	})

	// Return a permission error from the API
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/perm",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled for this app",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "calendar:calendar:readonly"},
				},
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test/perm", "--as", "bot"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for permission denied API response")
	}

	// Error should be marked Raw
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if !exitErr.Raw {
		t.Error("expected API error from api command to be marked Raw")
	}

	// Note: stderr envelope output is tested at the root level (TestHandleRootError_*)
	// since WriteErrorEnvelope is called by handleRootError, not by cobra's Execute.
	_ = stderr
}

func TestApiCmd_APIError_PreservesOriginalMessage(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-origmsg", AppSecret: "test-secret-origmsg", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/origmsg",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled for this app",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "im:message:readonly"},
				},
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test/origmsg", "--as", "bot"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	// The message should NOT have been enriched (no "App scope not enabled" replacement)
	if strings.Contains(exitErr.Error(), "App scope not enabled") {
		t.Error("expected original message, not enriched message")
	}
	// Detail should still contain the raw API error detail
	if exitErr.Detail == nil {
		t.Fatal("expected non-nil Detail")
	}
	if exitErr.Detail.Detail == nil {
		t.Error("expected raw Detail.Detail to be preserved (not cleared by enrichment)")
	}
}

func TestApiCmd_PageAll_APIError_IsRaw(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-rawpage", AppSecret: "test-secret-rawpage", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/rawpage",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "scope not enabled",
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test/rawpage", "--as", "bot", "--page-all"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if !exitErr.Raw {
		t.Error("expected paginated API error to be marked Raw")
	}
}

func TestApiCmd_JqFlag_Parsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--jq", ".data"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.JqExpr != ".data" {
		t.Errorf("expected JqExpr=.data, got %s", gotOpts.JqExpr)
	}
}

func TestApiCmd_JqFlag_ShortForm(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "-q", ".data"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.JqExpr != ".data" {
		t.Errorf("expected JqExpr=.data, got %s", gotOpts.JqExpr)
	}
}

func TestApiCmd_JqAndOutputConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--jq", ".data", "--output", "file.bin"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --jq + --output conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestApiCmd_JqFilter_AppliesExpression(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-jq", AppSecret: "test-secret-jq", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/jq",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test/jq", "--as", "bot", "--jq", ".data.items[].name"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("expected jq-filtered names, got: %s", out)
	}
	// Should NOT contain the full envelope structure
	if strings.Contains(out, `"code"`) {
		t.Errorf("expected jq to filter out envelope, got: %s", out)
	}
}

func TestApiCmd_JqAndFormatConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--jq", ".data", "--format", "ndjson"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --jq + --format ndjson conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestApiCmd_JqInvalidExpression(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--jq", "invalid["})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
	if !strings.Contains(err.Error(), "invalid jq expression") {
		t.Errorf("expected 'invalid jq expression' error, got: %v", err)
	}
}

func TestApiCmd_PageAll_WithJq(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pjq", AppSecret: "test-secret-pjq", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "u1"}, map[string]interface{}{"id": "u2"}},
				"has_more": false,
			},
		},
	})

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all", "--jq", ".data.items[].id"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "u1") || !strings.Contains(out, "u2") {
		t.Errorf("expected jq-filtered ids, got: %s", out)
	}
	if strings.Contains(out, `"code"`) {
		t.Errorf("expected jq to filter out envelope, got: %s", out)
	}
}

func TestApiCmd_MethodUppercase(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := NewCmdApi(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"post", "/test"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.Method != "POST" {
		t.Errorf("expected method POST (uppercased), got %s", gotOpts.Method)
	}
}
