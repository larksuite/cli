// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestRecordListNDJSONOutputInfersFormatAndNormalizesValues(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "limit=2&offset=0",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"rev":             json.Number("42"),
			"timezone":        "Asia/Shanghai",
			"fields":          []any{"Name", "Tags", "When", "Done"},
			"field_id_list":   []any{"fld_name", "fld_tags", "fld_when", "fld_done"},
			"field_type_list": []any{"text", "select", "datetime", "checkbox"},
			"record_id_list":  []any{"rec_1", "rec_2"},
			"data": []any{
				[]any{"Alice", nil, "2026-08-04 12:30:00", true},
				[]any{nil, []any{"P0"}, nil, nil},
			},
			"has_more": false,
			"query_context": map[string]any{
				"record_scope": "all_records", "field_scope": "all_fields",
			},
		}},
	})

	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "2", "--output", "exports/customers.ndjson",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}

	var manifest map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("stdout is not a bare manifest: %v\n%s", err, stdout.String())
	}
	if manifest["format"] != "ndjson" || manifest["rev"] != float64(42) || manifest["records_count"] != float64(2) || manifest["has_more"] != false {
		t.Fatalf("manifest = %#v", manifest)
	}
	columns := manifest["columns"].(map[string]any)
	recordIDColumn := columns["record_id"].(map[string]any)
	if recordIDColumn["physical_type"] != "string" {
		t.Fatalf("record_id column = %#v", recordIDColumn)
	}
	if _, exists := recordIDColumn["field_id"]; exists {
		t.Fatalf("record_id column unexpectedly has field_id: %#v", recordIDColumn)
	}
	nameStats := columns["Name"].(map[string]any)["stats"].(map[string]any)
	if nameStats["null_count"] != float64(1) || nameStats["max_length"] != float64(5) {
		t.Fatalf("Name stats = %#v", nameStats)
	}
	tagStats := columns["Tags"].(map[string]any)["stats"].(map[string]any)
	if tagStats["empty_count"] != float64(1) || tagStats["max_length"] != float64(1) || tagStats["avg_length"] != 0.5 {
		t.Fatalf("Tags stats = %#v", tagStats)
	}
	doneColumn := columns["Done"].(map[string]any)
	if doneColumn["physical_type"] != "boolean" || doneColumn["stats"].(map[string]any)["true_count"] != float64(1) {
		t.Fatalf("Done column = %#v", doneColumn)
	}

	recordPath := filepath.Join(dir, "exports", "customers.ndjson")
	recordInfo, err := os.Stat(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest["record_file_size_bytes"] != float64(recordInfo.Size()) {
		t.Fatalf("record_file_size_bytes = %#v, want %d", manifest["record_file_size_bytes"], recordInfo.Size())
	}
	file, err := os.Open(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var rows []map[string]any
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if tags, ok := rows[0]["Tags"].([]any); !ok || len(tags) != 0 {
		t.Fatalf("empty Tags = %#v", rows[0]["Tags"])
	}
	if got := rows[0]["When"]; got != "2026-08-04T12:30:00+08:00" {
		t.Fatalf("When = %#v", got)
	}
	if rows[1]["Name"] != nil {
		t.Fatalf("Name = %#v, want null", rows[1]["Name"])
	}
	if rows[1]["Done"] != false {
		t.Fatalf("Done = %#v, want false", rows[1]["Done"])
	}
	manifestFileBytes, err := os.ReadFile(filepath.Join(dir, "exports", "customers.manifest.json"))
	if err != nil {
		t.Fatalf("read manifest file: %v", err)
	}
	var savedManifest map[string]any
	if err := json.Unmarshal(manifestFileBytes, &savedManifest); err != nil {
		t.Fatalf("decode manifest file: %v", err)
	}
	if savedManifest["record_file_size_bytes"] != float64(recordInfo.Size()) {
		t.Fatalf("saved record_file_size_bytes = %#v, want %d", savedManifest["record_file_size_bytes"], recordInfo.Size())
	}
}

func TestRecordListNDJSONDefaultsTo2000Records(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=500&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
	})

	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--output", "default.ndjson", "--minimal-stdout",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(dir, "default.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["requested_limit"] != float64(2000) || manifest["records_count"] != float64(1) {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestRecordListNDJSONSerializesPagesAbove500(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=500&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPageWithRev(0, 500, true, "fld_name", 100)},
	})
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=1&offset=500",
		Body: map[string]any{"code": 0, "data": recordMatrixPageWithRev(500, 1, true, "fld_name", 101)},
	})

	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "501", "--offset", "0", "--output", "paged.ndjson", "--minimal-stdout",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	var minimal map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &minimal); err != nil {
		t.Fatal(err)
	}
	if len(minimal) != 5 || minimal["record_file_size_bytes"].(float64) <= 0 || minimal["records_count"] != float64(501) || minimal["has_more"] != true {
		t.Fatalf("minimal stdout = %#v", minimal)
	}
	file, err := os.Open(filepath.Join(dir, "paged.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	if lineCount != 501 {
		t.Fatalf("ndjson line count = %d", lineCount)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(dir, "paged.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["rev"] != float64(100) || manifest["page_count"] != float64(2) || manifest["next_offset"] != float64(501) {
		t.Fatalf("manifest pagination = %#v", manifest)
	}
}

func TestRecordListNDJSONRejectsSchemaChangeWithoutPublishingFiles(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=500&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, true, "fld_name")},
	})
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=500&offset=1",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(1, 1, false, "fld_changed")},
	})

	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "501", "--output", "changed.ndjson",
	}, factory, stdout)
	if err == nil {
		t.Fatal("runShortcut() error = nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("problem = %#v, err = %v", problem, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "changed.ndjson")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("changed.ndjson should not exist, stat err = %v", statErr)
	}
}

