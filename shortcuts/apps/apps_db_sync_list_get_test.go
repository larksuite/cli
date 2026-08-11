// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsDBSyncList_DryRunDefaultOmitsEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncList,
		[]string{"+db-sync-list", "--app-id", "app_x", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "GET" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/db/sync_list" {
		t.Fatalf("dry-run method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if _, ok := env.API[0].Params["env"]; ok {
		t.Fatalf("default dry-run should omit env, got params=%v", env.API[0].Params)
	}
	if got := int(env.API[0].Params["page_size"].(float64)); got != 20 {
		t.Fatalf("page_size = %d, want 20", got)
	}
}

func TestAppsDBSyncList_DryRunWithEnvFilterAndPagination(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncList,
		[]string{
			"+db-sync-list", "--app-id", "app_x", "--environment", "dev",
			"--mode", "batch", "--status", "succeeded", "--table", "customers",
			"--page-size", "50", "--page-token", "next", "--dry-run", "--as", "user",
		},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	params := env.API[0].Params
	for k, want := range map[string]interface{}{
		"env":        "dev",
		"mode":       "batch",
		"status":     "succeeded",
		"table":      "customers",
		"page_token": "next",
	} {
		if params[k] != want {
			t.Fatalf("params[%s] = %v, want %v; all params=%v", k, params[k], want, params)
		}
	}
	if got := int(params["page_size"].(float64)); got != 50 {
		t.Fatalf("page_size = %d, want 50", got)
	}
}

func TestAppsDBSyncList_RejectsBadPageSizeAndMode(t *testing.T) {
	t.Run("bad page size", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBSyncList,
			[]string{"+db-sync-list", "--app-id", "app_x", "--page-size", "0", "--as", "user"},
			factory, stdout)
		requireAppsValidationProblem(t, err)
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected validation error, got %T: %v", err, err)
		}
		if ve.Param != "--page-size" {
			t.Fatalf("param = %q, want --page-size", ve.Param)
		}
	})

	t.Run("bad mode", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBSyncList,
			[]string{"+db-sync-list", "--app-id", "app_x", "--mode", "full", "--as", "user"},
			factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "invalid value") {
			t.Fatalf("expected mode enum error, got %v", err)
		}
		if !strings.Contains(err.Error(), "batch") || !strings.Contains(err.Error(), "streaming") {
			t.Fatalf("mode enum error should list allowed values, got %v", err)
		}
	})
}

