// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestAppTablesPath_ReusesExistingURL(t *testing.T) {
	if got := appTablesPath("app_x"); got != "/open-apis/spark/v1/apps/app_x/tables" {
		t.Fatalf("appTablesPath = %q (want existing /apps/{id}/tables, not /db/tables)", got)
	}
}

func TestAppTablePath_EncodesSegments(t *testing.T) {
	if got := appTablePath("app_x", "my table"); got != "/open-apis/spark/v1/apps/app_x/tables/my%20table" {
		t.Fatalf("appTablePath = %q", got)
	}
}

func TestAppSQLPath_ReusesExistingURL(t *testing.T) {
	if got := appSQLPath("app_x"); got != "/open-apis/spark/v1/apps/app_x/sql_commands" {
		t.Fatalf("appSQLPath = %q (want /apps/{id}/sql_commands)", got)
	}
}

func TestAppDbEnvCreatePath_NewURL(t *testing.T) {
	// db-env-create 是本期新增接口，URL 走 /db_dev_init（与上面三条复用 URL 不同）。
	if got := appDbEnvCreatePath("app_x"); got != "/open-apis/spark/v1/apps/app_x/db_dev_init" {
		t.Fatalf("appDbEnvCreatePath = %q", got)
	}
}

func TestRequireAppID_BlankRejected(t *testing.T) {
	if _, err := requireAppID("   "); err == nil {
		t.Fatal("expected error for blank app-id")
	}
	got, err := requireAppID("  app_x  ")
	if err != nil || got != "app_x" {
		t.Fatalf("requireAppID trimmed = %q err=%v", got, err)
	}
}

func TestAppDbSyncPaths(t *testing.T) {
	if got := appDbSyncCreatePath("app x"); got != "/open-apis/spark/v1/apps/app%20x/db/sync_create" {
		t.Fatalf("appDbSyncCreatePath = %q", got)
	}
	if got := appDbSyncTaskPath("app_x"); got != "/open-apis/spark/v1/apps/app_x/db/sync_task" {
		t.Fatalf("appDbSyncTaskPath = %q", got)
	}
	if got := appDbSyncActionPath("app_x", "enable"); got != "/open-apis/spark/v1/apps/app_x/db/sync_enable" {
		t.Fatalf("appDbSyncActionPath = %q", got)
	}
	if got := appDbSyncDeletePath("app_x"); got != "/open-apis/spark/v1/apps/app_x/db/sync_del" {
		t.Fatalf("appDbSyncDeletePath = %q", got)
	}
}

