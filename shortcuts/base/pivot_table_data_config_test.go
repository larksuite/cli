// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/tidwall/gjson"
)

func TestNormalizePivotTableDataConfig_CreateDefaultsAndMultipleValues(t *testing.T) {
	cfg := map[string]interface{}{
		"table_name": "Sales",
		"values": []interface{}{
			map[string]interface{}{"field_name": "Amount", "rollup": " sum "},
			map[string]interface{}{"field_name": "Customer", "rollup": "count_distinct"},
		},
	}

	got := normalizePivotTableDataConfig(cfg, true)
	if rows, ok := got["rows"].([]interface{}); !ok || len(rows) != 0 {
		t.Fatalf("rows=%#v, want a materialized empty array", got["rows"])
	}
	if columns, ok := got["columns"].([]interface{}); !ok || len(columns) != 0 {
		t.Fatalf("columns=%#v, want a materialized empty array", got["columns"])
	}
	values := got["values"].([]interface{})
	if len(values) != 2 {
		t.Fatalf("values=%#v, want two metrics", values)
	}
	if values[0].(map[string]interface{})["rollup"] != "SUM" || values[1].(map[string]interface{})["rollup"] != "COUNT_DISTINCT" {
		t.Fatalf("values=%#v, want normalized SUM and COUNT_DISTINCT", values)
	}
	if _, exists := got["sort"]; exists {
		t.Fatalf("sort=%#v, create must leave server-side FIELD asc initialization to the API", got["sort"])
	}
}

func TestNormalizePivotTableDataConfig_UpdateExplicitEmptySortIsExactReplacement(t *testing.T) {
	cfg := map[string]interface{}{"sort": []interface{}{}}

	got := normalizePivotTableDataConfig(cfg, false)
	if sortConfig, ok := got["sort"].([]interface{}); !ok || len(sortConfig) != 0 {
		t.Fatalf("sort=%#v, want an explicit empty replacement", got["sort"])
	}
	for _, key := range []string{"rows", "columns", "values"} {
		if _, exists := got[key]; exists {
			t.Fatalf("%s was synthesized in a partial update: %#v", key, got[key])
		}
	}
}

func TestBaseDashboardBlockCreate_InvalidPivotSortReferenceIsTyped(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseDashboardBlockCreate, []string{
		"+dashboard-block-create",
		"--base-token", "app_x",
		"--dashboard-id", "dsh_1",
		"--name", "Bad pivot",
		"--type", "pivotTable",
		"--data-config", `{"table_name":"Sales","rows":[{"field_name":"Category"}],"values":[{"field_name":"Amount","rollup":"SUM"}],"sort":[{"sort_type":"FIELD","order":"asc","group_ref":{"area":"rows","index":1}}]}`,
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected invalid pivot sort reference")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected validation/invalid_argument problem, got %T %v", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--data-config" {
		t.Fatalf("expected --data-config validation error, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "group_ref.index") {
		t.Fatalf("error must identify group_ref.index, got %v", err)
	}
}

func TestBaseDashboardBlockDryRun_PivotRequestBodies(t *testing.T) {
	t.Run("create values-only multiple metrics", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseDashboardBlockCreate, []string{
			"+dashboard-block-create",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--name", "Values only",
			"--type", "pivotTable",
			"--data-config", `{"table_name":"Sales","values":[{"field_name":"Amount","rollup":"sum"},{"field_name":"Customer","rollup":"count_distinct"}]}`,
			"--dry-run",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("dry-run create: %v", err)
		}
		out := stdout.String()
		if got := gjson.Get(out, "data.api.0.body.data_config.values.#").Int(); got != 2 {
			t.Fatalf("values count=%d, stdout=%s", got, out)
		}
		if got := gjson.Get(out, "data.api.0.body.data_config.values.0.rollup").String(); got != "SUM" {
			t.Fatalf("first rollup=%q, stdout=%s", got, out)
		}
		if got := gjson.Get(out, "data.api.0.body.data_config.values.1.rollup").String(); got != "COUNT_DISTINCT" {
			t.Fatalf("second rollup=%q, stdout=%s", got, out)
		}
		if !gjson.Get(out, "data.api.0.body.data_config.rows").IsArray() || !gjson.Get(out, "data.api.0.body.data_config.columns").IsArray() {
			t.Fatalf("create defaults must materialize rows and columns, stdout=%s", out)
		}
		if gjson.Get(out, "data.api.0.body.data_config.sort").Exists() {
			t.Fatalf("CLI must not synthesize server-owned create sort, stdout=%s", out)
		}
	})

	t.Run("update empty sort is exact replacement", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseDashboardBlockUpdate, []string{
			"+dashboard-block-update",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--block-id", "blk_1",
			"--data-config", `{"sort":[]}`,
			"--dry-run",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("dry-run update: %v", err)
		}
		out := stdout.String()
		if !gjson.Get(out, "data.api.0.body.data_config.sort").IsArray() || len(gjson.Get(out, "data.api.0.body.data_config.sort").Array()) != 0 {
			t.Fatalf("sort must remain an explicit empty array, stdout=%s", out)
		}
		for _, key := range []string{"rows", "columns", "values"} {
			if gjson.Get(out, "data.api.0.body.data_config."+key).Exists() {
				t.Fatalf("update synthesized %s, stdout=%s", key, out)
			}
		}
	})
}
