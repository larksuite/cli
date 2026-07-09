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

	"github.com/larksuite/cli/errs"
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

func requireAppsProblem(t *testing.T, err error, category errs.Category) *errs.Problem {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != category {
		t.Fatalf("error category = %q, want %q", p.Category, category)
	}
	return p
}

func requireAppsValidationProblem(t *testing.T, err error) *errs.Problem {
	t.Helper()
	return requireAppsProblem(t, err, errs.CategoryValidation)
}

func requireAppsAPIProblem(t *testing.T, err error) *errs.Problem {
	t.Helper()
	return requireAppsProblem(t, err, errs.CategoryAPI)
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

// TestAppsCreate_RejectsInvalidAppType pins that --app-type is a strict
// lowercase enum (html / full_stack). Unknown values and legacy uppercase are
// both rejected by the flag's Enum — the CLI does not normalize case; legacy
// uppercase compatibility is a server-side concern, not surfaced by the client.
func TestAppsCreate_RejectsInvalidAppType(t *testing.T) {
	for _, appType := range []string{"spa", "HTML", "Full_Stack"} {
		t.Run(appType, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsCreate,
				[]string{"+create", "--name", "Demo", "--app-type", appType, "--as", "user"},
				factory, stdout)
			if err == nil || !strings.Contains(err.Error(), "invalid value") {
				t.Fatalf("expected invalid-enum error for %q, got %v", appType, err)
			}
			if !strings.Contains(err.Error(), "full_stack") {
				t.Fatalf("expected enum error to list allowed values, got %v", err)
			}
		})
	}
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
