// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
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
		cfg, err := parseDBSyncConfigFlag(validWithoutFieldMaps, false, false)
		if err != nil {
			t.Fatalf("parseDBSyncConfigFlag preview = %v", err)
		}
		if cfg["mode"] != "batch" {
			t.Fatalf("mode = %v", cfg["mode"])
		}
	})

	t.Run("create-commit auto-matches when field_maps omitted or empty", func(t *testing.T) {
		// create passes allowAutoMatch=true: absent field_maps or an empty array is
		// allowed — the server auto-matches and creates the task.
		if _, err := parseDBSyncConfigFlag(validWithoutFieldMaps, true, true); err != nil {
			t.Fatalf("create without field_maps = %v, want nil (server auto-match)", err)
		}
		emptyArray := strings.Replace(validWithoutFieldMaps, `"target"`, `"field_maps": [], "target"`, 1)
		if _, err := parseDBSyncConfigFlag(emptyArray, true, true); err != nil {
			t.Fatalf("create with empty field_maps = %v, want nil (server auto-match)", err)
		}
		nullValue := strings.Replace(validWithoutFieldMaps, `"target"`, `"field_maps": null, "target"`, 1)
		assertDBSyncConfigValidation(t, nullValue, true, true, "array")
		objectValue := strings.Replace(validWithoutFieldMaps, `"target"`, `"field_maps": {}, "target"`, 1)
		assertDBSyncConfigValidation(t, objectValue, true, true, "array")
		// A present-but-all-disabled array is still a suspected mistake → rejected.
		disabledOnly := `{
			"mode": "batch",
			"source": {"type": "base"},
			"target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
			"field_maps": [{"source": "a", "target": "b", "enabled": false}]
		}`
		assertDBSyncConfigValidation(t, disabledOnly, true, true, "disabled")
		if _, err := parseDBSyncConfigFlag(validWithFieldMaps, true, true); err != nil {
			t.Fatalf("parseDBSyncConfigFlag valid field_maps = %v", err)
		}
	})

	t.Run("update still requires field maps", func(t *testing.T) {
		// update passes allowAutoMatch=false: absent field_maps is rejected (update
		// means changing the mapping, so an explicit mapping is required).
		assertDBSyncConfigValidation(t, validWithoutFieldMaps, true, false, "field_maps")
		emptyArray := strings.Replace(validWithoutFieldMaps, `"target"`, `"field_maps": [], "target"`, 1)
		assertDBSyncConfigValidation(t, emptyArray, true, false, "field_maps")
		disabledOnly := strings.Replace(validWithFieldMaps, `"enabled": true`, `"enabled": false`, 1)
		assertDBSyncConfigValidation(t, disabledOnly, true, false, "disabled")
	})

	t.Run("must be JSON object", func(t *testing.T) {
		assertDBSyncConfigValidation(t, `[1,2,3]`, false, false, "JSON object")
		assertDBSyncConfigValidation(t, `{`, false, false, "JSON object")
		assertDBSyncConfigValidation(t, validWithoutFieldMaps+` {}`, false, false, "one JSON object")
	})

	t.Run("singular map keys are rejected", func(t *testing.T) {
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"source"`, `"field_map": [], "source"`, 1), false, false, "field_maps")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"source"`, `"option_mapping": {}, "source"`, 1), false, false, "option_mappings")
		nestedSingular := `{
			"mode": "streaming",
			"source": {"type": "base"},
			"target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
			"field_maps": [{"source": "record_id", "target": "record_id", "option_mapping": []}]
		}`
		assertDBSyncConfigValidation(t, nestedSingular, true, true, "option_mappings")
	})

	t.Run("mode and endpoints are constrained", func(t *testing.T) {
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"batch"`, `"full"`, 1), false, false, "mode")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"base"`, `"sheet"`, 1), false, false, "source.type")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"postgresql"`, `"mysql"`, 1), false, false, "target.type")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"orders"`, `""`, 1), false, false, "target.table.name")
		assertDBSyncConfigValidation(t, strings.Replace(validWithoutFieldMaps, `"create"`, `"drop"`, 1), false, false, "target.table.action")
	})

	t.Run("schema only is batch create only", func(t *testing.T) {
		validSchemaOnly := strings.Replace(validWithoutFieldMaps, `"mode": "batch"`, `"schema_only": true, "mode": "batch"`, 1)
		if _, err := parseDBSyncConfigFlag(validSchemaOnly, false, false); err != nil {
			t.Fatalf("parseDBSyncConfigFlag valid schema_only = %v", err)
		}
		streamingSchemaOnly := strings.Replace(validWithFieldMaps, `"mode": "streaming"`, `"schema_only": true, "mode": "streaming"`, 1)
		assertDBSyncConfigValidation(t, streamingSchemaOnly, false, false, "schema_only")
		useExistingSchemaOnly := strings.Replace(validSchemaOnly, `"create"`, `"use_existing"`, 1)
		assertDBSyncConfigValidation(t, useExistingSchemaOnly, false, false, "schema_only")
	})

	t.Run("update allows omitted source base_url", func(t *testing.T) {
		// 契约：+db-sync-update 的 base_url 可选——省略时后端从原 syncTask 复用源 URL。
		// parseDBSyncConfigFlag 不得对 base_url 做前置校验（即便 requireFieldMaps=true）。
		noBaseURL := `{
			"mode": "streaming",
			"source": {"type": "base", "table": {"name": "数据表"}},
			"target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
			"field_maps": [{"source_field": "record_id", "target_field": "record_id", "enabled": true}]
		}`
		cfg, err := parseDBSyncConfigFlag(noBaseURL, true, false)
		if err != nil {
			t.Fatalf("parseDBSyncConfigFlag update without base_url = %v, want nil", err)
		}
		source, ok := cfg["source"].(map[string]interface{})
		if !ok {
			t.Fatalf("source = %T, want map", cfg["source"])
		}
		if _, exists := source["base_url"]; exists {
			t.Fatalf("source.base_url present %v, want absent (CLI must not inject default)", source["base_url"])
		}
	})
}

