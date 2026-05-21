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
				"app_id":     "app_x",
				"name":       "Demo",
				"icon_url":   "https://lf3-static.bytednsdoc.com/.../default.svg",
				"created_at": "2026-05-18T10:00:00Z",
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "HTML", "--description", "d", "--as", "user"},
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
	if sent["app_type"] != "HTML" {
		t.Fatalf("body.app_type = %v (want HTML)", sent["app_type"])
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
			"data": map[string]interface{}{"app_id": "app_x", "name": "Demo"},
		},
	})

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "HTML", "--icon-url", "https://example.com/icon.svg", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsCreate_RequiresName(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsCreate, []string{"+create", "--app-type", "HTML", "--as", "user"}, factory, stdout)
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
}

func TestAppsCreate_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "HTML", "--dry-run", "--as", "user"},
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
	if !strings.Contains(got, `"app_type": "HTML"`) {
		t.Fatalf("dry-run missing app_type: %s", got)
	}
}
