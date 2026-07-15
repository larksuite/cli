// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func newTestApiCmd(f *cmdutil.Factory, runF func(*APIOptions) error) *cobra.Command {
	cmd := NewCmdApi(f, runF)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return cmd
}

func newTestRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "lark-cli",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
}

func TestApiCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got["ok"] != true || got["identity"] != "bot" || got["dry_run"] != true {
		t.Fatalf("unexpected dry-run envelope: %#v", got)
	}
	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want object", got["data"])
	}
	api, ok := data["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Fatalf("api = %#v, want one call", data["api"])
	}
	call, ok := api[0].(map[string]interface{})
	if !ok || call["url"] != "/open-apis/test" {
		t.Fatalf("api[0] = %#v", api[0])
	}
	if strings.Contains(stdout.String(), "=== Dry Run ===") {
		t.Fatalf("stdout should not contain dry-run banner: %s", stdout.String())
	}
}

func TestApiCmd_DryRunWithJq(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--dry-run", "--jq", ".data.api[0].url"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "/open-apis/test" {
		t.Fatalf("jq output = %q, want /open-apis/test", got)
	}
}

// Regression: --params null parses to a nil map; writing page_size onto it must
// not panic. Symmetric to the typed-flag overlay path in cmd/service — both
// write into the map ParseJSONMap returns.
func TestApiCmd_NullParamsWithPageSize(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--params", "null", "--page-size", "50", "--as", "bot", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--params null with --page-size should not error, got: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "page_size") {
		t.Errorf("expected page_size applied over null --params, got:\n%s", out)
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

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if got["ok"] != true || got["identity"] != "bot" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, hasCode := got["code"]; hasCode {
		t.Fatalf("success envelope leaked outer code: %s", stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if !ok || data["result"] != "success" {
		t.Fatalf("data = %#v, want result=success", got["data"])
	}
}

func TestApiCmd_MissingArgs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET"}) // missing path
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for missing args")
	}
}

func TestApiCmd_EmptyMethodRejected(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"", "/open-apis/test", "--as", "bot", "--dry-run"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error for empty HTTP method")
	}
	if !strings.Contains(err.Error(), "method") {
		t.Fatalf("error should name the method argument, got: %v", err)
	}
}

func TestApiCmd_InvalidParamsJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
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

	cmd := newTestApiCmd(f, nil)
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

func TestNewCmdApi_StrictModeHidesAsFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, SupportedIdentities: 2,
	})

	cmd := newTestApiCmd(f, nil)
	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if !flag.Hidden {
		t.Fatal("expected --as flag to be hidden in strict mode")
	}
	if got := flag.DefValue; got != "bot" {
		t.Fatalf("default value = %q, want %q", got, "bot")
	}
}

func TestApiCmd_PageLimitDefault(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

func TestApiCmd_ParamsAndDataBothStdinConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/test", "--as", "bot", "--params", "-", "--data", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --params and --data use stdin")
	}
	if !strings.Contains(err.Error(), "cannot both read from stdin") {
		t.Errorf("expected stdin conflict error, got: %v", err)
	}
}

