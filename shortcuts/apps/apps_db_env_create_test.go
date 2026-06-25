// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsDBEnvCreate_WithYesPostsSyncData(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/db_dev_init", // URL 仍走 db_dev_init，CLI 命令名 +db-env-create
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"status":       "initialized",
				"environments": []interface{}{"dev", "online"},
				"data_synced":  true,
			},
		},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "dev", "--sync-data", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["sync_data"] != true {
		t.Fatalf("body.sync_data = %v (want true)", sent["sync_data"])
	}
	if !strings.Contains(stdout.String(), "initialized") {
		t.Fatalf("stdout should include status, got %s", stdout.String())
	}
}

// 不传 --sync-data（默认）→ body.sync_data=false
func TestAppsDBEnvCreate_SyncDataFalseByDefault(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/db_dev_init",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": "initialized"}},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "dev", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["sync_data"] != false {
		t.Fatalf("body.sync_data = %v (want false by default)", sent["sync_data"])
	}
}

func TestAppsDBEnvCreate_PrettyEmitsAllFourLines(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/db_dev_init",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"status":       "initialized",
				"environments": []interface{}{"dev", "online"},
				"data_synced":  true,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "dev", "--sync-data", "--yes", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	wantLines := []string{
		"✓ Multi-env initialized",
		"Environments: dev, online",
		"Data synced: yes",
		"Note: structure changes in dev now need to be released to online.",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("pretty output missing line %q\ngot:\n%s", line, got)
		}
	}
}

func TestAppsDBEnvCreate_DryRunNoConfirm(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "dev", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/db_dev_init") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
}

// --env 只接受 dev：传 online 应被 enum 校验拒绝。
func TestAppsDBEnvCreate_RejectsNonDevEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBEnvCreate,
		[]string{"+db-env-create", "--app-id", "app_x", "--environment", "online", "--yes", "--as", "user"},
		factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "env") {
		t.Fatalf("expected env enum rejection, got %v", err)
	}
}
