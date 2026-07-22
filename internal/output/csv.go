// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/csv"
	"fmt"
	"io"
)

// FormatAsCSV formats data as CSV (with header) and writes it to w.
func FormatAsCSV(w io.Writer, data interface{}) {
	// Match the other legacy wrappers: surface only a marshal failure (as the
	// JSON fallback historically did); plain write failures stay swallowed.
	if err := WriteCSV(w, data); isOutputMarshalError(err) {
		legacyStderrf("json marshal error: %v\n", err)
	}
}

// WriteCSV formats data as CSV and returns marshal or write errors.
func WriteCSV(w io.Writer, data interface{}) error {
	return WriteCSVPaginated(w, data, true)
}

// FormatAsCSVPaginated formats data as CSV with pagination awareness.
// When isFirstPage is true, outputs the header row; otherwise only data rows.
func FormatAsCSVPaginated(w io.Writer, data interface{}, isFirstPage bool) {
	if err := WriteCSVPaginated(w, data, isFirstPage); isOutputMarshalError(err) {
		legacyStderrf("json marshal error: %v\n", err)
	}
}

// WriteCSVPaginated formats data as CSV and returns marshal or write errors.
func WriteCSVPaginated(w io.Writer, data interface{}, isFirstPage bool) error {
	rows, cols, isList := prepareRows(data)
	if cols == nil {
		if isList {
			_, err := fmt.Fprintln(w, "(empty)")
			return err
		} else {
			return WriteJSON(w, data)
		}
	}

	if len(rows) == 0 {
		if isFirstPage {
			_, err := fmt.Fprintln(w, "(empty)")
			return err
		}
		return nil
	}

	if !isList {
		// Single object: key,value rows
		cw := csv.NewWriter(w)
		if isFirstPage {
			if err := cw.Write([]string{"key", "value"}); err != nil {
				return err
			}
		}
		for _, col := range cols {
			if err := cw.Write([]string{col, rows[0][col]}); err != nil {
				return err
			}
		}
		return flushCSV(cw)
	}

	return writeCSVRows(w, rows, cols, isFirstPage)
}

// writeCSVRows writes CSV data rows (and optionally header) using the given columns.
func writeCSVRows(w io.Writer, rows []map[string]string, cols []string, writeHeader bool) error {
	cw := csv.NewWriter(w)
	if writeHeader {
		if err := cw.Write(cols); err != nil {
			return err
		}
	}
	for _, row := range rows {
		record := make([]string, len(cols))
		for i, col := range cols {
			record[i] = row[col]
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	return flushCSV(cw)
}

// flushCSV flushes the csv.Writer and returns any write error.
func flushCSV(cw *csv.Writer) error {
	cw.Flush()
	return cw.Error()
}
