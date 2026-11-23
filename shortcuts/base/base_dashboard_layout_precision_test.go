// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
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
// (out of range or fractional) fail, absent field is a no-op, explicit null is
// rejected, and non-statistics types reject number_format because the backend
// schemas are strict.
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

	t.Run("number_format explicit null", func(t *testing.T) {
		cfg := baseStat(nil)
		cfg["number_format"] = nil
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "number_format") {
			t.Fatalf("expected explicit null number_format error, got %v", errs)
		}
	})

	t.Run("formatName whitespace rejected", func(t *testing.T) {
		cfg := baseStat(map[string]interface{}{"formatName": " digital ", "precision": 2})
		errs := validateBlockDataConfig("statistics", cfg)
		if !containsSubstr(errs, "formatName") {
			t.Fatalf("expected whitespace formatName error, got %v", errs)
		}
	})

	t.Run("absent number_format is fine", func(t *testing.T) {
		cfg := baseStat(nil)
		if errs := validateBlockDataConfig("statistics", cfg); len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
	})

	t.Run("non-statistics rejects number_format", func(t *testing.T) {
		cfg := map[string]interface{}{
			"table_name":    "T",
			"count_all":     true,
			"group_by":      []interface{}{map[string]interface{}{"field_name": "x", "mode": "integrated"}},
			"number_format": map[string]interface{}{"formatName": "not_a_format", "precision": 99},
		}
		if errs := validateBlockDataConfig("column", cfg); !containsSubstr(errs, "仅支持 statistics") {
			t.Fatalf("column type must reject number_format, got %v", errs)
		}
	})

	t.Run("block type whitespace still validates statistics", func(t *testing.T) {
		cfg := baseStat(map[string]interface{}{"formatName": "bogus"})
		errs := validateBlockDataConfig(" statistics ", cfg)
		if !containsSubstr(errs, "formatName") {
			t.Fatalf("trimmed statistics type must validate number_format, got %v", errs)
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

type changingJSONProvider struct {
	fio *changingJSONFileIO
}

func (p changingJSONProvider) Name() string { return "changing-json" }

func (p changingJSONProvider) ResolveFileIO(context.Context) fileio.FileIO {
	return p.fio
}

type changingJSONFileIO struct {
	contents []string
	opens    int
}

func (f *changingJSONFileIO) Open(string) (fileio.File, error) {
	index := f.opens
	f.opens++
	if index >= len(f.contents) {
		index = len(f.contents) - 1
	}
	return changingJSONFile{Reader: bytes.NewReader([]byte(f.contents[index]))}, nil
}

func (*changingJSONFileIO) Stat(string) (fileio.FileInfo, error) {
	return changingJSONFileInfo{}, nil
}

func (*changingJSONFileIO) ResolvePath(path string) (string, error) { return path, nil }

func (*changingJSONFileIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, nil
}

type changingJSONFile struct {
	*bytes.Reader
}

func (changingJSONFile) Close() error { return nil }

type changingJSONFileInfo struct{}

func (changingJSONFileInfo) Size() int64       { return 0 }
func (changingJSONFileInfo) IsDir() bool       { return false }
func (changingJSONFileInfo) Mode() fs.FileMode { return 0 }

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

// TestBaseDashboardBlockPositionRequiresAllKeys proves a partial position
// object is rejected on both commands. position is submitted whole rather than
// merged field by field, so a partial object does not mean "move it sideways and
// keep the size" — it is a placement with three quarters missing.
func TestBaseDashboardBlockPositionRequiresAllKeys(t *testing.T) {
	for _, tc := range []struct {
		name     string
		position string
		missing  []string
	}{
		{"only x", `{"x":6}`, []string{"y", "w", "h"}},
		{"missing height", `{"x":6,"y":0,"w":6}`, []string{"h"}},
		{"empty object", `{}`, []string{"x", "y", "w", "h"}},
		// A present-but-null coordinate carries exactly as little information
		// as an absent one, so key presence alone must not satisfy the check.
		{"explicit nulls", `{"x":6,"y":null,"w":null,"h":null}`, []string{"y", "w", "h"}},
		{"string coordinate", `{"x":"6","y":0,"w":6,"h":4}`, []string{"x"}},
		{"object coordinate", `{"x":{"a":1},"y":0,"w":6,"h":4}`, []string{"x"}},
		{"bool coordinate", `{"x":true,"y":0,"w":6,"h":4}`, []string{"x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--block-id", "blk_a", "--position", tc.position}
			err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout)
			assertInvalidArgumentValidation(t, err, "--position", nil, "x/y/w/h")
			for _, key := range tc.missing {
				if !strings.Contains(err.Error(), key) {
					t.Fatalf("error must name the missing key %q, got %v", key, err)
				}
			}
		})
	}

	t.Run("create rejects partial too", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
			"--name", "N", "--type", "statistics", "--data-config", `{"table_name":"T","count_all":true}`,
			"--position", `{"x":0,"y":0,"w":6}`}
		err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
		assertInvalidArgumentValidation(t, err, "--position", nil, "h")
	})
}

