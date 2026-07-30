// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// ── toIntStrict ──────────────────────────────────────────────────────

// TestToIntStrict guards the strict integer semantics used by number_format
// precision: exact integers pass (including json.Number and integral float64),
// while fractional values, strings and bools are rejected instead of coerced.
func TestToIntStrict(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     interface{}
		want   int
		wantOK bool
	}{
		{"int", 3, 3, true},
		{"int64", int64(9), 9, true},
		{"json.Number integer", json.Number("2"), 2, true},
		{"json.Number fractional", json.Number("2.5"), 0, false},
		{"float64 integral", float64(4), 4, true},
		{"float64 fractional", 2.5, 0, false},
		{"string", "2", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toIntStrict(tc.in)
			if ok != tc.wantOK || (ok && got != tc.want) {
				t.Fatalf("toIntStrict(%v)=(%d,%v), want (%d,%v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// ── validateNumberFormat via validateBlockDataConfig ─────────────────

// TestValidateBlockDataConfig_NumberFormat covers the statistics number_format
// light validation: valid enum + precision pass, illegal formatName / precision
// (out of range or fractional) fail, absent field is a no-op, and non-statistics
// types ignore number_format entirely (F-5).
func TestValidateBlockDataConfig_NumberFormat(t *testing.T) {
	baseStat := func(nf interface{}) map[string]interface{} {
		cfg := map[string]interface{}{
			"table_name": "T",
			"count_all":  true,
		}
		if nf != nil {
			cfg["number_format"] = nf
		}
		return cfg
	}

	// UseNumber-decoded precision keeps json.Number so strict parsing applies.
	decode := func(raw string) map[string]interface{} {
		var m map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		return m
	}

	t.Run("valid formatName + precision", func(t *testing.T) {
		cfg := decode(`{"table_name":"T","count_all":true,"number_format":{"formatName":"dollar_rounded","precision":2}}`)
		if errs := validateBlockDataConfig("statistics", cfg); len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
	})

	t.Run("precision boundaries 0 and 9", func(t *testing.T) {
		for _, p := range []string{"0", "9"} {
			cfg := decode(`{"table_name":"T","count_all":true,"number_format":{"precision":` + p + `}}`)
			if errs := validateBlockDataConfig("statistics", cfg); len(errs) != 0 {
				t.Fatalf("precision=%s expected no errors, got %v", p, errs)
			}
		}
	})

	t.Run("illegal formatName", func(t *testing.T) {
		cfg := baseStat(map[string]interface{}{"formatName": "not_a_format"})
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "formatName") {
			t.Fatalf("expected formatName error, got %v", errs)
		}
	})

	t.Run("precision out of range", func(t *testing.T) {
		cfg := decode(`{"table_name":"T","count_all":true,"number_format":{"precision":10}}`)
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "precision") {
			t.Fatalf("expected precision error, got %v", errs)
		}
	})

	t.Run("precision fractional rejected", func(t *testing.T) {
		cfg := decode(`{"table_name":"T","count_all":true,"number_format":{"precision":2.5}}`)
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "precision") {
			t.Fatalf("expected precision error for 2.5, got %v", errs)
		}
	})

	t.Run("number_format not object", func(t *testing.T) {
		cfg := baseStat("digital")
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "number_format") {
			t.Fatalf("expected number_format object error, got %v", errs)
		}
	})

	t.Run("absent number_format is fine", func(t *testing.T) {
		cfg := baseStat(nil)
		if errs := validateBlockDataConfig("statistics", cfg); len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
	})

	t.Run("non-statistics ignores number_format (F-5)", func(t *testing.T) {
		cfg := map[string]interface{}{
			"table_name":    "T",
			"count_all":     true,
			"group_by":      []interface{}{map[string]interface{}{"field_name": "x", "mode": "integrated"}},
			"number_format": map[string]interface{}{"formatName": "not_a_format", "precision": 99},
		}
		if errs := validateBlockDataConfig("column", cfg); len(errs) != 0 {
			t.Fatalf("column type must ignore number_format, got %v", errs)
		}
	})
}