func TestApiCmd_OutputAndPageAllConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-bin", AppSecret: "test-secret-bin", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL:         "/open-apis/drive/v1/files/xxx/download",
		RawBody:     []byte("fake-binary-content"),
		ContentType: "application/octet-stream",
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/drive/v1/files/xxx/download", "--as", "bot"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "binary response detected") {
		t.Error("expected binary response hint in stderr")
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout:\n%s", err, stdout.String())
	}
	savedPath, _ := got["saved_path"].(string)
	if savedPath == "" {
		t.Fatalf("saved_path missing from output: %#v", got)
	}
	// The file must land inside the temporary cwd — this pins the isolation
	// contract: rolling back TestChdir would leave download.bin in the repo.
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := filepath.EvalSymlinks(filepath.Dir(savedPath))
	if err != nil {
		t.Fatalf("saved_path %q dir not resolvable: %v", savedPath, err)
	}
	if gotDir != wantDir {
		t.Errorf("saved_path %q is outside temp cwd %q", savedPath, wantDir)
	}
	content, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(content) != "fake-binary-content" {
		t.Errorf("saved file content = %q, want %q", content, "fake-binary-content")
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

	cmd := newTestApiCmd(f, nil)
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
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if got["ok"] != true || got["identity"] != "bot" || !ok || data["user_id"] != "u123" {
		t.Fatalf("unexpected fallback envelope: %#v", got)
	}
	if _, hasCode := got["code"]; hasCode {
		t.Fatalf("fallback success envelope leaked outer code: %s", stdout.String())
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
			"code": 230027, "msg": "user not authorized",
		},
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/im/v1/chats/oc_xxx/announcement", "--as", "bot", "--page-all"})
	err := cmd.Execute()
	// Should return an error
	if err == nil {
		t.Fatal("expected an error for non-zero code")
	}
	// Should still output the response body so user can see the error details
	if !strings.Contains(stdout.String(), "230027") {
		t.Errorf("expected error response in stdout, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "user not authorized") {
		t.Errorf("expected error message in stdout, got: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("unexpected success envelope on error path: %s", stdout.String())
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected PermissionError, got %T: %v", err, err)
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

	cmd := newTestApiCmd(f, nil)
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

func TestApiCmd_PageAll_StreamBusinessErrorDoesNotDumpJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-stream-err", AppSecret: "test-secret-pageall-stream-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "safe-page"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 230027, "msg": "user not authorized",
		},
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all", "--format", "ndjson"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code on later page")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	out := stdout.String()
	if !strings.Contains(out, "safe-page") {
		t.Fatalf("expected earlier successful page to remain streamed, got: %s", out)
	}
	if strings.Contains(out, "230027") || strings.Contains(out, "user not authorized") {
		t.Fatalf("streaming stdout should not contain raw error JSON, got: %s", out)
	}
	if strings.Contains(out, "\n  \"code\"") {
		t.Fatalf("streaming stdout should not contain indented JSON error dump, got: %s", out)
	}
}

func TestApiCmd_PageAll_BatchAPI_DefaultJSONEnvelope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-json", AppSecret: "test-secret-pageall-json", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}},
				"has_more": false,
			},
		},
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if got["ok"] != true || got["identity"] != "bot" || !ok {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, hasCode := got["code"]; hasCode {
		t.Fatalf("success envelope leaked outer code: %s", stdout.String())
	}
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("data.items = %#v, want one item", data["items"])
	}
}

type apiContentSafetyProvider struct {
	called bool
	path   string
	data   interface{}
	match  string
}

func (p *apiContentSafetyProvider) Name() string { return "api-test" }

func (p *apiContentSafetyProvider) Scan(_ context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	p.called = true
	p.path = req.Path
	p.data = req.Data
	if p.match != "" {
		b, _ := json.Marshal(req.Data)
		if !strings.Contains(string(b), p.match) {
			return nil, nil
		}
	}
	return &extcs.Alert{Provider: "api-test", MatchedRules: []string{"pagination"}}, nil
}

