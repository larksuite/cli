// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestRecordListNumberVerification(t *testing.T) {
	t.Run("preserves a legitimate null and requests only ambiguous field IDs", func(t *testing.T) {
		factory, stdout, registry := newExecuteFactory(t)
		registry.Register(recordListVerificationListStub(map[string]interface{}{
			"fields":          []interface{}{"Cost", "Stable"},
			"field_id_list":   []interface{}{"fld_cost", "fld_stable"},
			"field_type_list": []interface{}{"number", "number"},
			"record_id_list":  []interface{}{"rec_1"},
			"data":            []interface{}{[]interface{}{nil, 7}},
			"rev":             42,
		}))
		batchStub := recordListVerificationBatchStub(map[string]interface{}{
			"fields":         []interface{}{"Cost"},
			"field_id_list":  []interface{}{"fld_cost"},
			"record_id_list": []interface{}{"rec_1"},
			"data":           []interface{}{[]interface{}{nil}},
			"rev":            42,
		})
		registry.Register(batchStub)

		err := runShortcut(t, BaseRecordList, recordListVerificationArgs(), factory, stdout)
		if err != nil {
			t.Fatalf("runShortcut() error = %v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `null`) || !strings.Contains(got, `7`) {
			t.Fatalf("stdout = %s", got)
		}
		body := string(batchStub.CapturedBody)
		if !strings.Contains(body, `"select_fields":["fld_cost"]`) || strings.Contains(body, `fld_stable`) {
			t.Fatalf("batch_get body = %s", body)
		}
	})

	t.Run("accepts legacy responses without revisions", func(t *testing.T) {
		factory, stdout, registry := newExecuteFactory(t)
		registry.Register(recordListVerificationListStub(map[string]interface{}{
			"fields":          []interface{}{"Cost"},
			"field_id_list":   []interface{}{"fld_cost"},
			"field_type_list": []interface{}{"number"},
			"record_id_list":  []interface{}{"rec_1"},
			"data":            []interface{}{[]interface{}{nil}},
		}))
		registry.Register(recordListVerificationBatchStub(map[string]interface{}{
			"fields":         []interface{}{"Cost"},
			"field_id_list":  []interface{}{"fld_cost"},
			"record_id_list": []interface{}{"rec_1"},
			"data":           []interface{}{[]interface{}{500}},
		}))

		if err := runShortcut(t, BaseRecordList, recordListVerificationArgs(), factory, stdout); err != nil {
			t.Fatalf("runShortcut() error = %v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `500`) || strings.Contains(got, `null`) {
			t.Fatalf("stdout = %s", got)
		}
	})

	for _, test := range []struct {
		name      string
		batchData map[string]interface{}
		want      string
	}{
		{
			name: "rejects a missing requested field",
			batchData: map[string]interface{}{
				"fields":         []interface{}{"Other"},
				"field_id_list":  []interface{}{"fld_other"},
				"record_id_list": []interface{}{"rec_1"},
				"data":           []interface{}{[]interface{}{500}},
				"rev":            42,
			},
			want: "requested field",
		},
		{
			name: "rejects a missing requested record",
			batchData: map[string]interface{}{
				"fields":         []interface{}{"Cost"},
				"field_id_list":  []interface{}{"fld_cost"},
				"record_id_list": []interface{}{},
				"data":           []interface{}{},
				"rev":            42,
			},
			want: "requested record",
		},
		{
			name: "rejects record_not_found",
			batchData: map[string]interface{}{
				"fields":           []interface{}{"Cost"},
				"field_id_list":    []interface{}{"fld_cost"},
				"record_id_list":   []interface{}{"rec_1"},
				"data":             []interface{}{[]interface{}{nil}},
				"record_not_found": []interface{}{"rec_1"},
				"rev":              42,
			},
			want: "was not found",
		},
		{
			name: "rejects a changed table revision",
			batchData: map[string]interface{}{
				"fields":         []interface{}{"Cost"},
				"field_id_list":  []interface{}{"fld_cost"},
				"record_id_list": []interface{}{"rec_1"},
				"data":           []interface{}{[]interface{}{500}},
				"rev":            43,
			},
			want: "table changed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(recordListVerificationListStub(map[string]interface{}{
				"fields":          []interface{}{"Cost"},
				"field_id_list":   []interface{}{"fld_cost"},
				"field_type_list": []interface{}{"number"},
				"record_id_list":  []interface{}{"rec_1"},
				"data":            []interface{}{[]interface{}{nil}},
				"rev":             42,
			}))
			registry.Register(recordListVerificationBatchStub(test.batchData))

			err := runShortcut(t, BaseRecordList, recordListVerificationArgs(), factory, stdout)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("problem = %#v", problem)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %s", stdout.String())
			}
		})
	}
}