func containsSubstr(errs []string, want string) bool {
	for _, e := range errs {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

// ── position + number_format body passthrough (Execute) ──────────────

// TestBaseDashboardBlockExecuteCreate_PositionAndNumberFormat proves the create
// Execute path forwards the top-level position sibling and statistics
// data_config.number_format verbatim into the request body.
func TestBaseDashboardBlockExecuteCreate_PositionAndNumberFormat(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new", "name": "Revenue", "type": "statistics"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Revenue", "--type", "statistics",
		"--data-config", `{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}],"number_format":{"formatName":"dollar_rounded","precision":2}}`,
		"--position", `{"x":0,"y":0,"w":6,"h":4}`,
	}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	body := decodeCapturedBody(t, stub.CapturedBody)
	pos, ok := body["position"].(map[string]interface{})
	if !ok {
		t.Fatalf("position missing/not object: body=%s", string(stub.CapturedBody))
	}
	if toInt(pos["w"]) != 6 || toInt(pos["h"]) != 4 {
		t.Fatalf("position not forwarded verbatim: %v", pos)
	}
	dc, _ := body["data_config"].(map[string]interface{})
	nf, ok := dc["number_format"].(map[string]interface{})
	if !ok {
		t.Fatalf("number_format missing: body=%s", string(stub.CapturedBody))
	}
	if nf["formatName"] != "dollar_rounded" || toInt(nf["precision"]) != 2 {
		t.Fatalf("number_format not forwarded verbatim: %v", nf)
	}
}

// TestBaseDashboardBlockExecuteUpdate_Position proves the update Execute path
// forwards the top-level position sibling verbatim.
func TestBaseDashboardBlockExecuteUpdate_Position(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
		"--position", `{"x":6,"y":0,"w":6,"h":4}`,
	}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	body := decodeCapturedBody(t, stub.CapturedBody)
	pos, ok := body["position"].(map[string]interface{})
	if !ok {
		t.Fatalf("position missing: body=%s", string(stub.CapturedBody))
	}
	if toInt(pos["x"]) != 6 || toInt(pos["w"]) != 6 {
		t.Fatalf("position not forwarded verbatim: %v", pos)
	}
}

// TestBaseDashboardBlockCreate_PositionNotValidated proves out-of-range /
// negative coordinates pass through unvalidated (aligns with dws grid).
func TestBaseDashboardBlockCreate_PositionNotValidated(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", "statistics", "--data-config", `{"table_name":"T","count_all":true}`,
		"--position", `{"x":-5,"y":0,"w":99,"h":0}`,
	}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("out-of-range position must pass through, got err=%v", err)
	}
	body := decodeCapturedBody(t, stub.CapturedBody)
	pos, _ := body["position"].(map[string]interface{})
	if toInt(pos["w"]) != 99 {
		t.Fatalf("position coords must pass through verbatim: %v", pos)
	}
}

// TestBaseDashboardBlockCreate_InvalidPositionJSON proves malformed position
// JSON fails consistently on the execute path.
func TestBaseDashboardBlockCreate_InvalidPositionJSON(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", "statistics", "--data-config", `{"table_name":"T","count_all":true}`,
		"--position", `not-json`,
	}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err == nil {
		t.Fatalf("expected error for malformed --position JSON")
	}
}