// TestBaseDashboardBlockPartialPositionBypass proves --no-validate skips the
// key-completeness check (it is semantic) while still parsing the JSON (that is
// required for preview/request parity), and that the partial object reaches the
// request untouched.
func TestBaseDashboardBlockPartialPositionBypass(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--block-id", "blk_a", "--position", `{"x":6}`, "--no-validate"}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("--no-validate must bypass the key check, got err=%v", err)
	}
	body := decodeCapturedBody(t, stub.CapturedBody)
	pos, ok := body["position"].(map[string]interface{})
	if !ok || len(pos) != 1 || toInt(pos["x"]) != 6 {
		t.Fatalf("partial position must pass through verbatim: body=%s", string(stub.CapturedBody))
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
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--position", nil, "JSON")
}

// TestBaseDashboardBlockNoValidateStillParsesJSON ensures --no-validate only
// skips semantic validation/normalization. It must not make dry-run silently
// drop malformed JSON that the execute path would reject.
func TestBaseDashboardBlockNoValidateStillParsesJSON(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		param    string
	}{
		{
			name:     "create position",
			shortcut: BaseDashboardBlockCreate,
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "N", "--type", "statistics", "--data-config", `{"table_name":"T","count_all":true}`,
				"--position", "not-json", "--no-validate", "--dry-run"},
			param: "--position",
		},
		{
			name:     "create data-config",
			shortcut: BaseDashboardBlockCreate,
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "N", "--type", "statistics", "--data-config", "not-json",
				"--no-validate", "--dry-run"},
			param: "--data-config",
		},
		{
			name:     "update position",
			shortcut: BaseDashboardBlockUpdate,
			args: []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
				"--position", "not-json", "--no-validate", "--dry-run"},
			param: "--position",
		},
		{
			name:     "update data-config",
			shortcut: BaseDashboardBlockUpdate,
			args: []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
				"--data-config", "not-json", "--no-validate", "--dry-run"},
			param: "--data-config",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, tc.shortcut, tc.args, factory, stdout)
			assertInvalidArgumentValidation(t, err, tc.param, nil, "JSON")
		})
	}
}

// TestBaseDashboardBlockCreate_ExplicitNullNumberFormatRejected pins the
// distinction between an omitted optional field and an explicit JSON null.
func TestBaseDashboardBlockCreate_ExplicitNullNumberFormatRejected(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", "statistics",
		"--data-config", `{"table_name":"T","count_all":true,"number_format":null}`,
	}
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "number_format")
}

// TestBaseDashboardBlockCreate_NonStatisticsNumberFormatRejected prevents a
// strict backend schema error from being deferred until after a network call.
func TestBaseDashboardBlockCreate_NonStatisticsNumberFormatRejected(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", " column ",
		"--data-config", `{"table_name":"T","count_all":true,"number_format":{"formatName":"digital"}}`,
	}
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "仅支持 statistics")
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
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "formatName")
}

// TestBaseDashboardBlockCreate_PrecisionRejectedOnRealPath exercises precision
// validation through the actual command. The table-driven unit tests above
// decode with UseNumber and therefore hit toIntStrict's json.Number branch,
// which production never takes: parseJSONObject goes through common.ParseJSON
// (a plain json.Unmarshal), so precision always arrives as float64. These cases
// pin the branch real input actually uses.
func TestBaseDashboardBlockCreate_PrecisionRejectedOnRealPath(t *testing.T) {
	for _, tc := range []struct {
		name      string
		precision string
	}{
		{"fractional", "2.5"},
		{"above range", "10"},
		{"negative", "-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "N", "--type", "statistics",
				"--data-config", `{"table_name":"T","count_all":true,"number_format":{"precision":` + tc.precision + `}}`}
			err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
			assertInvalidArgumentValidation(t, err, "--data-config", nil, "precision")
		})
	}

	t.Run("integral float64 accepted", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new"}},
		}
		reg.Register(stub)
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
			"--name", "N", "--type", "statistics",
			"--data-config", `{"table_name":"T","count_all":true,"number_format":{"precision":9}}`}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("precision 9 must be accepted, got err=%v", err)
		}
		body := decodeCapturedBody(t, stub.CapturedBody)
		dc, _ := body["data_config"].(map[string]interface{})
		nf, _ := dc["number_format"].(map[string]interface{})
		if toInt(nf["precision"]) != 9 {
			t.Fatalf("precision not forwarded: body=%s", string(stub.CapturedBody))
		}
	})
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
	assertInvalidArgumentValidation(t, err, "--data-config", nil, "formatName")
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