func TestRecordListNDJSONOutputCollisionHintsOverwrite(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "existing.ndjson"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=1&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
	})
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "1", "--output", "existing.ndjson",
	}, factory, stdout)
	if err == nil {
		t.Fatal("runShortcut() error = nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Hint == "" || !strings.Contains(problem.Hint, "--overwrite") {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestRecordListNDJSONOverwriteReplacesArtifactPair(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "existing.ndjson"), []byte("old records\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "existing.manifest.json"), []byte("old manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=1&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
	})
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "1", "--output", "existing.ndjson", "--overwrite",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	records, err := os.ReadFile(filepath.Join(dir, "existing.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(records), "old records") || !strings.Contains(string(records), `"record_id":"rec_0000"`) {
		t.Fatalf("existing.ndjson = %s", records)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "existing.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), "old manifest") || !strings.Contains(string(manifest), `"manifest_version": "v1"`) {
		t.Fatalf("existing.manifest.json = %s", manifest)
	}
}

func TestRecordListNDJSONAutoNamesArtifactPair(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	originalNow := recordExportNow
	recordExportNow = func() time.Time {
		return time.Date(2026, time.August, 4, 12, 34, 56, 789_000_000, time.UTC)
	}
	t.Cleanup(func() { recordExportNow = originalNow })

	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=1&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
	})
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "1", "--format", "ndjson", "--minimal-stdout",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	for _, name := range []string{
		"tbl_x_20260804_123456_789.ndjson",
		"tbl_x_20260804_123456_789.manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("auto output %s missing: %v", name, err)
		}
	}
}

func TestRecordListNDJSONRejectsInlineJQ(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "1", "--output", "jq.ndjson", "--jq", ".record_file",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "process the saved records file with Python or another data analysis engine") {
		t.Fatalf("runShortcut() error = %v, want external analysis guidance", err)
	}
}

func TestRecordListNDJSONJQRecordsQueriesExportWithoutChangingArtifacts(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "GET", URL: "limit=2&offset=0",
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 2, false, "fld_name")},
	})
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "2", "--output", "query.ndjson",
		"--jq-records", `map(select(.Name == "Name 1")) | {records_count: length, record_ids: map(.record_id)}`,
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["records_count"] != float64(1) || !reflect.DeepEqual(result["record_ids"], []any{"rec_0001"}) {
		t.Fatalf("jq result = %#v", result)
	}

	records, err := os.ReadFile(filepath.Join(dir, "query.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(records), "\n") != 2 || !strings.Contains(string(records), `"record_id":"rec_0000"`) {
		t.Fatalf("query.ndjson = %s", records)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "query.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"records_count": 2`) {
		t.Fatalf("query.manifest.json = %s", manifest)
	}
}

