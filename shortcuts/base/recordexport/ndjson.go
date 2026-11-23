// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recordexport

import (
	"encoding/json"
	"io"
)

// WriteNDJSON writes one complete object per line. Values are materialized by
// column name here so future exporters can consume Dataset without inheriting
// NDJSON-specific maps.
func WriteNDJSON(w io.Writer, dataset Dataset) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	for rowIndex, record := range dataset.Records {
		if len(record.Values) != len(dataset.Columns) {
			return newDetailErrorf(
				"record %d has %d values; dataset schema has %d columns",
				rowIndex+1, len(record.Values), len(dataset.Columns),
			)
		}
		object := make(map[string]any, len(dataset.Columns))
		for index, column := range dataset.Columns {
			object[column.Name] = record.Values[index]
		}
		if err := encoder.Encode(object); err != nil {
			return wrapDetailErrorf(err, "encode record %d", rowIndex+1)
		}
	}
	return nil
}