// TestBaseDashboardBlockNoValidateSnapshotsDataConfigFile proves @file input is
// parsed once and then carried inline to Execute. A changing FileIO provider
// makes a second Open observable and would send a different request body.
func TestBaseDashboardBlockNoValidateSnapshotsDataConfigFile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		method   string
		url      string
		args     []string
	}{
		{
			name:     "create",
			shortcut: BaseDashboardBlockCreate,
			method:   "POST",
			url:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "N", "--type", "statistics", "--data-config", "@payload.json", "--no-validate"},
		},
		{
			name:     "update",
			shortcut: BaseDashboardBlockUpdate,
			method:   "PATCH",
			url:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
			args: []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--block-id", "blk_a", "--data-config", "@payload.json", "--no-validate"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			fio := &changingJSONFileIO{contents: []string{
				`{"marker":"validated"}`,
				`{"marker":"changed-before-execute"}`,
			}}
			factory.FileIOProvider = changingJSONProvider{fio: fio}
			stub := &httpmock.Stub{
				Method: tc.method,
				URL:    tc.url,
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
			}
			reg.Register(stub)

			if err := runShortcut(t, tc.shortcut, tc.args, factory, stdout); err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if fio.opens != 1 {
				t.Fatalf("data-config file opened %d times, want 1", fio.opens)
			}
			body := decodeCapturedBody(t, stub.CapturedBody)
			dataConfig, _ := body["data_config"].(map[string]interface{})
			if dataConfig["marker"] != "validated" {
				t.Fatalf("request used a different file snapshot: body=%s", stub.CapturedBody)
			}
		})
	}
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

// TestBaseDashboardBlockDryRunMatchesExecuteBody proves the preview and the real
// request are assembled from the same body: one set of arguments is run through
// --dry-run and through Execute, and the two request bodies must be equal. This
// is what keeps a malformed or reshaped body from being visible on one path and
// not the other; a future change that reassembles the body on only one path
// fails here.
func TestBaseDashboardBlockDryRunMatchesExecuteBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		method   string
		url      string
		args     []string
	}{
		{
			name:     "create",
			shortcut: BaseDashboardBlockCreate,
			method:   "POST",
			url:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "Revenue", "--type", "statistics",
				"--data-config", `{"table_name":"Orders","count_all":true,"number_format":{"formatName":"dollar_rounded","precision":2}}`,
				"--position", `{"x":0,"y":0,"w":6,"h":4}`},
		},
		{
			name:     "update",
			shortcut: BaseDashboardBlockUpdate,
			method:   "PATCH",
			url:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
			args: []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a",
				"--name", "Total Sales",
				"--data-config", `{"number_format":{"formatName":"dollar_rounded","precision":2}}`,
				"--position", `{"x":6,"y":0,"w":6,"h":4}`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: tc.method,
				URL:    tc.url,
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
			}
			reg.Register(stub)
			if err := runShortcut(t, tc.shortcut, tc.args, factory, stdout); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			executed := decodeCapturedBody(t, stub.CapturedBody)

			dryFactory, dryStdout, _ := newExecuteFactory(t)
			dryArgs := append(append([]string{}, tc.args...), "--dry-run")
			if err := runShortcut(t, tc.shortcut, dryArgs, dryFactory, dryStdout); err != nil {
				t.Fatalf("dry-run err=%v", err)
			}
			previewed, previewCall := dryRunPreviewBody(t, dryStdout.Bytes())

			if !reflect.DeepEqual(previewed, executed) {
				t.Fatalf("preview and executed request bodies diverge:\npreview=%v\nexecuted=%v", previewed, executed)
			}
			// The endpoint has to match too: a preview that shows an
			// unsubstituted :param template, or a different verb, describes a
			// request the client would never send.
			if previewCall.Method != tc.method || previewCall.URL != tc.url {
				t.Fatalf("preview targets %s %s, executed %s %s", previewCall.Method, previewCall.URL, tc.method, tc.url)
			}
			if strings.Contains(previewCall.URL, ":") {
				t.Fatalf("preview URL still carries an unsubstituted placeholder: %s", previewCall.URL)
			}
		})
	}
}