// TestBaseDashboardBlockCreate_NumberFormatInvalidRejected proves the statistics
// number_format validation is wired into the create command.
func TestBaseDashboardBlockCreate_NumberFormatInvalidRejected(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", "statistics",
		"--data-config", `{"table_name":"T","count_all":true,"number_format":{"formatName":"bogus","precision":2}}`,
	}
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error for bad formatName")
	}
	if !strings.Contains(err.Error(), "formatName") || !strings.Contains(err.Error(), "data_config 校验失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBaseDashboardBlockUpdate_NumberFormatInvalidRejected proves the update
// command intercepts an illegal statistics number_format locally, symmetric with
// create (FIX-1 / backend-design §4.5). Update has no --type flag, so this must
// still fire without demanding table_name/series (which update intentionally
// leaves to the server).
func TestBaseDashboardBlockUpdate_NumberFormatInvalidRejected(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
		"--data-config", `{"number_format":{"formatName":"bogus","precision":2}}`,
	}
	err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error for bad formatName on update")
	}
	if !strings.Contains(err.Error(), "formatName") || !strings.Contains(err.Error(), "data_config 校验失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBaseDashboardBlockUpdate_NumberFormatOnlyAllowed proves that a
// number_format-only data_config (no table_name/series) passes update
// validation — the update path must not run create's strong type checks.
func TestBaseDashboardBlockUpdate_NumberFormatOnlyAllowed(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
		"--data-config", `{"number_format":{"precision":0}}`,
	}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("number_format-only update must pass validation, got err=%v", err)
	}
	body := decodeCapturedBody(t, stub.CapturedBody)
	dc, _ := body["data_config"].(map[string]interface{})
	nf, ok := dc["number_format"].(map[string]interface{})
	if !ok || toInt(nf["precision"]) != 0 {
		t.Fatalf("number_format not forwarded: body=%s", string(stub.CapturedBody))
	}
}

// TestBaseDashboardBlockNoValidateBypass proves --no-validate lets an otherwise
// illegal statistics number_format pass through untouched on BOTH create and
// update paths (the light enum/precision check is skipped along with the rest of
// data_config validation).
func TestBaseDashboardBlockNoValidateBypass(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new"}},
		}
		reg.Register(stub)
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
			"--name", "N", "--type", "statistics",
			"--data-config", `{"table_name":"T","count_all":true,"number_format":{"formatName":"bogus","precision":42}}`,
			"--no-validate",
		}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("--no-validate must bypass number_format validation, got err=%v", err)
		}
		body := decodeCapturedBody(t, stub.CapturedBody)
		dc, _ := body["data_config"].(map[string]interface{})
		nf, ok := dc["number_format"].(map[string]interface{})
		if !ok || nf["formatName"] != "bogus" || toInt(nf["precision"]) != 42 {
			t.Fatalf("illegal number_format must pass through verbatim: body=%s", string(stub.CapturedBody))
		}
	})

	t.Run("update", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
		}
		reg.Register(stub)
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
			"--data-config", `{"number_format":{"formatName":"bogus","precision":42}}`,
			"--no-validate",
		}
		if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
			t.Fatalf("--no-validate must bypass number_format validation on update, got err=%v", err)
		}
		body := decodeCapturedBody(t, stub.CapturedBody)
		dc, _ := body["data_config"].(map[string]interface{})
		nf, ok := dc["number_format"].(map[string]interface{})
		if !ok || nf["formatName"] != "bogus" || toInt(nf["precision"]) != 42 {
			t.Fatalf("illegal number_format must pass through verbatim: body=%s", string(stub.CapturedBody))
		}
	})
}

// TestBaseDashboardBlockExecuteUpdate_PositionNumberFormatName proves a single
// update call carrying position + number_format + name forwards all three into
// the request body together.
func TestBaseDashboardBlockExecuteUpdate_PositionNumberFormatName(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
		"--name", "Total Sales",
		"--data-config", `{"number_format":{"formatName":"dollar_rounded","precision":2}}`,
		"--position", `{"x":6,"y":0,"w":6,"h":4}`,
	}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	body := decodeCapturedBody(t, stub.CapturedBody)
	if body["name"] != "Total Sales" {
		t.Fatalf("name missing/wrong: body=%s", string(stub.CapturedBody))
	}
	pos, ok := body["position"].(map[string]interface{})
	if !ok || toInt(pos["x"]) != 6 || toInt(pos["w"]) != 6 {
		t.Fatalf("position not forwarded: %v", pos)
	}
	dc, _ := body["data_config"].(map[string]interface{})
	nf, ok := dc["number_format"].(map[string]interface{})
	if !ok || nf["formatName"] != "dollar_rounded" || toInt(nf["precision"]) != 2 {
		t.Fatalf("number_format not forwarded: %v", nf)
	}
}

func decodeCapturedBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("captured body json err=%v body=%s", err, string(raw))
	}
	return body
}
