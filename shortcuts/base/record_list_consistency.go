// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// listRecordsVerified reads one record-list page and rechecks ambiguous null
// number cells through records/batch_get. The list matrix endpoint can
// occasionally return a null for a populated number cell without reporting an
// error. A null is otherwise indistinguishable from an intentionally empty
// cell, so the independent ID-based read is the narrowest reliable check.
func listRecordsVerified(runtime *common.RuntimeContext, params map[string]interface{}) (map[string]interface{}, error) {
	path := baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records")
	data, err := baseV3Call(runtime, "GET", path, params, nil)
	if err != nil {
		return nil, err
	}
	return verifyNullNumberCells(runtime, data)
}

func verifyNullNumberCells(runtime *common.RuntimeContext, data map[string]interface{}) (map[string]interface{}, error) {
	fields, fieldsOK := interfaceStrings(data["fields"])
	fieldTypes, typesOK := interfaceStrings(data["field_type_list"])
	recordIDs, recordsOK := interfaceStrings(data["record_id_list"])
	rows, rowsOK := data["data"].([]interface{})
	if !fieldsOK || !typesOK || !recordsOK || !rowsOK || len(fields) != len(fieldTypes) || len(recordIDs) != len(rows) {
		// Keep legacy JSON/Markdown behavior for old response shapes. NDJSON has
		// its own strict matrix parser and will reject malformed responses later.
		return data, nil
	}

	numberIndexes := make([]int, 0, len(fieldTypes))
	numberFields := make([]string, 0, len(fieldTypes))
	for index, fieldType := range fieldTypes {
		if fieldType == "number" {
			numberIndexes = append(numberIndexes, index)
			numberFields = append(numberFields, fields[index])
		}
	}
	if len(numberIndexes) == 0 {
		return data, nil
	}

	affected := make([]string, 0, len(rows))
	affectedSet := make(map[string]bool, len(rows))
	for rowIndex, rawRow := range rows {
		row, ok := rawRow.([]interface{})
		if !ok || len(row) != len(fields) {
			continue
		}
		for _, columnIndex := range numberIndexes {
			if row[columnIndex] == nil {
				affected = append(affected, recordIDs[rowIndex])
				affectedSet[recordIDs[rowIndex]] = true
				break
			}
		}
	}
	if len(affected) == 0 {
		return data, nil
	}

	verified := make(map[string]map[string]interface{}, len(affected))
	path := baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "batch_get")
	for recordStart := 0; recordStart < len(affected); recordStart += maxRecordSelectionCount {
		recordEnd := min(recordStart+maxRecordSelectionCount, len(affected))
		for fieldStart := 0; fieldStart < len(numberFields); fieldStart += maxBatchGetSelectFieldCount {
			fieldEnd := min(fieldStart+maxBatchGetSelectFieldCount, len(numberFields))
			selection := recordSelection{
				recordIDs:    affected[recordStart:recordEnd],
				selectFields: numberFields[fieldStart:fieldEnd],
			}
			result, callErr := baseV3Raw(runtime, "POST", path, nil, recordGetBatchBody(selection))
			batch, callErr := handleBaseAPIResult(result, callErr, "verify record-list number cells")
			if callErr != nil {
				return nil, callErr
			}
			if err := requireSameRecordRevision(data, batch); err != nil {
				return nil, err
			}
			if err := collectVerifiedNumberCells(verified, batch); err != nil {
				return nil, err
			}
		}
	}

	for rowIndex, rawRow := range rows {
		if !affectedSet[recordIDs[rowIndex]] {
			continue
		}
		row, ok := rawRow.([]interface{})
		if !ok || len(row) != len(fields) {
			continue
		}
		values := verified[recordIDs[rowIndex]]
		for _, columnIndex := range numberIndexes {
			if row[columnIndex] == nil && values[fields[columnIndex]] != nil {
				row[columnIndex] = values[fields[columnIndex]]
			}
		}
	}
	return data, nil
}

func interfaceStrings(value interface{}) ([]string, bool) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		out[index] = text
	}
	return out, true
}

func requireSameRecordRevision(listData, batchData map[string]interface{}) error {
	listRev, listHasRev := listData["rev"]
	batchRev, batchHasRev := batchData["rev"]
	if !listHasRev || !batchHasRev || fmt.Sprint(listRev) == fmt.Sprint(batchRev) {
		return nil
	}
	return errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"record table changed while verifying list results (list rev %v, verification rev %v)", listRev, batchRev,
	).WithHint("Retry +record-list so the result comes from one table revision.")
}

func collectVerifiedNumberCells(dst map[string]map[string]interface{}, data map[string]interface{}) error {
	fields, fieldsOK := interfaceStrings(data["fields"])
	recordIDs, recordsOK := interfaceStrings(data["record_id_list"])
	rows, rowsOK := data["data"].([]interface{})
	if !fieldsOK || !recordsOK || !rowsOK || len(recordIDs) != len(rows) {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "record-list verification returned a malformed matrix")
	}
	for rowIndex, rawRow := range rows {
		row, ok := rawRow.([]interface{})
		if !ok || len(row) != len(fields) {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "record-list verification returned a malformed row")
		}
		values := dst[recordIDs[rowIndex]]
		if values == nil {
			values = make(map[string]interface{}, len(fields))
			dst[recordIDs[rowIndex]] = values
		}
		for columnIndex, field := range fields {
			values[field] = row[columnIndex]
		}
	}
	return nil
}