func TestApiCmd_PageAll_DefaultJSONRunsContentSafety(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	provider := &apiContentSafetyProvider{}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-safety", AppSecret: "test-secret-pageall-safety", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}},
				"has_more": false,
			},
		},
	})

	root := newTestRootCmd()
	root.AddCommand(newTestApiCmd(f, nil))
	root.SetArgs([]string{"api", "GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.called {
		t.Fatal("expected content safety provider to scan paginated output")
	}
	if provider.path != "api" {
		t.Fatalf("scan path = %q, want api", provider.path)
	}
	data, ok := provider.data.(map[string]interface{})
	if !ok {
		t.Fatalf("scanned data type = %T, want map", provider.data)
	}
	if _, hasCode := data["code"]; hasCode {
		t.Fatalf("scanned data should be business data only, got %#v", data)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	alert, ok := got["_content_safety_alert"].(map[string]interface{})
	if !ok || alert["provider"] != "api-test" {
		t.Fatalf("missing content safety alert in envelope: %#v", got)
	}
}

func TestApiCmd_PageAll_StreamFormatRunsContentSafety(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	provider := &apiContentSafetyProvider{}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-stream-safety", AppSecret: "test-secret-pageall-stream-safety", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}},
				"has_more": false,
			},
		},
	})

	root := newTestRootCmd()
	root.AddCommand(newTestApiCmd(f, nil))
	root.SetArgs([]string{"api", "GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all", "--format", "ndjson"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.called {
		t.Fatal("expected content safety provider to scan streamed paginated output")
	}
	if provider.path != "api" {
		t.Fatalf("scan path = %q, want api", provider.path)
	}
	items, ok := provider.data.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("scanned data = %#v, want one streamed item", provider.data)
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert from api-test") {
		t.Fatalf("expected content safety warning on stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id":"1"`) {
		t.Fatalf("expected streamed ndjson output, got: %s", stdout.String())
	}
}

func TestApiCmd_PageAll_StreamFormatBlockSkipsBlockedPage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	provider := &apiContentSafetyProvider{match: "blocked"}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-pageall-stream-block", AppSecret: "test-secret-pageall-stream-block", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "safe-page"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/contact/v3/users",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "blocked-page"}},
				"has_more": false,
			},
		},
	})

	root := newTestRootCmd()
	root.AddCommand(newTestApiCmd(f, nil))
	root.SetArgs([]string{"api", "GET", "/open-apis/contact/v3/users", "--as", "bot", "--page-all", "--format", "ndjson"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected content safety block error")
	}
	var safetyErr *errs.ContentSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected ContentSafetyError, got %T: %v", err, err)
	}
	if safetyErr.Category != errs.CategoryPolicy || safetyErr.Subtype != errs.SubtypeContentSafety {
		t.Fatalf("problem = %s/%s, want %s/%s", safetyErr.Category, safetyErr.Subtype, errs.CategoryPolicy, errs.SubtypeContentSafety)
	}
	if len(safetyErr.Rules) != 1 || safetyErr.Rules[0] != "pagination" {
		t.Fatalf("rules = %v, want [pagination]", safetyErr.Rules)
	}
	out := stdout.String()
	if !strings.Contains(out, "safe-page") {
		t.Fatalf("expected earlier safe page to remain streamed, got: %s", out)
	}
	if strings.Contains(out, "blocked-page") {
		t.Fatalf("blocked page was written before safety block: %s", out)
	}
}

func requireProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype, code int) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype || p.Code != code {
		t.Fatalf("problem = %s/%s/%d, want %s/%s/%d", p.Category, p.Subtype, p.Code, category, subtype, code)
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

func TestApiCmd_JqFlag_Parsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

	cmd := newTestApiCmd(f, nil)
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

	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

	cmd := newTestApiCmd(f, nil)
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
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
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

func TestApiCmd_FileFlagParsing(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"POST", "/open-apis/test", "--file", "image=photo.jpg", "--data", `{"image_type":"message"}`})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotOpts.File != "image=photo.jpg" {
		t.Errorf("expected File = %q, got %q", "image=photo.jpg", gotOpts.File)
	}
}

func TestApiCmd_FileAndOutputConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"POST", "/open-apis/test", "--as", "bot", "--file", "photo.jpg", "--output", "out.json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --file with --output")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual exclusion error, got: %v", err)
	}
}

func TestApiCmd_FileWithGET(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--as", "bot", "--file", "photo.jpg"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --file with GET")
	}
	if !strings.Contains(err.Error(), "requires POST") {
		t.Errorf("expected method error, got: %v", err)
	}
}

func TestApiCmd_FileStdinConflictWithData(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
		return apiRun(opts)
	})
	cmd.SetArgs([]string{"POST", "/open-apis/test", "--as", "bot", "--file", "-", "--data", "-"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --file stdin with --data stdin")
	}
	if !strings.Contains(err.Error(), "cannot both read from stdin") {
		t.Errorf("expected stdin conflict error, got: %v", err)
	}
}

func TestApiCmd_DryRunWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.jpg"
	if err := os.WriteFile(tmpFile, []byte("fake-image"), 0600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/im/v1/images", "--file", "image=" + tmpFile, "--data", `{"image_type":"message"}`, "--dry-run", "--as", "bot"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, out)
	}
	if env["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", env["dry_run"])
	}
	data := env["data"].(map[string]interface{})
	api := data["api"].([]interface{})
	call := api[0].(map[string]interface{})
	body := call["body"].(map[string]interface{})
	file := body["file"].(map[string]interface{})
	if file["field"] != "image" || file["path"] != tmpFile {
		t.Fatalf("unexpected file dry-run body: %#v", body)
	}
	if strings.Contains(out, "=== Dry Run ===") {
		t.Fatalf("stdout should not contain dry-run banner: %s", out)
	}
}

// TestApiCmd_PermissionError_DerivesFirstClassFields pins that when a Lark
// API returns a missing-scope failure, the typed *errs.PermissionError
// surfaced by `lark-cli api` lifts the diagnostic signals BuildAPIError
// consumed during classification into first-class wire fields
// (MissingScopes, LogID, ConsoleURL). The wire shape is the typed envelope
// — there is no raw-payload passthrough; new Lark diagnostic fields require
// a CLI release.
func TestApiCmd_PermissionError_DerivesFirstClassFields(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_test_perm", AppSecret: "secret", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/docx/v1/documents/test",
		Body: map[string]interface{}{
			"code":   99991679,
			"msg":    "scope missing",
			"log_id": "20260527-test-log",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "docx:document"},
				},
			},
		},
	})

	cmd := newTestApiCmd(f, nil)
	cmd.SetArgs([]string{"GET", "/open-apis/docx/v1/documents/test", "--as", "bot"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}

	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}

	if len(pe.MissingScopes) != 1 || pe.MissingScopes[0] != "docx:document" {
		t.Errorf("MissingScopes = %v, want [docx:document]", pe.MissingScopes)
	}
	if pe.LogID != "20260527-test-log" {
		t.Errorf("LogID = %q, want %q", pe.LogID, "20260527-test-log")
	}
}

func TestApiCmd_JsonFlag_Accepted(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	var gotOpts *APIOptions
	cmd := newTestApiCmd(f, func(opts *APIOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"GET", "/open-apis/test", "--json"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("--json should be accepted without error, got: %v", err)
	}
	if gotOpts.Method != "GET" {
		t.Errorf("expected method GET, got %s", gotOpts.Method)
	}
}

// parseMultipartFilenames drives one api --file upload through the mock
// transport and returns a map of field name -> part filename parsed from the
// captured multipart body, plus the map of text form fields. It fails the test
// if the captured request is not multipart/form-data.
func parseMultipartFilenames(t *testing.T, stub *httpmock.Stub) (map[string]string, map[string]string) {
	t.Helper()
	ct := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("parse Content-Type %q: %v", ct, err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("Content-Type = %q, want multipart/*", mediaType)
	}
	filenames := map[string]string{}
	fields := map[string]string{}
	mr := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		if fn := part.FileName(); fn != "" {
			filenames[part.FormName()] = fn
		} else {
			buf := &bytes.Buffer{}
			_, _ = buf.ReadFrom(part)
			fields[part.FormName()] = buf.String()
		}
	}
	return filenames, fields
}

func TestApiCmd_FileUpload_PreservesFilename(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "invoice.pdf"), []byte("%PDF-1.4 fake"), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stub := &httpmock.Stub{
		URL:  "/open-apis/approval/v4/files/upload",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"code": "file_xxx"}},
	}
	reg.Register(stub)

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/approval/v4/files/upload", "--as", "bot", "--file", "invoice.pdf"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filenames, _ := parseMultipartFilenames(t, stub)
	if got := filenames["file"]; got != "invoice.pdf" {
		t.Fatalf("part filename for field %q = %q, want %q", "file", got, "invoice.pdf")
	}
}

func TestApiCmd_FileUpload_FieldPrefixKeepsBasename(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "invoice.pdf"), []byte("%PDF-1.4 fake"), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stub := &httpmock.Stub{
		URL:  "/open-apis/approval/v4/files/upload",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"code": "file_xxx"}},
	}
	reg.Register(stub)

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/approval/v4/files/upload", "--as", "bot", "--file", "upload=sub/invoice.pdf"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filenames, _ := parseMultipartFilenames(t, stub)
	if _, ok := filenames["upload"]; !ok {
		t.Fatalf("expected field name %q from field=path form, got fields %v", "upload", filenames)
	}
	if got := filenames["upload"]; got != "invoice.pdf" {
		t.Fatalf("part filename for field %q = %q, want %q (basename only)", "upload", got, "invoice.pdf")
	}
}

func TestApiCmd_FileUpload_WithDataFields(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "invoice.pdf"), []byte("%PDF-1.4 fake"), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stub := &httpmock.Stub{
		URL:  "/open-apis/approval/v4/files/upload",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"code": "file_xxx"}},
	}
	reg.Register(stub)

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/approval/v4/files/upload", "--as", "bot",
		"--file", "invoice.pdf", "--data", `{"type":"attachment"}`})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filenames, fields := parseMultipartFilenames(t, stub)
	if got := filenames["file"]; got != "invoice.pdf" {
		t.Fatalf("part filename = %q, want %q", got, "invoice.pdf")
	}
	if got := fields["type"]; got != "attachment" {
		t.Fatalf("text field type = %q, want %q", got, "attachment")
	}
}

func TestApiCmd_FileUpload_StdinFallsBackToUnknown(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	f.IOStreams.In = bytes.NewReader([]byte("stdin-bytes"))

	stub := &httpmock.Stub{
		URL:  "/open-apis/approval/v4/files/upload",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"code": "file_xxx"}},
	}
	reg.Register(stub)

	cmd := NewCmdApi(f, nil)
	cmd.SetArgs([]string{"POST", "/open-apis/approval/v4/files/upload", "--as", "bot", "--file", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filenames, _ := parseMultipartFilenames(t, stub)
	if got := filenames["file"]; got != "unknown-file" {
		t.Fatalf("stdin part filename = %q, want %q (no stable local name, fallback)", got, "unknown-file")
	}
}
