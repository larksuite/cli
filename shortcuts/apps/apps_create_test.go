// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// 测试基础设施 —— 后续 Task 2.2-2.4 / Task 3.4 复用

func newAppsExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, cfg)
	return factory, stdout, reg
}

func runAppsShortcut(t *testing.T, sc common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "apps"}
	sc.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.ExecuteContext(context.Background())
}

// +create 测试

func TestAppsCreate_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":     "app_x",
					"name":       "Demo",
					"icon_url":   "https://lf3-static.bytednsdoc.com/.../default.svg",
					"created_at": "2026-05-18T10:00:00Z",
				},
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "html", "--description", "d", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"app_id": "app_x"`) {
		t.Fatalf("stdout missing app_id: %s", got)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["name"] != "Demo" {
		t.Fatalf("body.name = %v", sent["name"])
	}
	if sent["app_type"] != "html" {
		t.Fatalf("body.app_type = %v (want html)", sent["app_type"])
	}
	if sent["description"] != "d" {
		t.Fatalf("body.description = %v", sent["description"])
	}
	if _, present := sent["icon_url"]; present {
		t.Fatalf("icon_url should be omitted when not provided: %v", sent)
	}
}

func TestAppsCreate_WithIconURL(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x", "name": "Demo"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "html", "--icon-url", "https://example.com/icon.svg", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

// TestAppsCreate_PrettyOutputReadsNestedAppID exercises the prettyFn callback
// passed to OutFormat (only invoked under --format pretty) so the new
// data.app.app_id nesting is actually read by the text writer. Without this,
// default --format json dumps the whole envelope and the substring assertion
// in TestAppsCreate_Success would pass even if the GetString path were wrong.
func TestAppsCreate_PrettyOutputReadsNestedAppID(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x", "name": "Demo"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "html", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "created: app_x") {
		t.Fatalf("pretty output should read app_id from data.app.app_id, got: %q", got)
	}
}

func TestAppsCreate_RequiresName(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsCreate, []string{"+create", "--app-type", "html", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected name required error, got %v", err)
	}
}

func TestAppsCreate_RequiresAppType(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-type") {
		t.Fatalf("expected --app-type required error, got %v", err)
	}
}

func TestAppsCreate_RejectsInvalidAppType(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "spa", "--as", "user"},
		factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported app-type error, got %v", err)
	}
	if !strings.Contains(err.Error(), "full_stack") {
		t.Fatalf("expected allow-list error to mention \"full_stack\", got %v", err)
	}
}

// newAppsCreateRuntime builds a RuntimeContext whose flags carry the given
// --name / --app-type values, so Validate and buildAppsCreateBody can be
// exercised directly without wiring a full cobra execution.
func newAppsCreateRuntime(t *testing.T, name, appType string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "+create"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("app-type", "", "")
	cmd.Flags().String("description", "", "")
	cmd.Flags().String("icon-url", "", "")
	if err := cmd.Flags().Set("name", name); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if err := cmd.Flags().Set("app-type", appType); err != nil {
		t.Fatalf("set app-type: %v", err)
	}
	return common.TestNewRuntimeContext(cmd, &core.CliConfig{})
}

// TestAppsCreateAppTypeNormalization verifies --app-type is normalized
// (trimmed + lowercased) before validation and before being sent in the
// request body. The server has historically accepted any case; the CLI
// canonicalizes to the lowercase enum (the documented external form). Values
// like "react" that are not in the enum still fail validation.
func TestAppsCreateAppTypeNormalization(t *testing.T) {
	pass := []struct {
		input string
		want  string
	}{
		{"html", "html"},
		{"HTML", "html"},
		{"Html", "html"},
		{"  html  ", "html"},
		{"FULL_STACK", "full_stack"},
		{"full_stack", "full_stack"},
	}
	for _, tc := range pass {
		t.Run("pass/"+tc.input, func(t *testing.T) {
			rctx := newAppsCreateRuntime(t, "Demo", tc.input)
			if err := AppsCreate.Validate(context.Background(), rctx); err != nil {
				t.Fatalf("Validate(%q) unexpected error: %v", tc.input, err)
			}
			body := buildAppsCreateBody(rctx)
			if body["app_type"] != tc.want {
				t.Fatalf("buildAppsCreateBody app_type = %v, want %q", body["app_type"], tc.want)
			}
		})
	}

	t.Run("reject/react", func(t *testing.T) {
		rctx := newAppsCreateRuntime(t, "Demo", "react")
		err := AppsCreate.Validate(context.Background(), rctx)
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("expected unsupported app-type error for \"react\", got %v", err)
		}
	})
}

func TestAppsCreate_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "html", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, `"name": "Demo"`) {
		t.Fatalf("dry-run missing body: %s", got)
	}
	if !strings.Contains(got, `"app_type": "html"`) {
		t.Fatalf("dry-run missing app_type: %s", got)
	}
}

func TestAppsCreate_FullstackSuccess(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_fs", "name": "Demo"},
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "full_stack", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["app_type"] != "full_stack" {
		t.Fatalf("body.app_type = %v (want full_stack)", sent["app_type"])
	}
	if _, present := sent["message"]; present {
		t.Fatalf("message should never be sent: %v", sent)
	}
}

func TestAppsCreate_FullstackDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "full_stack", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"app_type": "full_stack"`) {
		t.Fatalf("dry-run missing app_type full_stack: %s", got)
	}
	if strings.Contains(got, `"message"`) {
		t.Fatalf("dry-run should not contain message: %s", got)
	}
}