// previewedCall is the method and resolved URL of a previewed request.
type previewedCall struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// dryRunPreviewBody extracts the single previewed request from a --dry-run
// envelope (data.api[0]), returning its body and its method/URL.
func dryRunPreviewBody(t *testing.T, raw []byte) (map[string]interface{}, previewedCall) {
	t.Helper()
	var envelope struct {
		Data struct {
			API []struct {
				previewedCall
				Body map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("dry-run envelope json err=%v out=%s", err, string(raw))
	}
	if len(envelope.Data.API) != 1 {
		t.Fatalf("expected exactly one previewed call, got %d: %s", len(envelope.Data.API), string(raw))
	}
	return envelope.Data.API[0].Body, envelope.Data.API[0].previewedCall
}

// TestBaseDashboardBlockCreateDryRunOmitsBlockID pins that a create preview
// carries only the identifiers the command actually takes. Create has no
// block_id yet, and an empty one in the envelope reads to both humans and
// agents like an argument that failed to resolve.
func TestBaseDashboardBlockCreateDryRunOmitsBlockID(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "N", "--type", "statistics", "--data-config", `{"table_name":"T","count_all":true}`,
		"--dry-run"}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("dry-run envelope json err=%v out=%s", err, stdout.String())
	}
	if _, hasBlockID := envelope.Data["block_id"]; hasBlockID {
		t.Fatalf("create preview must not carry block_id: %s", stdout.String())
	}
	if envelope.Data["dashboard_id"] != "dsh_1" || envelope.Data["base_token"] != "app_x" {
		t.Fatalf("create preview lost its identifiers: %s", stdout.String())
	}
}

// TestDryRunDashboardBaseCarriesDeclaredIdentifiers pins that the shared helper
// keys off which identifiers the command declares, not off whether the value
// happens to be non-empty. Set doubles as the substitution source for :param
// placeholders, so dropping a declared-but-empty identifier would print the raw
// route template and hide the empty argument that caused it.
func TestDryRunDashboardBaseCarriesDeclaredIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   map[string]string
		want    map[string]interface{}
		wantURL string
	}{
		{
			name:    "all declared and set",
			flags:   map[string]string{"base-token": "app_x", "dashboard-id": "dsh_1", "block-id": "blk_1"},
			want:    map[string]interface{}{"base_token": "app_x", "dashboard_id": "dsh_1", "block_id": "blk_1"},
			wantURL: "/b/app_x/d/dsh_1/blk/blk_1",
		},
		{
			name:    "declared but empty still substitutes",
			flags:   map[string]string{"base-token": "app_x", "dashboard-id": "dsh_1", "block-id": ""},
			want:    map[string]interface{}{"base_token": "app_x", "dashboard_id": "dsh_1", "block_id": ""},
			wantURL: "/b/app_x/d/dsh_1/blk/",
		},
		{
			name:    "undeclared identifier is omitted",
			flags:   map[string]string{"base-token": "app_x", "dashboard-id": "dsh_1"},
			want:    map[string]interface{}{"base_token": "app_x", "dashboard_id": "dsh_1"},
			wantURL: "/b/app_x/d/dsh_1/blk/:block_id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newBaseTestRuntime(tc.flags, nil, nil)
			raw, err := json.Marshal(dryRunDashboardBase(rt).GET("/b/:base_token/d/:dashboard_id/blk/:block_id"))
			if err != nil {
				t.Fatalf("marshal err=%v", err)
			}
			var envelope struct {
				API []struct {
					URL string `json:"url"`
				} `json:"api"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("unmarshal err=%v raw=%s", err, raw)
			}
			if envelope.API[0].URL != tc.wantURL {
				t.Fatalf("url = %q, want %q", envelope.API[0].URL, tc.wantURL)
			}
			var got map[string]interface{}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal err=%v raw=%s", err, raw)
			}
			delete(got, "api")
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("identifiers = %v, want %v", got, tc.want)
			}
		})
	}
}