func TestDBSyncEnvironmentHelpDefaultsToOnline(t *testing.T) {
	for _, shortcut := range []common.Shortcut{AppsDBSyncCreate, AppsDBSyncList, AppsDBSyncUpdate} {
		t.Run(shortcut.Command, func(t *testing.T) {
			for _, flag := range shortcut.Flags {
				if flag.Name != "environment" {
					continue
				}
				if !strings.Contains(flag.Desc, "leave unset to use online") {
					t.Fatalf("%s --environment description = %q, want online default", shortcut.Command, flag.Desc)
				}
				if strings.Contains(flag.Desc, "auto-select") || strings.Contains(flag.Desc, "multi-env app uses dev") {
					t.Fatalf("%s --environment description still claims auto-select: %q", shortcut.Command, flag.Desc)
				}
				return
			}
			t.Fatalf("%s missing --environment flag", shortcut.Command)
		})
	}
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
		in := errs.NewAPIError(errs.SubtypeNotFound, "target missing").WithCode(400002483).WithLogID("log_x")
		out := withDBSyncHint(in, "fallback")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("withDBSyncHint returned untyped error: %T", out)
		}
		if !strings.Contains(p.Hint, "target.table.action") {
			t.Fatalf("hint = %q, want target-table recovery", p.Hint)
		}
		if p.Code != 400002483 || p.LogID != "log_x" || p.Subtype != errs.SubtypeNotFound {
			t.Fatalf("problem metadata mutated: %+v", p)
		}
	})

	t.Run("existing hint is preserved", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeInvalidParameters, "bad mapping").WithCode(400002477).WithHint("server hint")
		out := withDBSyncHint(in, "fallback")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "server hint" {
			t.Fatalf("hint = %q, want existing hint", p.Hint)
		}
	})

	t.Run("record-id mapping code guides adding a unique column", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeInvalidParameters, "mapping must include Base 表记录 ID").WithCode(400002477)
		out := withDBSyncHint(in, "fallback")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("withDBSyncHint returned untyped error: %T", out)
		}
		if !strings.Contains(p.Hint, "+db-execute") || !strings.Contains(p.Hint, "base_record_id") {
			t.Fatalf("hint = %q, want +db-execute add-column guidance", p.Hint)
		}
		if p.Code != 400002477 || p.Subtype != errs.SubtypeInvalidParameters {
			t.Fatalf("problem metadata mutated: %+v", p)
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

	t.Run("online DDL subcode gets multi-env dev hint", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "k_dl_4000001：forbid ddl/dcl operation in online env").
			WithCode(500002776).WithLogID("log_ddl")
		out := withDBSyncHint(in, "fallback")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("withDBSyncHint returned untyped error: %T", out)
		}
		if !strings.Contains(p.Hint, "multi-env") || !strings.Contains(p.Hint, "--environment dev") {
			t.Fatalf("hint = %q, want multi-env online-DDL guidance", p.Hint)
		}
		if p.Code != 500002776 || p.LogID != "log_ddl" || p.Subtype != errs.SubtypeUnknown {
			t.Fatalf("problem metadata mutated: %+v", p)
		}
	})

	t.Run("generic 500002776 without subcode does not get dev hint", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "some other 500002776 failure").WithCode(500002776)
		out := withDBSyncHint(in, "fallback")
		p, _ := errs.ProblemOf(out)
		if strings.Contains(p.Hint, "multi-env") {
			t.Fatalf("hint = %q, must not attach online-DDL guidance without subcode", p.Hint)
		}
		if p.Hint != "fallback" {
			t.Fatalf("hint = %q, want fallback for unmapped 500002776", p.Hint)
		}
	})

	t.Run("online DDL subcode does not override server hint", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "k_dl_4000001：forbid ddl/dcl operation in online env").
			WithCode(500002776).WithHint("server hint")
		out := withDBSyncHint(in, "fallback")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "server hint" {
			t.Fatalf("hint = %q, want preserved server hint", p.Hint)
		}
	})
}

func assertDBSyncConfigValidation(t *testing.T, raw string, requireFieldMaps, allowAutoMatch bool, wantText string) {
	t.Helper()
	_, err := parseDBSyncConfigFlag(raw, requireFieldMaps, allowAutoMatch)
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
