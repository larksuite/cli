// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/larksuite/cli/internal/validate"
)

var errorSink io.Writer = os.Stderr

// SetErrorSink replaces the package-level sink used by output helpers for
// internal formatting failures. It returns the previous sink so callers can
// restore it after redirecting output.
func SetErrorSink(w io.Writer) io.Writer {
	prev := errorSink
	if w == nil {
		errorSink = io.Discard
		return prev
	}
	errorSink = w
	return prev
}

func writeHelperError(format string, args ...interface{}) {
	fmt.Fprintf(errorSink, format, args...)
}

// PrintJson prints data as formatted JSON to w.
func PrintJson(w io.Writer, data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		writeHelperError("json marshal error: %v\n", err)
		return
	}
	fmt.Fprintln(w, string(b))
}

// PrintNdjson prints data as NDJSON (Newline Delimited JSON) to w.
func PrintNdjson(w io.Writer, data interface{}) {
	emit := func(item interface{}) {
		b, err := json.Marshal(item)
		if err != nil {
			writeHelperError("ndjson marshal error: %v\n", err)
			return
		}
		fmt.Fprintln(w, string(b))
	}
	if arr, ok := data.([]interface{}); ok {
		for _, item := range arr {
			emit(item)
		}
	} else {
		emit(data)
	}
}

func cellStr(val interface{}) string {
	if val == nil {
		return ""
	}
	var s string
	switch v := val.(type) {
	case string:
		s = v
	case json.Number:
		s = v.String()
	case float64:
		if v == float64(int(v)) {
			s = fmt.Sprintf("%d", int(v))
		} else {
			s = fmt.Sprintf("%g", v)
		}
	case bool:
		s = fmt.Sprintf("%v", v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		s = string(b)
	}
	// Sanitize for terminal display: strip ANSI escapes, control chars, dangerous Unicode.
	return validate.SanitizeForTerminal(s)
}

// PrintTable prints rows as a table to w.
// Delegates to FormatAsTable for flattening, column union, and width handling.
func PrintTable(w io.Writer, rows []map[string]interface{}) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no data)")
		return
	}
	items := make([]interface{}, len(rows))
	for i, r := range rows {
		items[i] = r
	}
	FormatAsTable(w, items)
}

// PrintSuccess prints a success message to w.
func PrintSuccess(w io.Writer, msg string) {
	fmt.Fprintf(w, "OK: %s\n", msg)
}

// PrintError prints an error message to w.
func PrintError(w io.Writer, msg string) {
	fmt.Fprintf(w, "ERROR: %s\n", msg)
}
