// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type recordVerificationMatrix struct {
	fields         []string
	fieldIDs       []string
	fieldTypes     []string
	recordIDs      []string
	rows           [][]interface{}
	recordNotFound map[string]bool
	rev            interface{}
	hasRev         bool
}

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
	matrix, complete, err := parseListVerificationMatrix(data)
	if err != nil {
		return nil, err
	}
	if !complete {
		// Keep legacy JSON/Markdown behavior for old response shapes. NDJSON has
		// its own strict matrix parser and will reject malformed responses later.
		return data, nil
	}

	type numberColumn struct {
		index   int
		fieldID string
	}
	numberColumns := make([]numberColumn, 0, len(matrix.fieldTypes))
	for index, fieldType := range matrix.fieldTypes {
		if fieldType == "number" || fieldType == "currency" {
			numberColumns = append(numberColumns, numberColumn{index: index, fieldID: matrix.fieldIDs[index]})
		}
	}
	if len(numberColumns) == 0 {
		return data, nil
	}

	affected := make([]string, 0, len(matrix.rows))
	affectedSet := make(map[string]bool, len(matrix.rows))
	ambiguousFields := make([]string, 0, len(numberColumns))
	ambiguousFieldSet := make(map[string]bool, len(numberColumns))
	for rowIndex, row := range matrix.rows {
		for _, column := range numberColumns {
			if row[column.index] != nil {
				continue
			}
			if !affectedSet[matrix.recordIDs[rowIndex]] {
				affected = append(affected, matrix.recordIDs[rowIndex])
				affectedSet[matrix.recordIDs[rowIndex]] = true
			}
			if !ambiguousFieldSet[column.fieldID] {
				ambiguousFields = append(ambiguousFields, column.fieldID)
				ambiguousFieldSet[column.fieldID] = true
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
		for fieldStart := 0; fieldStart < len(ambiguousFields); fieldStart += maxBatchGetSelectFieldCount {
			fieldEnd := min(fieldStart+maxBatchGetSelectFieldCount, len(ambiguousFields))
			selection := recordSelection{
				recordIDs:    affected[recordStart:recordEnd],
				selectFields: ambiguousFields[fieldStart:fieldEnd],
			}
			result, callErr := baseV3Raw(runtime, "POST", path, nil, recordGetBatchBody(selection))
			batch, callErr := handleBaseAPIResult(result, callErr, "verify record-list number cells")
			if callErr != nil {
				return nil, callErr
			}
			batchMatrix, parseErr := parseRecordVerificationMatrix(batch, false)
			if parseErr != nil {
				return nil, parseErr
			}
			if err := requireSameRecordRevision(matrix, batchMatrix); err != nil {
				return nil, err
			}
			if err := collectVerifiedNumberCells(verified, batchMatrix, selection); err != nil {
				return nil, err
			}
		}
	}

	for rowIndex, row := range matrix.rows {
		if !affectedSet[matrix.recordIDs[rowIndex]] {
			continue
		}
		values := verified[matrix.recordIDs[rowIndex]]
		for _, column := range numberColumns {
			value, exists := values[column.fieldID]
			if row[column.index] == nil && exists && value != nil {
				row[column.index] = value
			}
		}
	}
	return data, nil
}

func parseListVerificationMatrix(data map[string]interface{}) (*recordVerificationMatrix, bool, error) {
	for _, key := range []string{"fields", "field_id_list", "field_type_list", "record_id_list", "data"} {
		if _, exists := data[key]; !exists {
			return nil, false, nil
		}
	}
	matrix, err := parseRecordVerificationMatrix(data, true)
	if err != nil {
		return nil, true, err
	}
	return matrix, true, nil
}

func parseRecordVerificationMatrix(data map[string]interface{}, requireFieldTypes bool) (*recordVerificationMatrix, error) {
	fields, fieldsOK := interfaceStrings(data["fields"])
	fieldIDs, fieldIDsOK := interfaceStrings(data["field_id_list"])
	recordIDs, recordsOK := interfaceStrings(data["record_id_list"])
	rawRows, rowsOK := data["data"].([]interface{})
	if !fieldsOK || !fieldIDsOK || !recordsOK || !rowsOK || len(fields) != len(fieldIDs) || len(recordIDs) != len(rawRows) {
		return nil, invalidRecordVerificationMatrix("matrix dimensions do not match")
	}
	fieldTypes := make([]string, len(fields))
	if requireFieldTypes {
		var typesOK bool
		fieldTypes, typesOK = interfaceStrings(data["field_type_list"])
		if !typesOK || len(fields) != len(fieldTypes) {
			return nil, invalidRecordVerificationMatrix("field metadata dimensions do not match")
		}
	}
	rows := make([][]interface{}, len(rawRows))
	for rowIndex, rawRow := range rawRows {
		row, ok := rawRow.([]interface{})
		if !ok || len(row) != len(fields) {
			return nil, invalidRecordVerificationMatrix("row %d does not match the field count", rowIndex+1)
		}
		rows[rowIndex] = row
	}
	recordNotFound := map[string]bool{}
	if raw, exists := data["record_not_found"]; exists && raw != nil {
		items, ok := interfaceStrings(raw)
		if !ok {
			return nil, invalidRecordVerificationMatrix("record_not_found is not a string array")
		}
		for _, recordID := range items {
			recordNotFound[recordID] = true
		}
	}
	rev, hasRev := data["rev"]
	return &recordVerificationMatrix{
		fields: fields, fieldIDs: fieldIDs, fieldTypes: fieldTypes,
		recordIDs: recordIDs, rows: rows, recordNotFound: recordNotFound,
		rev: rev, hasRev: hasRev,
	}, nil
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

func requireSameRecordRevision(listMatrix, batchMatrix *recordVerificationMatrix) error {
	if !listMatrix.hasRev || !batchMatrix.hasRev || fmt.Sprint(listMatrix.rev) == fmt.Sprint(batchMatrix.rev) {
		return nil
	}
	return errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"record table changed while verifying list results (list rev %v, verification rev %v)", listMatrix.rev, batchMatrix.rev,
	).WithHint("Retry +record-list so the result comes from one table revision.")
}

func collectVerifiedNumberCells(dst map[string]map[string]interface{}, matrix *recordVerificationMatrix, selection recordSelection) error {
	fieldIndexes := make(map[string]int, len(matrix.fieldIDs))
	for index, fieldID := range matrix.fieldIDs {
		if _, duplicate := fieldIndexes[fieldID]; duplicate {
			return invalidRecordVerificationMatrix("duplicate field id %q", fieldID)
		}
		fieldIndexes[fieldID] = index
	}
	for _, fieldID := range selection.selectFields {
		if _, exists := fieldIndexes[fieldID]; !exists {
			return invalidRecordVerificationMatrix("requested field %q is missing", fieldID)
		}
	}
	recordIndexes := make(map[string]int, len(matrix.recordIDs))
	for index, recordID := range matrix.recordIDs {
		if _, duplicate := recordIndexes[recordID]; duplicate {
			return invalidRecordVerificationMatrix("duplicate record id %q", recordID)
		}
		recordIndexes[recordID] = index
	}
	for _, recordID := range selection.recordIDs {
		if matrix.recordNotFound[recordID] {
			return invalidRecordVerificationMatrix("requested record %q was not found", recordID)
		}
		rowIndex, exists := recordIndexes[recordID]
		if !exists {
			return invalidRecordVerificationMatrix("requested record %q is missing", recordID)
		}
		values := dst[recordID]
		if values == nil {
			values = make(map[string]interface{}, len(selection.selectFields))
			dst[recordID] = values
		}
		for _, fieldID := range selection.selectFields {
			values[fieldID] = matrix.rows[rowIndex][fieldIndexes[fieldID]]
		}
	}
	return nil
}

func invalidRecordVerificationMatrix(format string, args ...interface{}) error {
	return errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"record-list verification returned a malformed matrix: "+format,
		args...,
	)
}