func TestRecordListNumberVerificationBatchesAtRecordLimit(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, registry := newExecuteFactory(t)
	recordIDs := make([]interface{}, maxRecordSelectionCount+1)
	rows := make([]interface{}, len(recordIDs))
	firstBatchRecordIDs := make([]interface{}, maxRecordSelectionCount)
	firstBatchRows := make([]interface{}, maxRecordSelectionCount)
	for index := range recordIDs {
		recordID := fmt.Sprintf("rec_%03d", index)
		recordIDs[index] = recordID
		rows[index] = []interface{}{nil}
		if index < maxRecordSelectionCount {
			firstBatchRecordIDs[index] = recordID
			firstBatchRows[index] = []interface{}{nil}
		}
	}
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "limit=201&offset=0",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"timezone":        "UTC",
			"fields":          []interface{}{"Cost"},
			"field_id_list":   []interface{}{"fld_cost"},
			"field_type_list": []interface{}{"number"},
			"record_id_list":  recordIDs,
			"data":            rows,
			"has_more":        false,
			"rev":             42,
		}},
	})
	firstBatch := recordListVerificationBatchStub(map[string]interface{}{
		"fields":         []interface{}{"Cost"},
		"field_id_list":  []interface{}{"fld_cost"},
		"record_id_list": firstBatchRecordIDs,
		"data":           firstBatchRows,
		"rev":            42,
	})
	firstBatch.BodyFilter = batchRecordCountFilter(maxRecordSelectionCount)
	registry.Register(firstBatch)
	secondBatch := recordListVerificationBatchStub(map[string]interface{}{
		"fields":         []interface{}{"Cost"},
		"field_id_list":  []interface{}{"fld_cost"},
		"record_id_list": []interface{}{"rec_200"},
		"data":           []interface{}{[]interface{}{nil}},
		"rev":            42,
	})
	secondBatch.BodyFilter = batchRecordCountFilter(1)
	registry.Register(secondBatch)

	err := runShortcut(t, BaseRecordList, []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "201", "--output", "verified.ndjson", "--minimal-stdout",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	if len(firstBatch.CapturedBodies) != 1 || len(secondBatch.CapturedBodies) != 1 {
		t.Fatalf("batch calls = %d, %d", len(firstBatch.CapturedBodies), len(secondBatch.CapturedBodies))
	}
}

func TestRecordListNumberVerificationBatchesAtFieldLimit(t *testing.T) {
	factory, stdout, registry := newExecuteFactory(t)
	fieldCount := maxBatchGetSelectFieldCount + 1
	fields := make([]interface{}, fieldCount)
	fieldIDs := make([]interface{}, fieldCount)
	fieldTypes := make([]interface{}, fieldCount)
	row := make([]interface{}, fieldCount)
	for index := 0; index < fieldCount; index++ {
		fields[index] = fmt.Sprintf("Cost %03d", index)
		fieldIDs[index] = fmt.Sprintf("fld_%03d", index)
		fieldTypes[index] = "number"
	}
	registry.Register(recordListVerificationListStub(map[string]interface{}{
		"fields":          fields,
		"field_id_list":   fieldIDs,
		"field_type_list": fieldTypes,
		"record_id_list":  []interface{}{"rec_1"},
		"data":            []interface{}{row},
		"rev":             42,
	}))
	firstBatch := recordListVerificationBatchStub(verificationFieldBatchData(fields[:maxBatchGetSelectFieldCount], fieldIDs[:maxBatchGetSelectFieldCount]))
	firstBatch.BodyFilter = batchFieldCountFilter(maxBatchGetSelectFieldCount)
	registry.Register(firstBatch)
	secondBatch := recordListVerificationBatchStub(verificationFieldBatchData(fields[maxBatchGetSelectFieldCount:], fieldIDs[maxBatchGetSelectFieldCount:]))
	secondBatch.BodyFilter = batchFieldCountFilter(1)
	registry.Register(secondBatch)

	if err := runShortcut(t, BaseRecordList, recordListVerificationArgs(), factory, stdout); err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	if len(firstBatch.CapturedBodies) != 1 || len(secondBatch.CapturedBodies) != 1 {
		t.Fatalf("batch calls = %d, %d", len(firstBatch.CapturedBodies), len(secondBatch.CapturedBodies))
	}
}

func verificationFieldBatchData(fields, fieldIDs []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"fields":         fields,
		"field_id_list":  fieldIDs,
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{make([]interface{}, len(fields))},
		"rev":            42,
	}
}

func batchRecordCountFilter(want int) func([]byte) bool {
	return func(body []byte) bool {
		var request struct {
			RecordIDs []string `json:"record_id_list"`
		}
		return json.Unmarshal(body, &request) == nil && len(request.RecordIDs) == want
	}
}

func batchFieldCountFilter(want int) func([]byte) bool {
	return func(body []byte) bool {
		var request struct {
			SelectFields []string `json:"select_fields"`
		}
		return json.Unmarshal(body, &request) == nil && len(request.SelectFields) == want
	}
}

func recordListVerificationArgs() []string {
	return []string{
		"+record-list", "--base-token", "app_x", "--table-id", "tbl_x",
		"--limit", "1", "--format", "json",
	}
}

func recordListVerificationListStub(data map[string]interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "limit=1&offset=0",
		Body:   map[string]interface{}{"code": 0, "data": data},
	}
}

func recordListVerificationBatchStub(data map[string]interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/batch_get",
		Body:   map[string]interface{}{"code": 0, "data": data},
	}
}
