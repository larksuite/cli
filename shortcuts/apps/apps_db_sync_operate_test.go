// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	dbSyncEnableURL  = "/open-apis/spark/v1/apps/app_x/db/sync_enable"
	dbSyncDisableURL = "/open-apis/spark/v1/apps/app_x/db/sync_disable"
	dbSyncDeleteURL  = "/open-apis/spark/v1/apps/app_x/db/sync_del"
)

func TestAppsDBSyncEnableDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncEnable,
		[]string{"+db-sync-enable", "--app-id", "app_x", "--task-id", "streaming_1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != dbSyncEnableURL {
		t.Fatalf("dry-run = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["task_id"] != "streaming_1" {
		t.Fatalf("dry-run body = %v", env.API[0].Body)
	}
}

func TestAppsDBSyncDisableDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncDisable,
		[]string{"+db-sync-disable", "--app-id", "app_x", "--task-id", "streaming_1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != dbSyncDisableURL {
		t.Fatalf("dry-run = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["task_id"] != "streaming_1" {
		t.Fatalf("dry-run body = %v", env.API[0].Body)
	}
}

func TestAppsDBSyncDeleteDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncDelete,
		[]string{"+db-sync-delete", "--app-id", "app_x", "--task-id", "streaming_1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != dbSyncDeleteURL {
		t.Fatalf("dry-run = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["task_id"] != "streaming_1" {
		t.Fatalf("dry-run body = %v", env.API[0].Body)
	}
}

func TestAppsDBSyncEnableExecute(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    dbSyncEnableURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"task_id": "streaming_1",
			"mode":    "streaming",
			"status":  "active",
		}},
	})
	if err := runAppsShortcut(t, AppsDBSyncEnable,
		[]string{"+db-sync-enable", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"task_id": "streaming_1"`) || !strings.Contains(got, `"status": "active"`) {
		t.Fatalf("stdout missing task/status:\n%s", got)
	}
}

func TestAppsDBSyncDisableExecute(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    dbSyncDisableURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"task_id": "streaming_1",
			"mode":    "streaming",
			"status":  "disabled",
		}},
	})
	if err := runAppsShortcut(t, AppsDBSyncDisable,
		[]string{"+db-sync-disable", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"status": "disabled"`) {
		t.Fatalf("stdout missing disabled status:\n%s", stdout.String())
	}
}

func TestAppsDBSyncDeleteExecute(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    dbSyncDeleteURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"task_id": "streaming_1",
			"mode":    "streaming",
			"deleted": true,
		}},
	})
	if err := runAppsShortcut(t, AppsDBSyncDelete,
		[]string{"+db-sync-delete", "--app-id", "app_x", "--task-id", "streaming_1", "--yes", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"deleted": true`) {
		t.Fatalf("stdout missing deleted=true:\n%s", stdout.String())
	}
}

func TestAppsDBSyncDeleteRequiresConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBSyncDelete,
		[]string{"+db-sync-delete", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "requires confirmation") {
		t.Fatalf("expected confirmation_required, got %v", err)
	}
}

func TestAppsDBSyncEnableNoConfirmationRequired(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    dbSyncEnableURL,
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"status": "active"}},
	})
	if err := runAppsShortcut(t, AppsDBSyncEnable,
		[]string{"+db-sync-enable", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("enable should not require --yes, got %v", err)
	}
}

func TestAppsDBSyncOperateErrorCarriesOperationHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    dbSyncEnableURL,
		Body: map[string]interface{}{
			"code": 400002479,
			"msg":  "Operation 'enable' is only available for streaming tasks.",
		},
	})
	err := runAppsShortcut(t, AppsDBSyncEnable,
		[]string{"+db-sync-enable", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected API error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T %v", err, err)
	}
	if p.Code != 400002479 || !strings.Contains(p.Hint, "+db-sync-get") {
		t.Fatalf("problem = %+v, want operation-not-allowed hint", p)
	}
}