func TestRecordListJQRecordsValidatesOutputContractBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "requires ndjson",
			args: []string{"--format", "json", "--jq-records", "length"},
			want: "--jq-records requires --format ndjson",
		},
		{
			name: "general jq is unavailable for ndjson",
			args: []string{"--format", "ndjson", "--jq", ".record_file", "--jq-records", "length"},
			want: "process the saved records file with Python or another data analysis engine",
		},
		{
			name: "conflicts with minimal stdout",
			args: []string{"--format", "ndjson", "--minimal-stdout", "--jq-records", "length"},
			want: "--jq-records and --minimal-stdout are mutually exclusive",
		},
		{
			name: "validates expression",
			args: []string{"--format", "ndjson", "--jq-records", "invalid["},
			want: "invalid jq expression",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := append([]string{
				"+record-list", "--base-token", "app_x", "--table-id", "tbl_x", "--limit", "1",
			}, tt.args...)
			err := runShortcut(t, BaseRecordList, args, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runShortcut() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRecordSearchNDJSONPaginatesAndPreservesSearchBody(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	first := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), `"offset":0`) && strings.Contains(string(body), `"limit":500`)
		},
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 500, true, "fld_name")},
	}
	second := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), `"offset":500`) && strings.Contains(string(body), `"limit":1`)
		},
		Body: map[string]any{"code": 0, "data": recordMatrixPage(500, 1, false, "fld_name")},
	}
	registry.Register(first)
	registry.Register(second)

	err := runShortcut(t, BaseRecordSearch, []string{
		"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
		"--keyword", "Name", "--search-field", "Name",
		"--limit", "501", "--output", "search.ndjson", "--filter-json", `{"logic":"and","conditions":[]}`,
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	for _, body := range [][]byte{first.CapturedBody, second.CapturedBody} {
		if !strings.Contains(string(body), `"keyword":"Name"`) || !strings.Contains(string(body), `"filter":{"conditions":[],"logic":"and"}`) {
			t.Fatalf("search body lost query fields: %s", body)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "search.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest["records_count"] != float64(501) || manifest["page_count"] != float64(2) {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestRecordSearchInlineDefaultsTo10Records(t *testing.T) {
	factory, stdout, registry := newExecuteFactory(t)
	searchStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
		BodyFilter: func(body []byte) bool {
			return strings.Contains(string(body), `"limit":10`)
		},
		Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
	}
	registry.Register(searchStub)

	err := runShortcut(t, BaseRecordSearch, []string{
		"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
		"--keyword", "Name", "--search-field", "Name", "--format", "json",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	if !strings.Contains(string(searchStub.CapturedBody), `"limit":10`) {
		t.Fatalf("search body = %s, want dynamic inline limit 10", searchStub.CapturedBody)
	}
}

func TestRecordSearchNDJSONDefaultsTo2000Records(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{
			name: "flags",
			args: []string{"--keyword", "Name", "--search-field", "Name"},
		},
		{
			name: "json body",
			args: []string{"--json", `{"keyword":"Name","search_fields":["Name"]}`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			withBaseWorkingDir(t, dir)
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/search",
				BodyFilter: func(body []byte) bool {
					return strings.Contains(string(body), `"offset":0`) && strings.Contains(string(body), `"limit":500`)
				},
				Body: map[string]any{"code": 0, "data": recordMatrixPage(0, 1, false, "fld_name")},
			})

			args := []string{
				"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
				"--output", "default-search.ndjson", "--minimal-stdout",
			}
			args = append(args, tt.args...)
			err := runShortcut(t, BaseRecordSearch, args, factory, stdout)
			if err != nil {
				t.Fatalf("runShortcut() error = %v", err)
			}

			manifestBytes, err := os.ReadFile(filepath.Join(dir, "default-search.manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest["requested_limit"] != float64(2000) || manifest["records_count"] != float64(1) {
				t.Fatalf("manifest = %#v", manifest)
			}
		})
	}
}

func TestRecordGetNDJSONWritesArtifact(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"timezone":        "UTC",
			"fields":          []any{"Name"},
			"field_id_list":   []any{"fld_name"},
			"field_type_list": []any{"text"},
			"record_id_list":  []any{"rec_1"},
			"data":            []any{[]any{"Alice"}},
			"query_context": map[string]any{
				"record_scope": "all_records", "field_scope": "all_fields",
			},
		}},
	})
	err := runShortcut(t, BaseRecordGet, []string{
		"+record-get", "--base-token", "app_x", "--table-id", "tbl_x",
		"--record-id", "rec_1", "--output", "get.ndjson", "--minimal-stdout",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "get.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"record_id":"rec_1"`) || !strings.Contains(string(data), `"Name":"Alice"`) {
		t.Fatalf("get.ndjson = %s", data)
	}
	manifestData, err := os.ReadFile(filepath.Join(dir, "get.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	queryContext := manifest["query_context"].(map[string]any)
	if queryContext["record_scope"] != "selected_record_ids" ||
		queryContext["requested_record_count"] != float64(1) ||
		queryContext["field_scope"] != "all_fields" {
		t.Fatalf("query_context = %#v", queryContext)
	}
}

func TestRecordNDJSONFlagValidation(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--format", "json", "--output", "out.ndjson",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--output", []string{"--output", "--format"}, "conflicts")

	err = runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "2001", "--output", "out.ndjson",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--limit", nil, "between 1 and 2000")

	err = runShortcut(t, BaseRecordSearch, []string{
		"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
		"--json", `{"keyword":"Alice","search_fields":["Name"],"limit":2001}`,
		"--output", "out.ndjson",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--json", nil, "between 1 and 2000")

	err = runShortcut(t, BaseRecordSearch, []string{
		"+record-search", "--base-token", "app_x", "--table-id", "tbl_x",
		"--json", `{"keyword":"Alice","search_fields":["Name"],"limit":201}`,
		"--format", "json",
	}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--json", nil, "between 1 and 200")
}

func recordMatrixPage(start, count int, hasMore bool, fieldID string) map[string]any {
	recordIDs := make([]any, 0, count)
	rows := make([]any, 0, count)
	for index := 0; index < count; index++ {
		number := start + index
		recordIDs = append(recordIDs, fmt.Sprintf("rec_%04d", number))
		rows = append(rows, []any{fmt.Sprintf("Name %d", number)})
	}
	return map[string]any{
		"timezone":        "UTC",
		"fields":          []any{"Name"},
		"field_id_list":   []any{fieldID},
		"field_type_list": []any{"text"},
		"record_id_list":  recordIDs,
		"data":            rows,
		"has_more":        hasMore,
	}
}

func recordMatrixPageWithRev(start, count int, hasMore bool, fieldID string, rev int64) map[string]any {
	page := recordMatrixPage(start, count, hasMore, fieldID)
	page["rev"] = json.Number(fmt.Sprintf("%d", rev))
	return page
}