func TestAppsDBSyncList_ExecuteSuccessPreservesPaginationAndPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/db/sync_list",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"task_id":    "batch_1",
						"mode":       "batch",
						"status":     "succeeded",
						"source":     map[string]interface{}{"table": map[string]interface{}{"name": "orders"}},
						"target":     map[string]interface{}{"table": map[string]interface{}{"name": "orders_pg"}},
						"created_at": "2026-08-03T10:00:00Z",
					},
				},
				"page_token": "next",
				"has_more":   true,
			},
		},
	})

	if err := runAppsShortcut(t, AppsDBSyncList,
		[]string{"+db-sync-list", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"task_id": "batch_1"`, `"page_token": "next"`, `"has_more": true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("json output missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/db/sync_list",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"task_id":    "streaming_1",
						"mode":       "streaming",
						"status":     "enabled",
						"source":     map[string]interface{}{"table": map[string]interface{}{"name": "customers"}},
						"created_at": "2026-08-03T11:00:00Z",
					},
				},
				"page_token": "cursor_2",
				"has_more":   true,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBSyncList,
		[]string{"+db-sync-list", "--app-id", "app_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("pretty execute err=%v", err)
	}
	got = stdout.String()
	for _, want := range []string{"task_id", "streaming_1", "customers", "1 total (more available, page_token: cursor_2)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
}

func TestAppsDBSyncGet_DryRunPathNoEnv(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncGet,
		[]string{"+db-sync-get", "--app-id", "app_x", "--task-id", "streaming_1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "GET" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/db/sync_task" {
		t.Fatalf("dry-run method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Params["task_id"] != "streaming_1" {
		t.Fatalf("get dry-run should send task_id query, got %v", env.API[0].Params)
	}
	if _, ok := env.API[0].Params["env"]; ok {
		t.Fatalf("get dry-run should not send env, got %v", env.API[0].Params)
	}
}

func TestAppsDBSyncGet_ExecutePreservesWarningsAndPretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	response := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"task_id":        "streaming_1",
			"mode":           "streaming",
			"status":         "enabled",
			"source":         map[string]interface{}{"table": map[string]interface{}{"name": "customers"}},
			"target":         map[string]interface{}{"table": map[string]interface{}{"name": "customers_pg"}},
			"created_at":     "2026-08-03T10:00:00Z",
			"last_synced_at": "2026-08-03T11:00:00Z",
			"warnings": []interface{}{
				map[string]interface{}{
					"code":         "schema_changed",
					"message":      "target schema changed",
					"target_table": "customers_pg",
					"hint":         "update mapping",
				},
			},
		},
	}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/db/sync_task", Body: response})

	if err := runAppsShortcut(t, AppsDBSyncGet,
		[]string{"+db-sync-get", "--app-id", "app_x", "--task-id", "streaming_1", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"warnings"`, `"schema_changed"`, `"target schema changed"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("json output missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/db/sync_task", Body: response})
	if err := runAppsShortcut(t, AppsDBSyncGet,
		[]string{"+db-sync-get", "--app-id", "app_x", "--task-id", "streaming_1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("pretty execute err=%v", err)
	}
	got = stdout.String()
	for _, want := range []string{"task_id:", "streaming_1", "status:", "enabled", "source:", "customers", "target:", "customers_pg", "warnings:", "schema_changed", "update mapping"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
}

func TestAppsDBSyncGet_BatchPrettyRendersSchemaOnlyAndStatistics(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	response := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"task_id":     "batch_1",
			"mode":        "batch",
			"status":      "done",
			"source":      map[string]interface{}{"table": map[string]interface{}{"name": "orders"}},
			"target":      map[string]interface{}{"table": map[string]interface{}{"name": "orders_pg"}},
			"created_at":  "2026-08-03T10:00:00Z",
			"schema_only": false,
			"statistics":  map[string]interface{}{"rows": float64(10), "columns": float64(3)},
		},
	}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/db/sync_task", Body: response})

	if err := runAppsShortcut(t, AppsDBSyncGet,
		[]string{"+db-sync-get", "--app-id", "app_x", "--task-id", "batch_1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("pretty execute err=%v", err)
	}
	got := stdout.String()
	// schema_only renders as a bare bool, not fmt.Sprint's default; statistics
	// renders as deterministic key=value pairs, not Go's "map[...]" syntax.
	for _, want := range []string{"schema_only:", "false", "statistics:", "columns=3 rows=10"} {
		if !strings.Contains(got, want) {
			t.Fatalf("batch pretty output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "map[") {
		t.Fatalf("statistics must not render as Go map syntax:\n%s", got)
	}
}

func TestAppsDBSyncGet_TaskNotFoundGetsListHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/db/sync_task",
		Body: map[string]interface{}{
			"code": 400002480,
			"msg":  "task not found",
		},
	})

	err := runAppsShortcut(t, AppsDBSyncGet,
		[]string{"+db-sync-get", "--app-id", "app_x", "--task-id", "missing", "--as", "user"},
		factory, stdout)
	p := requireAppsAPIProblem(t, err)
	if p.Code != 400002480 {
		t.Fatalf("code = %d, want 400002480", p.Code)
	}
	if !strings.Contains(p.Hint, "+db-sync-list") {
		t.Fatalf("hint should mention +db-sync-list, got %q", p.Hint)
	}
}

func TestAppsDBSyncList_TaskNotFoundGetsListHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/db/sync_list",
		Body: map[string]interface{}{
			"code": 400002480,
			"msg":  "task not found",
		},
	})

	err := runAppsShortcut(t, AppsDBSyncList,
		[]string{"+db-sync-list", "--app-id", "app_x", "--as", "user"},
		factory, stdout)
	p := requireAppsAPIProblem(t, err)
	if p.Code != 400002480 {
		t.Fatalf("code = %d, want 400002480", p.Code)
	}
	if !strings.Contains(p.Hint, "+db-sync-list") {
		t.Fatalf("hint should mention +db-sync-list, got %q", p.Hint)
	}
	if p.Category != errs.CategoryAPI {
		t.Fatalf("category = %q, want api", p.Category)
	}
}
