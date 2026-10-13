// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"strings"
)

// cellsSetPlannedWrites selects the existing multi-region write pipeline before
// the dense cells normalizer can turn a flat list into one row. Sparse cells
// have their own explicit coordinates; a bounding rectangle would overwrite
// gaps that the caller never selected.
func cellsSetPlannedWrites(runtime flagView, token string) ([]interface{}, []string, bool, error) {
	if runtime.Changed("writes") {
		ops, notes, err := cellsSetWritesOps(runtime, token)
		return ops, notes, true, err
	}
	items, sparse, err := cellsSetSparseWrites(runtime)
	if !sparse || err != nil {
		return nil, nil, sparse, err
	}
	for _, flag := range []string{"range", "start-cell", "copy-to-range", "formula", "formulas"} {
		if runtime.Changed(flag) {
			return nil, nil, true, sheetsValidationForFlag(flag, "--%s cannot be combined with coordinate-bearing --cells", flag).
				WithHint("keep each target and its content in the --cells item, or use --writes")
		}
	}
	ops, notes, err := buildCellsSetWritesOps(runtime, token, items, "cells")
	return ops, notes, true, err
}

// cellsSetSparseWrites recognizes only a top-level list containing explicit
// A1 cell/range keys (optionally inside the existing lone cells envelope).
// Other shapes retain the dense parser's validation and error ordering.
// runtime.Str is already resolved by the runner's invocation-scoped FileIO.
func cellsSetSparseWrites(runtime flagView) ([]interface{}, bool, error) {
	payload, err := decodeValuesPayload(canonicalPayloadJSON(runtime.Str("cells")))
	if err != nil {
		return nil, false, nil //nolint:nilerr // Recognition probe only; the caller's dense parser reports malformed JSON in its existing validation order.
	}
	items, ok := unwrapCellsEnvelope(payload).([]interface{})
	if !ok {
		return nil, false, nil
	}
	if !hasSparseCellTargets(items) {
		return nil, false, nil
	}
	if len(items) > maxBatchOperations {
		return nil, true, sheetsValidationForFlag("cells", "coordinate-bearing --cells accepts at most %d items; got %d", maxBatchOperations, len(items))
	}
	writes := make([]interface{}, 0, len(items))
	for i, raw := range items {
		write, err := sparseCellWrite(raw, fmt.Sprintf("--cells[%d]", i))
		if err != nil {
			return nil, true, err
		}
		writes = append(writes, write)
	}
	return writes, true, nil
}

func hasSparseCellTargets(items []interface{}) bool {
	for _, raw := range items {
		if item, ok := raw.(map[string]interface{}); ok {
			_, hasCell := item["cell"]
			_, hasRange := item["range"]
			if hasCell || hasRange {
				return true
			}
		}
	}
	return false
}

func sparseCellWrite(raw interface{}, path string) (map[string]interface{}, error) {
	item, ok := raw.(map[string]interface{})
	if !ok {
		return nil, sheetsValidationForFlag("cells", "%s must be an object with one cell or range coordinate", path)
	}
	cell, hasCell := item["cell"]
	target, hasRange := item["range"]
	if hasCell == hasRange {
		return nil, sheetsValidationForFlag("cells", "%s must specify exactly one of cell or range", path)
	}
	if hasCell {
		target = cell
	}
	rangeStr, ok := target.(string)
	if !ok || strings.TrimSpace(rangeStr) == "" {
		return nil, sheetsValidationForFlag("cells", "%s coordinate must be a non-empty A1 string", path)
	}
	extent, err := parseCellRange(rangeStr)
	if err != nil {
		return nil, sheetsValidationForFlag("cells", "%s coordinate: %v", path, err).WithCause(err)
	}
	if hasCell && (extent.rows != 1 || extent.cols != 1) {
		return nil, sheetsValidationForFlag("cells", "%s.cell must name one cell; use range + cells for a block", path)
	}
	block, hasCells := item["cells"]
	values, hasValues := item["values"]
	if hasCells && hasValues {
		return nil, sheetsValidationForFlag("cells", "%s sets both cells and values; keep one block", path)
	}
	if hasValues {
		block = values
	}
	if hasCells || hasValues {
		if len(item) != 2 {
			return nil, sheetsValidationForFlag("cells", "%s block must contain only its coordinate and cells or values; put content and styles inside the block", path)
		}
		// Normalize the decoded value before the flag view: a JSON-looking
		// string is one literal cell, not a second JSON document to decode.
		block = normalizeCellsFlagValue(newMapFlagViewForCommand("+cells-set", map[string]interface{}{"range": rangeStr}), block)
		if hasCell {
			rows, matrix := block.([]interface{})
			if !matrix || len(rows) != 1 {
				return nil, sheetsValidationForFlag("cells", "%s.cell can hold only one cell; use range for a block", path)
			}
			row, isRow := rows[0].([]interface{})
			if !isRow || len(row) != 1 {
				return nil, sheetsValidationForFlag("cells", "%s.cell can hold only one cell; use range for a block", path)
			}
		}
		return map[string]interface{}{"range": rangeStr, "cells": block}, nil
	}
	if extent.rows != 1 || extent.cols != 1 {
		return nil, sheetsValidationForFlag("cells", "%s scalar content needs a single-cell coordinate; use cells or values for a rectangular range", path)
	}
	leaf := make(map[string]interface{}, len(item)-1)
	for key, value := range item {
		if key != "cell" && key != "range" {
			leaf[key] = value
		}
	}
	foldCellLevelStyleVocabulary(leaf)
	for _, key := range sortedKeys(leaf) {
		if !cellCarrierFields[key] && key != "style" {
			return nil, sheetsValidationForFlag("cells", "%s.%s is not a supported cell field", path, key)
		}
	}
	return map[string]interface{}{"range": rangeStr, "cells": []interface{}{[]interface{}{leaf}}}, nil
}