func TestDBSyncConfig(t *testing.T) {
	validWithoutFieldMaps := `{
		"mode": "batch",
		"source": {"type": "base"},
		"target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}}
	}`
	validWithFieldMaps := `{
		"mode": "streaming",
		"source": {"type": "base"},
		"target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
		"field_maps": [{"source": "record_id", "target": "record_id", "enabled": true}]
	}`

	t.Run("preview allows omitted field maps", func(t *testing.T) {
		cfg, err := parseDBSyncConfigFlag(validWithoutFieldMaps, false)
		if err != nil {
			t.Fatalf("parseDBSyncConfigFlag preview = %v", err)
		}
		if cfg["mode"] != "batch" {
			t.Fatalf("mode = %v", cfg["mode"])
		}
	})

	t.Run("commit requires enabled field map", func(t *testing.T) {
		assertDBSyncConfigValidation(t, validWithoutFieldMaps, true, "field_maps")
		disabledOnly := `{
			"mode": "batch",
			"source": {"type": "base"},
			"target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
			"field_maps": [{"source": "a", "target": "b", "enabled": false}]
		}`
		assertDBSyncConfigValidation(t, disabledOnly, true, "field_maps")
		if _, err := parseDBSyncConfigFlag(validWithFieldMaps, true); err != nil {
			t.Fatalf("parseDBSyncConfigFlag valid field_maps = %v", err)
		}
	})

	t.Run("must be JSON object", func(t *testing.T) {
		assertDBSyncConfigValidation(t, `[1,2,3]`, false, "JSON object")
		assertDBSyncConfigValidation(t, `{`, false, "JSON object")
		assertDBSyncConfigValidation(t, validWithoutFieldMaps+` {}`, false, "one JSON object")
	})

	t.Run("singular map keys are rejected", func(t *testing.T) {
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"source"`, `"field_map": [], "source"`, 1), false, "field_maps")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"source"`, `"option_mapping": {}, "source"`, 1), false, "option_mappings")
		nestedSingular := `{
			"mode": "streaming",
			"source": {"type": "base"},
			"target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
			"field_maps": [{"source": "record_id", "target": "record_id", "option_mapping": []}]
		}`
		assertDBSyncConfigValidation(t, nestedSingular, true, "option_mappings")
	})

	t.Run("mode and endpoints are constrained", func(t *testing.T) {
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"batch"`, `"full"`, 1), false, "mode")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"base"`, `"sheet"`, 1), false, "source.type")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"postgresql"`, `"mysql"`, 1), false, "target.type")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"orders"`, `""`, 1), false, "target.table.name")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"create"`, `"drop"`, 1), false, "target.table.action")
	})

	t.Run("schema only is batch create only", func(t *testing.T) {
		validSchemaOnly := strings.Replace(validWithoutFieldMaps, `"mode": "batch"`, `"schema_only": true, "mode": "batch"`, 1)
		if _, err := parseDBSyncConfigFlag(validSchemaOnly, false); err != nil {
			t.Fatalf("parseDBSyncConfigFlag valid schema_only = %v", err)
		}
		streamingSchemaOnly := strings.Replace(validWithFieldMaps, `"mode": "streaming"`, `"schema_only": true, "mode": "streaming"`, 1)
		assertDBSyncConfigValidation(t, streamingSchemaOnly, false, "schema_only")
		useExistingSchemaOnly := strings.Replace(validSchemaOnly, `"create"`, `"use_existing"`, 1)
		assertDBSyncConfigValidation(t, useExistingSchemaOnly, false, "schema_only")
	})
}

func TestDBSyncHint(t *testing.T) {
	t.Run("nil and untyped errors pass through", func(t *testing.T) {
		if got := withDBSyncHint(nil, "fallback"); got != nil {
			t.Fatalf("withDBSyncHint(nil) = %v", got)
		}
		plain := errors.New("plain")
		if got := withDBSyncHint(plain, "fallback"); got != plain {
			t.Fatalf("withDBSyncHint(plain) = %v, want original", got)
		}
	})

	t.Run("known code gets mapped hint", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeNotFound, "target missing").WithCode(500002789).WithLogID("log_x")
		out := withDBSyncHint(in, "fallback")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("withDBSyncHint returned untyped error: %T", out)
		}
		if !strings.Contains(p.Hint, "target.table.action") {
			t.Fatalf("hint = %q, want target-table recovery", p.Hint)
		}
		if p.Code != 500002789 || p.LogID != "log_x" || p.Subtype != errs.SubtypeNotFound {
			t.Fatalf("problem metadata mutated: %+v", p)
		}
	})

	t.Run("existing hint is preserved", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeInvalidParameters, "bad mapping").WithCode(500002783).WithHint("server hint")
		out := withDBSyncHint(in, "fallback")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "server hint" {
			t.Fatalf("hint = %q, want existing hint", p.Hint)
		}
	})

	t.Run("unknown code uses fallback", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(42)
		out := withDBSyncHint(in, "fallback")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "fallback" {
			t.Fatalf("hint = %q, want fallback", p.Hint)
		}
	})
}

func assertDBSyncConfigValidation(t *testing.T, raw string, requireFieldMaps bool, wantText string) {
	t.Helper()
	_, err := parseDBSyncConfigFlag(raw, requireFieldMaps)
	if err == nil {
		t.Fatalf("parseDBSyncConfigFlag(%s) = nil, want validation error", raw)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Param != "--config" {
		t.Fatalf("Param = %q, want --config", validationErr.Param)
	}
	if !strings.Contains(validationErr.Message, wantText) && !strings.Contains(validationErr.Hint, wantText) {
		t.Fatalf("message=%q hint=%q, want text %q", validationErr.Message, validationErr.Hint, wantText)
	}
	if strings.Contains(validationErr.Message, `"target"`) || strings.Contains(validationErr.Message, `"source"`) {
		t.Fatalf("error message echoes config: %q", validationErr.Message)
	}
}
