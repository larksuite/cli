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
	FormatAsCSVPaginated(w, data, true)
}

// FormatAsCSVPaginated formats data as CSV with pagination awareness.
// When isFirstPage is true, outputs the header row; otherwise only data rows.
func FormatAsCSVPaginated(w io.Writer, data interface{}, isFirstPage bool) {
	rows, cols, isList := prepareRows(data)
	if cols == nil {
		if isList {
			fmt.Fprintln(w, "(empty)")
		} else {
			PrintJson(w, data)
		}
		return
	}

	if len(rows) == 0 {
		if isFirstPage {
			fmt.Fprintln(w, "(empty)")
		}
		return
	}

	if !isList {
		// Single object: key,value rows
		cw := csv.NewWriter(w)
		if isFirstPage {
			cw.Write([]string{"key", "value"})
		}
		for _, col := range cols {
			cw.Write([]string{col, rows[0][col]})
		}
		flushCSV(cw)
		return
	}

	writeCSVRows(w, rows, cols, isFirstPage)
}

// writeCSVRows writes CSV data rows (and optionally header) using the given columns.
func writeCSVRows(w io.Writer, rows []map[string]string, cols []string, writeHeader bool) {
	cw := csv.NewWriter(w)
	if writeHeader {
		cw.Write(cols)
	}
	for _, row := range rows {
		record := make([]string, len(cols))
		for i, col := range cols {
			record[i] = row[col]
		}
		cw.Write(record)
	}
	flushCSV(cw)
}

// flushCSV flushes the csv.Writer and reports any write error to stderr.
func flushCSV(cw *csv.Writer) {
	cw.Flush()
	if err := cw.Error(); err != nil {
		writeErrorf("csv write error: %v\n", err)
	}
}
